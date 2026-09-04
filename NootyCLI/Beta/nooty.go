package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// NootyCLI v0.3 — Radin Pro
// Single-file, zero external dependencies.
// Build: go build -ldflags="-s -w" -o nooty nooty.go

var useColor = true

const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	white   = "\033[37m"
)

func init() {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		useColor = false
	}
}
func c(s string) string {
	if useColor {
		return s
	}
	return ""
}

// ---------------- Data ----------------
type Config struct {
	ProviderEndpoint string `json:"provider_endpoint"`
	APIKey           string `json:"api_key"`
	Model            string `json:"model"`
	Safety           string `json:"safety"`
	Workspace        string `json:"workspace"`
	ContextBudget    int    `json:"context_budget"`
}
type Memory struct {
	ID      int    `json:"id"`
	Tag     string `json:"tag"`
	Content string `json:"content"`
	Added   string `json:"added"`
}
type Message struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []NativeToolCall `json:"tool_calls,omitempty"`
}
type NativeToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
type ToolCall struct {
	ID   string
	Name string
	Args map[string]string
}
type Session struct {
	Name      string    `json:"name"`
	Messages  []Message `json:"messages"`
	Mode      string    `json:"mode"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	Workspace string    `json:"workspace"`
}
type DNSResolver struct{ Name, Address string }
type ChatRequest struct {
	Model      string       `json:"model"`
	Messages   []Message    `json:"messages"`
	Stream     bool         `json:"stream"`
	Tools      []ToolSchema `json:"tools,omitempty"`
	ToolChoice interface{}  `json:"tool_choice,omitempty"`
}
type ToolSchema struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}
type ChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
		Delta   struct {
			Content   string           `json:"content"`
			ToolCalls []NativeToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}
type ChatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string           `json:"content"`
			ToolCalls []NativeToolCall `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

type checkpoint struct {
	OriginalPath string    `json:"original_path"`
	BackupPath   string    `json:"backup_path"`
	Deleted      bool      `json:"deleted"`
	Created      time.Time `json:"created"`
}

type AgentStep struct {
	Action  string
	Result  string
	Success bool
	Retry   int
}

var (
	config                                                                    Config
	memories                                                                  []Memory
	sessionMessages                                                           []Message
	currentMode                                                               = "chat"
	workspace, homeDir, nootyDir, memFile, configFile, chatDir, checkpointDir string
	activeDNSName                                                             = "Direct Connection"
	agentRunning                                                              int32
	agentCancel                                                               context.CancelFunc
	fallbackDNS                                                               = []DNSResolver{{"Direct Connection", ""}, {"Electro DNS", "78.157.42.100"}, {"Shecan DNS #1", "178.22.122.100"}, {"Shecan DNS #2", "185.51.200.2"}, {"Begzar DNS #1", "185.55.226.26"}, {"Begzar DNS #2", "185.55.225.25"}}
	clientCache                                                               sync.Map
	httpTransport                                                             = &http.Transport{MaxIdleConns: 100, MaxIdleConnsPerHost: 20, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 35 * time.Second, ExpectContinueTimeout: 1 * time.Second}
	sharedHTTP                                                                = &http.Client{Transport: httpTransport, Timeout: 45 * time.Second}
	sessionMu                                                                 sync.Mutex
	currentSessionName                                                        string
	compactRunning                                                            int32
)

func main() {
	promptFlag := flag.String("p", "", "Direct prompt (non-interactive)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "NootyCLI v0.3 — Radin Pro\n\nUsage: nooty [-p prompt]\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	var err error
	homeDir, err = os.UserHomeDir()
	if err != nil {
		fatal("cannot locate home directory")
	}
	nootyDir = filepath.Join(homeDir, ".nooty")
	chatDir = filepath.Join(nootyDir, "chats")
	checkpointDir = filepath.Join(nootyDir, "checkpoints")
	configFile = filepath.Join(nootyDir, "config.json")
	memFile = filepath.Join(nootyDir, "memories.json")
	for _, d := range []string{nootyDir, chatDir, checkpointDir} {
		_ = os.MkdirAll(d, 0700)
	}
	loadConfig()
	loadMemories()
	if config.Workspace == "" {
		config.Workspace, _ = os.Getwd()
		if config.Workspace == "" {
			config.Workspace = homeDir
		}
	}
	workspace, _ = filepath.Abs(config.Workspace)
	config.Workspace = workspace
	setupSignals()
	currentSessionName = sessionAutoName()
	raceDNS()
	if *promptFlag != "" || stdinPiped() {
		data := ""
		if stdinPiped() {
			b, _ := io.ReadAll(os.Stdin)
			data = string(b)
		}
		full := strings.TrimSpace(strings.TrimSpace(data) + "\n" + strings.TrimSpace(*promptFlag))
		if full == "" {
			return
		}
		handleChat(full)
		return
	}
	drawHeader()
	repl()
}
func fatal(s string) { fmt.Fprintln(os.Stderr, "❌", s); os.Exit(1) }
func stdinPiped() bool {
	st, e := os.Stdin.Stat()
	if e != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice == 0
}

// ---------------- UI ----------------
func drawHeader() {
	w := 68
	line := strings.Repeat("─", w-2)
	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), center(" NOOTY CLI ", w-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), center("v0.3 Radin Pro — Agentic Terminal Intelligence", w-2), c(cyan), c(reset))
	fmt.Println(c(cyan) + "├" + line + "┤" + c(reset))
	entries := [][2]string{{"Provider", config.ProviderEndpoint}, {"Model", config.Model}, {"API Key", mask(config.APIKey)}, {"Workspace", formatPath(workspace)}, {"DNS Shield", activeDNSName}, {"Mode", strings.ToUpper(currentMode) + " Mode"}, {"Context", fmt.Sprintf("~%d tokens", estimateTokens(sessionMessages))}}
	for _, e := range entries {
		fmt.Printf("%s│%s %-12s: %s%-43s%s│%s\n", c(cyan), c(bold)+c(white), e[0], c(green), truncate(e[1], 43), c(cyan), c(reset))
	}
	fmt.Println(c(cyan) + "└" + line + "┘" + c(reset))
	fmt.Printf("%s💡 /help برای راهنما · /mode cli برای Agent Mode · /sessions برای resume%s\n\n", c(dim), c(reset))
}
func center(s string, w int) string {
	if len(s) >= w {
		return s[:w]
	}
	l := (w - len(s)) / 2
	return strings.Repeat(" ", l) + s + strings.Repeat(" ", w-l-len(s))
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n+3:]
}
func formatPath(p string) string {
	if homeDir != "" && strings.HasPrefix(p, homeDir) {
		return "~" + strings.TrimPrefix(p, homeDir)
	}
	return p
}
func mask(k string) string {
	if k == "" {
		return "(not configured)"
	}
	if len(k) <= 8 {
		return k[:1] + strings.Repeat("*", len(k)-1)
	}
	return k[:4] + strings.Repeat("*", len(k)-8) + k[len(k)-4:]
}
func prompt() string {
	if currentMode == "cli" {
		return c(bold) + c(cyan) + "🤖 nooty[agent]" + c(yellow) + " ❯ " + c(reset)
	}
	return c(bold) + c(green) + "⚡ nooty" + c(white) + " ❯ " + c(reset)
}

// ---------------- Signals / REPL ----------------
func setupSignals() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		for range ch {
			if atomic.LoadInt32(&agentRunning) == 1 {
				if agentCancel != nil {
					agentCancel()
				}
				atomic.StoreInt32(&agentRunning, 0)
				fmt.Println("\n" + c(yellow) + "⏸ Agent interrupted. Session preserved; continue or use /resume." + c(reset))
			} else {
				fmt.Println("\n👋 Goodbye!")
				os.Exit(0)
			}
		}
	}()
}
func repl() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for {
		fmt.Print(prompt())
		line, ok := readMultiline(scanner)
		if !ok {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			handleSlash(line)
		} else if strings.HasPrefix(line, "!") && currentMode == "cli" {
			handleShellBang(line[1:])
		} else {
			handleChat(line)
		}
	}
	fmt.Println("\n" + c(dim) + "👋 NootyCLI session ended." + c(reset))
}
func readMultiline(sc *bufio.Scanner) (string, bool) {
	if !sc.Scan() {
		return "", false
	}
	line := sc.Text()
	trim := strings.TrimSpace(line)
	if trim == `"""` || trim == "<<<" {
		term := trim
		var b strings.Builder
		for sc.Scan() {
			l := sc.Text()
			if strings.TrimSpace(l) == term {
				break
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(l)
		}
		return b.String(), true
	}
	return line, true
}

func handleSlash(cmd string) {
	p := strings.Fields(cmd)
	if len(p) == 0 {
		return
	}
	switch p[0] {
	case "/help":
		printHelp()
	case "/mode":
		if len(p) > 1 && p[1] == "cli" {
			currentMode = "cli"
		} else {
			currentMode = "chat"
		}
		fmt.Println("✅ Mode:", currentMode)
	case "/workspace":
		handleWorkspace(p[1:])
	case "/model":
		handleModel(p[1:])
	case "/config":
		handleConfig()
	case "/dns":
		showDNS()
	case "/doctor":
		runDoctor()
	case "/memory":
		handleMemory(p[1:])
	case "/safety":
		handleSafety(p[1:])
	case "/history":
		showHistory()
	case "/compact":
		compactHistory(false)
	case "/sessions":
		listSessions()
	case "/resume":
		resumeSession(p[1:])
	case "/session":
		handleSession(p[1:])
	case "/export":
		exportChat()
	case "/undo":
		undoLast()
	case "/clear":
		sessionMessages = nil
		drawHeader()
	case "/commit":
		quickCommit()
	case "/exit":
		os.Exit(0)
	default:
		fmt.Println("❌ Unknown command:", p[0])
	}
}
func printHelp() {
	fmt.Println(`
Commands:
  /mode chat|cli        Switch modes
  /config               Configure provider/API/model
  /workspace show|set   Manage workspace
  /model show|set|list  Manage model
  /dns                  DNS racing/fallback status
  /doctor               Provider diagnostic
  /memory list|add|forget
  /history              Show current session
  /compact              Compact history into summary
  /sessions             List saved sessions
  /resume [name]        Resume latest or named session
  /session save|load|list <name>
  /export               Export current chat to Markdown
  /undo                 Restore last checkpoint
  /commit               Generate Conventional Commit from git diff
  /clear                Clear session
  /exit                 Exit

CLI safety: destructive/tool commands ask for confirmation unless safety=balanced/allowed by policy.
Multiline input: start with """ or <<< and end with the same marker.
Pipe mode: cat log.txt | nooty "analyze" or nooty -p "..."`)
}

// ---------------- Network ----------------
func buildClient(dns string) *http.Client {
	if dns == "" {
		return &http.Client{Transport: httpTransport, Timeout: 45 * time.Second}
	}
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: 1500 * time.Millisecond}
		return d.DialContext(ctx, "udp", dns+":53")
	}}
	dial := &net.Dialer{Timeout: 10 * time.Second, Resolver: resolver}
	tr := &http.Transport{MaxIdleConns: 100, MaxIdleConnsPerHost: 20, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 35 * time.Second, DialContext: dial.DialContext}
	return &http.Client{Transport: tr, Timeout: 45 * time.Second}
}
func getClient(dns string) *http.Client {
	if v, ok := clientCache.Load(dns); ok {
		return v.(*http.Client)
	}
	cl := buildClient(dns)
	actual, _ := clientCache.LoadOrStore(dns, cl)
	return actual.(*http.Client)
}
func raceDNS() {
	type r struct {
		i  int
		ms time.Duration
		ok bool
	}
	ch := make(chan r, len(fallbackDNS))
	for i, d := range fallbackDNS {
		go func(i int, d DNSResolver) {
			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
			defer cancel()
			url := config.ProviderEndpoint
			if strings.TrimSpace(url) == "" {
				url = "https://api.openai.com/v1"
			}
			req, _ := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(url, "/")+"/models", nil)
			if config.APIKey != "" {
				req.Header.Set("Authorization", "Bearer "+config.APIKey)
			}
			resp, err := getClient(d.Address).Do(req)
			ok := err == nil && resp.StatusCode < 500
			if resp != nil {
				resp.Body.Close()
			}
			ch <- r{i, time.Since(start), ok}
		}(i, d)
	}
	best := r{ms: time.Hour}
	for range fallbackDNS {
		x := <-ch
		if x.ok && x.ms < best.ms {
			best = x
		}
	}
	if best.ms < time.Hour {
		activeDNSName = fallbackDNS[best.i].Name
		return
	}
	activeDNSName = fallbackDNS[0].Name
}
func doWithFallback(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	var last error
	for i, d := range fallbackDNS {
		for attempt := 0; attempt < 3; attempt++ {
			var rdr io.Reader
			if body != nil {
				rdr = bytes.NewReader(body)
			}
			req, err := http.NewRequest(method, url, rdr)
			if err != nil {
				return nil, err
			}
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			resp, err := getClient(d.Address).Do(req)
			if err == nil && resp.StatusCode < 500 && resp.StatusCode != 403 && resp.StatusCode != 451 {
				activeDNSName = d.Name
				return resp, nil
			}
			if resp != nil {
				last = fmt.Errorf("HTTP %d", resp.StatusCode)
				resp.Body.Close()
			} else {
				last = err
			}
			if attempt < 2 {
				time.Sleep(time.Duration(1<<attempt) * 250 * time.Millisecond)
			}
		}
		if i < len(fallbackDNS)-1 {
			fmt.Printf("%s↻ switching DNS: %s → %s%s\n", c(yellow), d.Name, fallbackDNS[i+1].Name, c(reset))
		}
	}
	return nil, fmt.Errorf("network failed: %v", last)
}

// ---------------- Chat / Context ----------------
func estimateTokens(ms []Message) int {
	n := 0
	for _, m := range ms {
		n += len(m.Content)/4 + 4
	}
	return n
}
func getRelevantMemories(q string) []Memory {
	q = strings.ToLower(q)
	var out []Memory
	for _, m := range memories {
		if q == "" || strings.Contains(strings.ToLower(m.Content), q) || strings.Contains(strings.ToLower(m.Tag), q) {
			out = append(out, m)
		}
	}
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

var atRe = regexp.MustCompile(`@[A-Za-z0-9_./\\-]+`)

func injectRefs(input string) string {
	matches := atRe.FindAllString(input, -1)
	if len(matches) == 0 {
		return input
	}
	var b strings.Builder
	b.WriteString(input + "\n\n[Auto context injections]\n")
	seen := map[string]bool{}
	for _, m := range matches {
		p := strings.TrimPrefix(m, "@")
		if seen[p] {
			continue
		}
		seen[p] = true
		abs := safeJoin(workspace, p)
		info, err := os.Stat(abs)
		if err != nil {
			continue
		}
		if info.IsDir() {
			b.WriteString(fmt.Sprintf("\n@%s (directory tree):\n%s\n", p, dirTree(abs, "")))
		} else if info.Size() <= 512*1024 {
			data, err := os.ReadFile(abs)
			if err == nil {
				b.WriteString(fmt.Sprintf("\n@%s:\n```\n%s\n```\n", p, string(data)))
			}
		}
	}
	return b.String()
}

func buildMessages(input string) []Message {
	if len(sessionMessages) >= 20 && atomic.CompareAndSwapInt32(&compactRunning, 0, 1) {
		go func() { defer atomic.StoreInt32(&compactRunning, 0); compactHistory(true) }()
	}
	sys := "You are NootyCLI v0.3, an autonomous software-engineering terminal agent. Be precise and safe. Use native tool calls when available. If native tools are unavailable, use fallback lines: TOOL: tool_name key=\"value\". Multiple independent tool calls may be requested in one response. Prefer patch_file or replace_in_file over write_file for edits. After meaningful changes, verify with run_and_verify or lint_or_check. Do not claim a command succeeded without its result."
	if currentMode == "chat" {
		sys += "\nIn chat mode, do not execute tools; explain commands and code."
	} else {
		sys += "\nIn CLI mode, execute workspace tools autonomously, respecting safety."
	}
	if r := getRelevantMemories(input); len(r) > 0 {
		sys += "\nMemories:\n"
		for _, m := range r {
			sys += fmt.Sprintf("- %s: %s\n", m.Tag, m.Content)
		}
	}
	input = injectRefs(input)
	budget := config.ContextBudget
	if budget <= 0 {
		budget = 10000
	}
	reserve := estimateTokens([]Message{{Role: "system", Content: sys}, {Role: "user", Content: input}})
	hist := fitMessagesToBudget(sessionMessages, budget-reserve)
	return append([]Message{{Role: "system", Content: sys}}, append(hist, Message{Role: "user", Content: input})...)
}

func fitMessagesToBudget(ms []Message, budget int) []Message {
	if budget <= 0 {
		return nil
	}
	out := make([]Message, 0, len(ms))
	used := 0
	for i := len(ms) - 1; i >= 0; i-- {
		t := estimateTokens([]Message{ms[i]})
		if used+t > budget {
			break
		}
		out = append([]Message{ms[i]}, out...)
		used += t
	}
	return out
}

func compactHistory(background bool) {
	sessionMu.Lock()
	if len(sessionMessages) < 6 {
		sessionMu.Unlock()
		return
	}
	older := append([]Message{}, sessionMessages[:len(sessionMessages)-3]...)
	recent := append([]Message{}, sessionMessages[len(sessionMessages)-3:]...)
	sessionMu.Unlock()
	msgs := []Message{{Role: "system", Content: "Summarize the conversation into a compact project-state context. Preserve requirements, decisions, filenames, commands, errors, and pending tasks. Max 220 words."}}
	msgs = append(msgs, older...)
	summary, err := getModelResponseTextWithOptions(msgs, false, nil)
	if err != nil {
		return
	}
	sessionMu.Lock()
	sessionMessages = append([]Message{{Role: "system", Content: "Context summary: " + summary}}, recent...)
	sessionMu.Unlock()
	if !background {
		saveCurrentSession(currentSessionName)
	}
}

func handleChat(input string) {
	msgs := buildMessages(input)
	sessionMu.Lock()
	sessionMessages = append(sessionMessages, Message{Role: "user", Content: input})
	sessionMu.Unlock()
	if currentMode == "cli" {
		runAgentLoop(msgs)
	} else {
		streamResponse(msgs)
	}
	saveCurrentSession(currentSessionName)
}

func commonHeaders() map[string]string {
	h := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" {
		h["Authorization"] = "Bearer " + config.APIKey
	}
	return h
}

func metrics(d time.Duration, chars int) {
	tok := chars / 4
	if tok < 1 && chars > 0 {
		tok = 1
	}
	rate := 0.0
	if d > 0 {
		rate = float64(tok) / d.Seconds()
	}
	fmt.Printf("%s⚡ %d tokens | %.0f tok/s | %.1fs | Model: %s | Context: ~%d tokens%s\n", c(dim), tok, rate, d.Seconds(), config.Model, estimateTokens(sessionMessages), c(reset))
}

func streamResponse(messages []Message) {
	start := time.Now()
	payload := ChatRequest{Model: config.Model, Messages: messages, Stream: true}
	data, _ := json.Marshal(payload)
	resp, err := doWithFallback("POST", strings.TrimRight(config.ProviderEndpoint, "/")+"/chat/completions", data, commonHeaders())
	if err != nil {
		fmt.Println(c(red) + "❌ " + err.Error() + c(reset))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ Provider %d: %s\n", resp.StatusCode, string(b))
		return
	}
	reader := bufio.NewReader(resp.Body)
	var full strings.Builder
	fmt.Print(c(cyan))
	for {
		line, er := reader.ReadString('\n')
		if er != nil && len(line) == 0 {
			break
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			d := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if d == "[DONE]" {
				break
			}
			var ch ChatStreamChunk
			if json.Unmarshal([]byte(d), &ch) == nil {
				for _, cc := range ch.Choices {
					fmt.Print(cc.Delta.Content)
					full.WriteString(cc.Delta.Content)
				}
			}
		}
		if er != nil {
			break
		}
	}
	fmt.Print(c(reset) + "\n")
	sessionMu.Lock()
	sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: full.String()})
	sessionMu.Unlock()
	metrics(time.Since(start), full.Len())
}

func getModelResponseTextWithOptions(messages []Message, stream bool, tools []ToolSchema) (string, error) {
	start := time.Now()
	payload := ChatRequest{Model: config.Model, Messages: messages, Stream: stream, Tools: tools}
	data, _ := json.Marshal(payload)
	resp, err := doWithFallback("POST", strings.TrimRight(config.ProviderEndpoint, "/")+"/chat/completions", data, commonHeaders())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	if stream {
		return parseStreamingText(resp.Body, start)
	}
	b, _ := io.ReadAll(resp.Body)
	var out ChatResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("empty choices")
	}
	return out.Choices[0].Message.Content, nil
}

func parseStreamingText(r io.Reader, start time.Time) (string, error) {
	br := bufio.NewReader(r)
	var b strings.Builder
	for {
		line, err := br.ReadString('\n')
		if err != nil && len(line) == 0 {
			break
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			d := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if d == "[DONE]" {
				break
			}
			var ch ChatStreamChunk
			if json.Unmarshal([]byte(d), &ch) == nil {
				for _, cc := range ch.Choices {
					if cc.Delta.Content != "" {
						fmt.Print(c(cyan) + cc.Delta.Content + c(reset))
						b.WriteString(cc.Delta.Content)
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
	metrics(time.Since(start), b.Len())
	return b.String(), nil
}

// ---------------- Agent ----------------
func runAgentLoop(messages []Message) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s⚠️ Agent crashed: %v. Session preserved.%s\n", c(red), r, c(reset))
		}
		atomic.StoreInt32(&agentRunning, 0)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	agentCancel = cancel
	atomic.StoreInt32(&agentRunning, 1)
	defer cancel()

	planMsgs := append([]Message{}, messages...)
	planMsgs = append(planMsgs, Message{Role: "user", Content: "Create a concise execution plan. Do not execute tools yet."})
	planText, err := getModelResponseTextWithOptions(planMsgs, true, toolSchemas())
	if err != nil {
		fmt.Println("❌ Planning failed:", err)
		return
	}
	fmt.Println("\n" + c(cyan) + c(bold) + "📋 Plan:" + c(reset))
	fmt.Println(planText)
	fmt.Print(c(bold) + "Approve execution? [Y/n]: " + c(reset))
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	if strings.EqualFold(strings.TrimSpace(scanner.Text()), "n") {
		fmt.Println("🛑 Cancelled.")
		return
	}

	msgs := append([]Message{}, messages...)
	msgs = append(msgs,
		Message{Role: "assistant", Content: planText},
		Message{Role: "user", Content: "Plan approved. Execute using tools. Use parallel independent calls when possible. After each tool result, reflect and self-correct if needed."},
	)

	for step := 0; step < 16; step++ {
		select {
		case <-ctx.Done():
			return
		default:
		}
		text, nativeCalls, err := requestAgent(ctx, msgs)
		if err != nil {
			fmt.Println("❌ Agent error:", err)
			return
		}
		calls := nativeCalls
		if len(calls) == 0 {
			calls = extractToolCalls(text)
		}
		if len(calls) == 0 {
			fmt.Println(c(green) + text + c(reset))
			sessionMu.Lock()
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: text})
			sessionMu.Unlock()
			return
		}

		results := executeToolBatch(ctx, calls)
		assistantCallMsg := Message{Role: "assistant", Content: text}
		for _, call := range calls {
			assistantCallMsg.ToolCalls = append(assistantCallMsg.ToolCalls, nativeFromToolCall(call))
		}
		msgs = append(msgs, assistantCallMsg)
		for _, r := range results {
			state := "failed"
			if r.Success {
				state = "success"
			}
			msg := fmt.Sprintf("Tool %s (%s):\n%s", r.Call.Name, state, r.Result)
			if len(msg) > 5000 {
				msg = msg[:5000] + "\n…truncated"
			}
			fmt.Printf("%s🔧 %s%s\n%s\n", c(yellow), r.Call.Name, c(reset), r.Result)
			msgs = append(msgs, Message{Role: "tool", Content: msg, ToolCallID: r.Call.ID})
			if !r.Success {
				msgs = append(msgs, Message{Role: "user", Content: "Reflection: the previous tool failed. Identify the cause, revise the plan if necessary, and correct it. Retry at most twice for this step."})
			}
		}
	}
	fmt.Println(c(yellow) + "⚠️ Agent step limit reached." + c(reset))
}

func nativeFromToolCall(tc ToolCall) NativeToolCall {
	return NativeToolCall{ID: tc.ID, Type: "function", Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: tc.Name, Arguments: jsonString(tc.Args)}}
}

type toolResult struct {
	Call    ToolCall
	Result  string
	Success bool
}

func executeToolBatch(ctx context.Context, calls []ToolCall) []toolResult {
	out := make([]toolResult, len(calls))
	var wg sync.WaitGroup
	for i, tc := range calls {
		wg.Add(1)
		go func(i int, tc ToolCall) {
			defer wg.Done()
			approved := approveTool(tc)
			if !approved {
				out[i] = toolResult{tc, "Operation denied by safety policy.", false}
				return
			}
			res, err := runToolContext(ctx, tc.Name, tc.Args)
			if err != nil {
				out[i] = toolResult{tc, "Tool Error: " + err.Error(), false}
				return
			}
			out[i] = toolResult{tc, res, true}
		}(i, tc)
	}
	wg.Wait()
	return out
}
func approveTool(tc ToolCall) bool {
	switch tc.Name {
	case "list_files", "tree", "read_file", "search_code", "file_info", "git_status", "git_diff", "find_files", "count_tokens":
		return true
	}
	if config.Safety == "balanced" {
		return true
	}
	if tc.Name == "delete_file" {
		fmt.Printf("%s⚠️ DELETE %s — type DELETE:%s ", c(red), tc.Args["path"], c(reset))
		r := bufio.NewReader(os.Stdin)
		v, _ := r.ReadString('\n')
		return strings.TrimSpace(v) == "DELETE"
	}
	fmt.Printf("Confirm %s? [Y/n]: ", tc.Name)
	r := bufio.NewReader(os.Stdin)
	v, _ := r.ReadString('\n')
	return !strings.EqualFold(strings.TrimSpace(v), "n")
}
func requestAgent(ctx context.Context, msgs []Message) (string, []ToolCall, error) {
	payload := ChatRequest{Model: config.Model, Messages: msgs, Stream: false, Tools: toolSchemas(), ToolChoice: "auto"}
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(config.ProviderEndpoint, "/")+"/chat/completions", bytes.NewReader(data))
	for k, v := range commonHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := doWithRequestFallback(ctx, req, data)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var out ChatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", nil, err
	}
	if len(out.Choices) == 0 {
		return "", nil, errors.New("empty choices")
	}
	m := out.Choices[0].Message
	var native []ToolCall
	for _, tc := range m.ToolCalls {
		args := map[string]string{}
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		native = append(native, ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: args})
	}
	return m.Content, native, nil
}
func doWithRequestFallback(ctx context.Context, req *http.Request, body []byte) (*http.Response, error) {
	var last error
	for i, d := range fallbackDNS {
		for a := 0; a < 3; a++ {
			r2 := req.Clone(ctx)
			r2.Body = io.NopCloser(bytes.NewReader(body))
			resp, err := getClient(d.Address).Do(r2)
			if err == nil && resp.StatusCode < 500 && resp.StatusCode != 403 && resp.StatusCode != 451 {
				activeDNSName = d.Name
				return resp, nil
			}
			if resp != nil {
				last = fmt.Errorf("HTTP %d", resp.StatusCode)
				resp.Body.Close()
			} else {
				last = err
			}
			if a < 2 {
				time.Sleep(time.Duration(1<<a) * 250 * time.Millisecond)
			}
		}
		i++
	}
	return nil, last
}
func extractToolCalls(text string) []ToolCall {
	var out []ToolCall
	re := regexp.MustCompile(`(?m)^\s*TOOL[:：]\s*([A-Za-z0-9_]+)\s*(.*)$`)
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		out = append(out, *parseToolArgs(m[1], m[2]))
	}
	return out
}
func parseToolArgs(name, argsStr string) *ToolCall {
	args := map[string]string{}
	re := regexp.MustCompile(`([A-Za-z0-9_]+)=("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|\S+)`)
	for _, m := range re.FindAllStringSubmatch(argsStr, -1) {
		v := m[2]
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		v = strings.ReplaceAll(v, "\\n", "\n")
		v = strings.ReplaceAll(v, "\\t", "\t")
		args[m[1]] = v
	}
	return &ToolCall{Name: name, Args: args}
}
func mergeToolDeltas(dst []NativeToolCall, src []NativeToolCall) []NativeToolCall {
	for _, d := range src {
		idx := -1
		for i, x := range dst {
			if x.IndexLike(d) {
				idx = i
				break
			}
		}
		if idx < 0 {
			dst = append(dst, d)
			continue
		}
		dst[idx].Function.Arguments += d.Function.Arguments
		if d.Function.Name != "" {
			dst[idx].Function.Name = d.Function.Name
		}
	}
	return dst
}
func (t NativeToolCall) IndexLike(x NativeToolCall) bool {
	if t.ID != "" && x.ID != "" {
		return t.ID == x.ID
	}
	return true
}
func jsonString(m map[string]string) string { b, _ := json.Marshal(m); return string(b) }

// ---------------- Tools ----------------
func toolSchemas() []ToolSchema {
	return []ToolSchema{
		{Type: "function", Function: ToolFunction{Name: "list_files", Description: "List directory entries", Parameters: obj(map[string]interface{}{"type": "object", "properties": map[string]interface{}{"path": map[string]string{"type": "string"}}})}},
		{Type: "function", Function: ToolFunction{Name: "tree", Description: "Show gitignore-aware directory tree", Parameters: obj(map[string]interface{}{"type": "object", "properties": map[string]interface{}{"path": map[string]string{"type": "string"}}})}},
		{Type: "function", Function: ToolFunction{Name: "read_file", Description: "Read a text file", Parameters: obj(map[string]interface{}{"type": "object", "required": []string{"path"}, "properties": map[string]interface{}{"path": map[string]string{"type": "string"}}})}},
		{Type: "function", Function: ToolFunction{Name: "write_file", Description: "Write full file; prefer patch/replace for existing files", Parameters: obj(fileArgs("content"))}},
		{Type: "function", Function: ToolFunction{Name: "create_file", Description: "Create or write file", Parameters: obj(fileArgs("content"))}},
		{Type: "function", Function: ToolFunction{Name: "append_file", Description: "Append text to file", Parameters: obj(fileArgs("content"))}},
		{Type: "function", Function: ToolFunction{Name: "patch_file", Description: "Replace first exact block in file", Parameters: obj(map[string]interface{}{"type": "object", "required": []string{"path", "search", "replace"}, "properties": map[string]interface{}{"path": map[string]string{"type": "string"}, "search": map[string]string{"type": "string"}, "replace": map[string]string{"type": "string"}}})}},
		{Type: "function", Function: ToolFunction{Name: "replace_in_file", Description: "Exact first search and replace", Parameters: obj(map[string]interface{}{"type": "object", "required": []string{"path", "old", "new"}, "properties": map[string]interface{}{"path": map[string]string{"type": "string"}, "old": map[string]string{"type": "string"}, "new": map[string]string{"type": "string"}}})}},
		{Type: "function", Function: ToolFunction{Name: "delete_file", Description: "Delete a file", Parameters: obj(map[string]interface{}{"type": "object", "required": []string{"path"}, "properties": map[string]interface{}{"path": map[string]string{"type": "string"}}})}},
		{Type: "function", Function: ToolFunction{Name: "search_code", Description: "Search text line by line, gitignore-aware", Parameters: obj(map[string]interface{}{"type": "object", "required": []string{"query"}, "properties": map[string]interface{}{"query": map[string]string{"type": "string"}, "path": map[string]string{"type": "string"}}})}},
		{Type: "function", Function: ToolFunction{Name: "find_files", Description: "Find files by glob-like pattern", Parameters: obj(map[string]interface{}{"type": "object", "required": []string{"pattern"}, "properties": map[string]interface{}{"pattern": map[string]string{"type": "string"}, "path": map[string]string{"type": "string"}}})}},
		{Type: "function", Function: ToolFunction{Name: "file_info", Description: "File metadata", Parameters: obj(map[string]interface{}{"type": "object", "required": []string{"path"}, "properties": map[string]interface{}{"path": map[string]string{"type": "string"}}})}},
		{Type: "function", Function: ToolFunction{Name: "git_status", Description: "Git status", Parameters: obj(map[string]interface{}{"type": "object", "properties": map[string]interface{}{}})}},
		{Type: "function", Function: ToolFunction{Name: "git_diff", Description: "Git diff", Parameters: obj(map[string]interface{}{"type": "object", "properties": map[string]interface{}{}})}},
		{Type: "function", Function: ToolFunction{Name: "run_command", Description: "Run a shell command with live streaming", Parameters: obj(map[string]interface{}{"type": "object", "required": []string{"command"}, "properties": map[string]interface{}{"command": map[string]string{"type": "string"}, "timeout": map[string]string{"type": "string"}}})}},
		{Type: "function", Function: ToolFunction{Name: "run_and_verify", Description: "Run command and return status/output for self-correction", Parameters: obj(map[string]interface{}{"type": "object", "required": []string{"command"}, "properties": map[string]interface{}{"command": map[string]string{"type": "string"}, "timeout": map[string]string{"type": "string"}}})}},
		{Type: "function", Function: ToolFunction{Name: "lint_or_check", Description: "Run basic Go checks or project tests", Parameters: obj(map[string]interface{}{"type": "object", "properties": map[string]interface{}{"command": map[string]string{"type": "string"}}})}},
		{Type: "function", Function: ToolFunction{Name: "count_tokens", Description: "Estimate tokens from content", Parameters: obj(map[string]interface{}{"type": "object", "required": []string{"content"}, "properties": map[string]interface{}{"content": map[string]string{"type": "string"}}})}},
	}
}
func obj(v map[string]interface{}) map[string]interface{} { return v }
func fileArgs(content string) map[string]interface{} {
	return map[string]interface{}{"type": "object", "required": []string{"path", content}, "properties": map[string]interface{}{"path": map[string]string{"type": "string"}, content: map[string]string{"type": "string"}}}
}

func runToolContext(ctx context.Context, name string, args map[string]string) (string, error) {
	switch name {
	case "list_files":
		p := workspace
		if args["path"] != "" && args["path"] != "." {
			p = safeJoin(workspace, args["path"])
		}
		es, err := os.ReadDir(p)
		if err != nil {
			return "", err
		}
		var n []string
		for _, e := range es {
			if shouldIgnore(e.Name(), e.IsDir()) {
				continue
			}
			if e.IsDir() {
				n = append(n, e.Name()+"/")
			} else {
				n = append(n, e.Name())
			}
		}
		sort.Strings(n)
		if len(n) == 0 {
			return "(directory empty)", nil
		}
		return strings.Join(n, "\n"), nil
	case "tree":
		p := workspace
		if args["path"] != "" && args["path"] != "." {
			p = safeJoin(workspace, args["path"])
		}
		return dirTree(p, ""), nil
	case "read_file":
		data, err := os.ReadFile(safeJoin(workspace, args["path"]))
		if err != nil {
			return "", err
		}
		return string(data), nil
	case "write_file", "create_file":
		p := safeJoin(workspace, args["path"])
		if err := checkpointFile(p, false); err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return "", err
		}
		if err := showDiffAndWrite(p, args["content"]); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ File saved: %s", args["path"]), nil
	case "append_file":
		p := safeJoin(workspace, args["path"])
		if err := checkpointFile(p, false); err != nil {
			return "", err
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return "", err
		}
		defer f.Close()
		_, err = f.WriteString(args["content"])
		return "✅ Appended.", err
	case "patch_file":
		return patchExact(args["path"], args["search"], args["replace"])
	case "replace_in_file":
		return patchExact(args["path"], args["old"], args["new"])
	case "delete_file":
		p := safeJoin(workspace, args["path"])
		if err := checkpointFile(p, true); err != nil {
			return "", err
		}
		if err := os.Remove(p); err != nil {
			return "", err
		}
		return "✅ File removed: " + args["path"], nil
	case "search_code":
		return searchCode(args["query"], args["path"])
	case "find_files":
		return findFiles(args["pattern"], args["path"])
	case "file_info":
		p := safeJoin(workspace, args["path"])
		i, err := os.Stat(p)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Path: %s\nSize: %d bytes\nMode: %s\nModified: %s", p, i.Size(), i.Mode(), i.ModTime().Format(time.RFC3339)), nil
	case "git_status":
		return shellCapture(ctx, "git", "status", "--short")
	case "git_diff":
		return shellCapture(ctx, "git", "diff")
	case "run_command", "run_and_verify":
		return runCommandStreaming(ctx, args["command"], args["timeout"])
	case "lint_or_check":
		cmd := args["command"]
		if cmd == "" {
			if _, e := os.Stat(filepath.Join(workspace, "go.mod")); e == nil {
				cmd = "go vet ./..."
			} else {
				cmd = ""
			}
		}
		if cmd == "" {
			return "No default linter detected.", nil
		}
		return runCommandStreaming(ctx, cmd, args["timeout"])
	case "count_tokens":
		return strconv.Itoa(len(args["content"])/4 + 4), nil
	}
	return "", fmt.Errorf("unknown tool: %s", name)
}
func safeJoin(base, rel string) string {
	if filepath.IsAbs(rel) {
		rel = strings.TrimPrefix(filepath.Clean(rel), string(filepath.Separator))
	}
	p := filepath.Clean(filepath.Join(base, rel))
	absBase, _ := filepath.Abs(base)
	absP, _ := filepath.Abs(p)
	if absP != absBase && !strings.HasPrefix(absP, absBase+string(filepath.Separator)) {
		return absBase
	}
	return absP
}
func checkpointFile(p string, deleted bool) error {
	if _, err := os.Stat(p); err != nil {
		return nil
	}
	ts := time.Now().Format("20060102_150405.000000000")
	base := filepath.Base(p)
	backup := filepath.Join(checkpointDir, ts+"__"+strings.ReplaceAll(base, string(filepath.Separator), "_"))
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	if err = os.WriteFile(backup, data, 0600); err != nil {
		return err
	}
	cp := checkpoint{p, backup, deleted, time.Now()}
	b, _ := json.Marshal(cp)
	return os.WriteFile(backup+".json", b, 0600)
}
func undoLast() {
	ents, _ := os.ReadDir(checkpointDir)
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() > ents[j].Name() })
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(checkpointDir, e.Name()))
		if err != nil {
			continue
		}
		var cp checkpoint
		if json.Unmarshal(b, &cp) != nil {
			continue
		}
		data, err := os.ReadFile(cp.BackupPath)
		if err != nil {
			continue
		}
		_ = os.MkdirAll(filepath.Dir(cp.OriginalPath), 0755)
		if err = os.WriteFile(cp.OriginalPath, data, 0644); err == nil {
			fmt.Println("✅ Restored:", cp.OriginalPath)
			return
		}
	}
	fmt.Println("ℹ️ No checkpoint available.")
}
func patchExact(path, old, new string) (string, error) {
	p := safeJoin(workspace, path)
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	s := string(data)
	if !strings.Contains(s, old) {
		return "❌ Search block not found in file. Exact match required.", nil
	}
	ns := strings.Replace(s, old, new, 1)
	if err = checkpointFile(p, false); err != nil {
		return "", err
	}
	if err = showDiffAndWrite(p, ns); err != nil {
		return "", err
	}
	return "✅ Patched: " + path, nil
}
func showDiffAndWrite(p, newContent string) error {
	oldContent := ""
	if b, err := os.ReadFile(p); err == nil {
		oldContent = string(b)
	}
	if oldContent != newContent {
		showSimpleDiff(oldContent, newContent)
	}
	return os.WriteFile(p, []byte(newContent), 0644)
}
func showSimpleDiff(oldc, newc string) {
	ol := strings.Split(oldc, "\n")
	nl := strings.Split(newc, "\n")
	max := len(ol)
	if len(nl) > max {
		max = len(nl)
	}
	fmt.Println(c(bold) + "🔍 Proposed Changes Preview:" + c(reset))
	for i := 0; i < max && i < 80; i++ {
		var a, b string
		if i < len(ol) {
			a = ol[i]
		}
		if i < len(nl) {
			b = nl[i]
		}
		if a == b {
			continue
		}
		if a != "" {
			fmt.Println(c(red) + "- " + a + c(reset))
		}
		if b != "" {
			fmt.Println(c(green) + "+ " + b + c(reset))
		}
	}
}

func shouldIgnore(name string, isDir bool) bool {
	switch name {
	case ".git", ".svn", ".hg", "node_modules", "vendor", "dist", "build", "coverage", "target", ".next", ".cache", "__pycache__":
		return true
	}
	if isDir && strings.HasPrefix(name, ".") {
		return true
	}
	return false
}
func walkFiles(root string, fn func(string, os.FileInfo) error) error {
	return filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if rel != "." {
			parts := strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' })
			for _, x := range parts {
				if shouldIgnore(x, info.IsDir()) {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
		}
		if info.IsDir() {
			return nil
		}
		return fn(p, info)
	})
}
func searchCode(query, scope string) (string, error) {
	root := workspace
	if scope != "" {
		root = safeJoin(workspace, scope)
	}
	var res []string
	err := walkFiles(root, func(p string, info os.FileInfo) error {
		if info.Size() > 2*1024*1024 {
			return nil
		}
		f, e := os.Open(p)
		if e != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 512*1024)
		ln := 0
		for sc.Scan() {
			ln++
			if strings.Contains(sc.Text(), query) {
				rel, _ := filepath.Rel(workspace, p)
				res = append(res, fmt.Sprintf("%s:%d:%s", rel, ln, truncate(strings.TrimSpace(sc.Text()), 240)))
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(res) == 0 {
		return "No matches found.", nil
	}
	return strings.Join(res, "\n"), nil
}
func findFiles(pattern, scope string) (string, error) {
	root := workspace
	if scope != "" {
		root = safeJoin(workspace, scope)
	}
	var res []string
	err := walkFiles(root, func(p string, info os.FileInfo) error {
		rel, _ := filepath.Rel(root, p)
		ok, _ := filepath.Match(pattern, rel)
		if !ok && strings.Contains(pattern, "**/") {
			ok, _ = filepath.Match(strings.TrimPrefix(pattern, "**/"), filepath.Base(rel))
		}
		if ok {
			r, _ := filepath.Rel(workspace, p)
			res = append(res, r)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(res)
	if len(res) == 0 {
		return "No files matched.", nil
	}
	return strings.Join(res, "\n"), nil
}
func dirTree(root, indent string) string {
	es, err := os.ReadDir(root)
	if err != nil {
		return err.Error()
	}
	var b strings.Builder
	for i, e := range es {
		if shouldIgnore(e.Name(), e.IsDir()) {
			continue
		}
		pre := indent + "├── "
		next := indent + "│   "
		if i == len(es)-1 {
			pre = indent + "└── "
			next = indent + "    "
		}
		b.WriteString(pre + e.Name() + "\n")
		if e.IsDir() {
			b.WriteString(dirTree(filepath.Join(root, e.Name()), next))
		}
	}
	return b.String()
}

func runCommandStreaming(ctx context.Context, command, timeoutStr string) (string, error) {
	timeout := 90
	if timeoutStr != "" {
		if n, e := strconv.Atoi(timeoutStr); e == nil && n > 0 {
			timeout = n
		}
	}
	ctx2, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx2, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx2, "sh", "-c", command)
	}
	cmd.Dir = workspace
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return "", err
	}
	var wg sync.WaitGroup
	var bufMu sync.Mutex
	var out bytes.Buffer
	copyPipe := func(r io.Reader, prefix string) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 16*1024), 512*1024)
		for sc.Scan() {
			line := sc.Text()
			fmt.Printf("%s%s%s\n", c(dim), prefix+line, c(reset))
			bufMu.Lock()
			out.WriteString(prefix + line + "\n")
			bufMu.Unlock()
		}
	}
	wg.Add(2)
	go copyPipe(stdout, "")
	go copyPipe(stderr, "[stderr] ")
	err := cmd.Wait()
	wg.Wait()
	if ctx2.Err() == context.DeadlineExceeded {
		return out.String(), fmt.Errorf("execution timed out (%d sec)", timeout)
	}
	if err != nil {
		return out.String() + fmt.Sprintf("Exit status: %v", err), nil
	}
	if out.Len() == 0 {
		return "(command executed successfully with no output)", nil
	}
	return out.String(), nil
}
func shellCapture(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workspace
	b, err := cmd.CombinedOutput()
	if err != nil {
		return string(b), fmt.Errorf("%s: %w", name, err)
	}
	s := string(b)
	if s == "" {
		s = "(none)"
	}
	return s, nil
}

// ---------------- Sessions ----------------
func sessionPath(name string) string { return filepath.Join(chatDir, filepath.Base(name)+".json") }
func saveCurrentSession(name string) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	created := time.Now()
	if old, err := os.ReadFile(sessionPath(name)); err == nil {
		var prior Session
		if json.Unmarshal(old, &prior) == nil && !prior.Created.IsZero() {
			created = prior.Created
		}
	}
	s := Session{Name: name, Messages: append([]Message{}, sessionMessages...), Mode: currentMode, Created: created, Updated: time.Now(), Workspace: workspace}
	b, _ := json.MarshalIndent(s, "", "  ")
	_ = os.WriteFile(sessionPath(name), b, 0600)
}
func sessionAutoName() string { return "session_" + time.Now().Format("20060102_150405") }
func handleSession(args []string) {
	if len(args) == 0 {
		listSessions()
		return
	}
	switch args[0] {
	case "save":
		name := sessionAutoName()
		if len(args) > 1 {
			name = args[1]
		}
		saveCurrentSession(name)
		fmt.Println("✅ Session saved:", name)
	case "load":
		if len(args) < 2 {
			fmt.Println("Usage: /session load <name>")
			return
		}
		loadSession(args[1])
	case "list":
		listSessions()
	default:
		fmt.Println("Usage: /session save|load|list [name]")
	}
}
func listSessions() {
	es, _ := os.ReadDir(chatDir)
	var names []string
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Println("ℹ️ No saved sessions.")
		return
	}
	fmt.Println("Saved sessions:")
	for _, n := range names {
		fmt.Println("  ", n)
	}
}
func resumeSession(args []string) {
	name := ""
	if len(args) > 0 {
		name = args[0]
	} else {
		es, _ := os.ReadDir(chatDir)
		sort.Slice(es, func(i, j int) bool { return es[i].Name() > es[j].Name() })
		for _, e := range es {
			if strings.HasSuffix(e.Name(), ".json") {
				name = strings.TrimSuffix(e.Name(), ".json")
				break
			}
		}
	}
	if name == "" {
		fmt.Println("ℹ️ No session found.")
		return
	}
	loadSession(name)
}
func loadSession(name string) {
	b, err := os.ReadFile(sessionPath(name))
	if err != nil {
		fmt.Println("❌", err)
		return
	}
	var s Session
	if err = json.Unmarshal(b, &s); err != nil {
		fmt.Println("❌", err)
		return
	}
	sessionMu.Lock()
	sessionMessages = s.Messages
	sessionMu.Unlock()
	currentSessionName = s.Name
	if s.Mode != "" {
		currentMode = s.Mode
	}
	if s.Workspace != "" {
		if i, e := os.Stat(s.Workspace); e == nil && i.IsDir() {
			workspace = s.Workspace
		}
	}
	fmt.Printf("✅ Resumed %s (%d messages).\n", name, len(sessionMessages))
}
func exportChat() {
	name := "export_" + time.Now().Format("20060102_150405") + ".md"
	p := filepath.Join(chatDir, name)
	var b strings.Builder
	b.WriteString("# NootyCLI Session\n\n")
	for _, m := range sessionMessages {
		switch m.Role {
		case "user":
			b.WriteString("**You:** " + m.Content + "\n\n")
		case "assistant":
			b.WriteString("**Nooty:** " + m.Content + "\n\n---\n\n")
		}
	}
	if err := os.WriteFile(p, []byte(b.String()), 0600); err != nil {
		fmt.Println("❌", err)
		return
	}
	fmt.Println("✅ Exported:", p)
}

// ---------------- Misc handlers ----------------
func handleWorkspace(a []string) {
	if len(a) == 0 || a[0] == "show" {
		fmt.Println("📁 Workspace:", formatPath(workspace))
		return
	}
	if a[0] == "set" && len(a) > 1 {
		p, _ := filepath.Abs(a[1])
		i, e := os.Stat(p)
		if e != nil || !i.IsDir() {
			fmt.Println("❌ Directory does not exist.")
			return
		}
		workspace = p
		config.Workspace = p
		saveConfig()
		fmt.Println("✅ Workspace set:", formatPath(p))
	}
}
func handleModel(a []string) {
	if len(a) == 0 || a[0] == "show" {
		fmt.Println("🤖 Model:", config.Model)
		return
	}
	if a[0] == "set" && len(a) > 1 {
		config.Model = a[1]
		saveConfig()
		fmt.Println("✅ Model:", config.Model)
		return
	}
	if a[0] == "list" {
		ms, e := fetchAvailableModels()
		if e != nil {
			fmt.Println("❌", e)
			return
		}
		for _, m := range ms {
			fmt.Println("•", m)
		}
	}
}
func handleConfig() {
	r := bufio.NewReader(os.Stdin)
	ask := func(label, current string) string {
		fmt.Printf("%s [%s]: ", label, current)
		x, _ := r.ReadString('\n')
		return strings.TrimSpace(x)
	}
	if x := ask("Provider endpoint", config.ProviderEndpoint); x != "" {
		config.ProviderEndpoint = x
	}
	if x := ask("API key", mask(config.APIKey)); x != "" && x != mask(config.APIKey) {
		config.APIKey = x
	}
	if x := ask("Model", config.Model); x != "" {
		config.Model = x
	}
	saveConfig()
	fmt.Println("✅ Configuration saved.")
}
func showDNS() {
	fmt.Println("🛡️ DNS Shield:")
	for _, d := range fallbackDNS {
		st := ""
		if d.Name == activeDNSName {
			st = " [ACTIVE]"
		}
		fmt.Printf("  %-22s %s%s\n", d.Name, d.Address, st)
	}
}
func runDoctor() {
	fmt.Println("🏥 Doctor")
	fmt.Println("Provider:", config.ProviderEndpoint)
	fmt.Println("Model:", config.Model)
	fmt.Println("DNS:", activeDNSName)
	ms, e := fetchAvailableModels()
	if e != nil {
		fmt.Println("Status: FAILED", e)
	} else {
		fmt.Printf("Status: OK (%d models)\n", len(ms))
	}
}
func handleMemory(a []string) {
	if len(a) == 0 || a[0] == "list" {
		for _, m := range memories {
			fmt.Printf("[%d] %s: %s\n", m.ID, m.Tag, m.Content)
		}
		return
	}
	switch a[0] {
	case "add":
		if len(a) > 1 {
			m := Memory{ID: len(memories) + 1, Tag: "context", Content: strings.Join(a[1:], " "), Added: time.Now().Format(time.RFC3339)}
			memories = append(memories, m)
			saveMemories()
			fmt.Println("✅ Saved memory", m.ID)
		}
	case "forget":
		if len(a) > 1 {
			id, _ := strconv.Atoi(a[1])
			var n []Memory
			for _, m := range memories {
				if m.ID != id {
					n = append(n, m)
				}
			}
			memories = n
			saveMemories()
		}
	}
}
func handleSafety(a []string) {
	if len(a) == 0 {
		fmt.Println("🛡️ Safety:", config.Safety)
		return
	}
	if a[0] == "strict" || a[0] == "balanced" {
		config.Safety = a[0]
		saveConfig()
		fmt.Println("✅ Safety:", config.Safety)
	}
}
func showHistory() {
	for _, m := range sessionMessages {
		fmt.Printf("%s: %s\n", m.Role, m.Content)
	}
}
func handleShellBang(command string) {
	fmt.Print("Execute? [Y/n]: ")
	r := bufio.NewReader(os.Stdin)
	x, _ := r.ReadString('\n')
	if strings.EqualFold(strings.TrimSpace(x), "n") {
		return
	}
	_, _ = runCommandStreaming(context.Background(), command, "90")
}
func fetchAvailableModels() ([]string, error) {
	resp, err := doWithFallback("GET", strings.TrimRight(config.ProviderEndpoint, "/")+"/models", nil, commonHeaders())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}
	var x struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.Unmarshal(b, &x); err != nil {
		return nil, err
	}
	var out []string
	for _, d := range x.Data {
		out = append(out, d.ID)
	}
	sort.Strings(out)
	return out, nil
}
func loadConfig() {
	b, e := os.ReadFile(configFile)
	if e != nil {
		config = Config{ProviderEndpoint: "https://api.openai.com/v1", APIKey: os.Getenv("OPENAI_API_KEY"), Model: "gpt-4o-mini", Safety: "strict", ContextBudget: 10000}
		if config.APIKey == "" {
			config.APIKey = os.Getenv("NOOTY_API_KEY")
		}
		return
	}
	_ = json.Unmarshal(b, &config)
	if config.ContextBudget <= 0 {
		config.ContextBudget = 10000
	}
	if config.APIKey == "" {
		config.APIKey = os.Getenv("OPENAI_API_KEY")
		if config.APIKey == "" {
			config.APIKey = os.Getenv("NOOTY_API_KEY")
		}
	}
}
func saveConfig() {
	b, _ := json.MarshalIndent(config, "", "  ")
	_ = os.WriteFile(configFile, b, 0600)
}
func loadMemories() {
	b, e := os.ReadFile(memFile)
	if e != nil {
		return
	}
	_ = json.Unmarshal(b, &memories)
}
func saveMemories() {
	b, _ := json.MarshalIndent(memories, "", "  ")
	_ = os.WriteFile(memFile, b, 0600)
}

func quickCommit() {
	diff, e := shellCapture(context.Background(), "git", "diff")
	if e != nil || strings.TrimSpace(diff) == "" {
		fmt.Println("ℹ️ No diff available.")
		return
	}
	msgs := []Message{{Role: "system", Content: "Generate only one Conventional Commit message based on this git diff. Format: type(scope): short imperative summary"}, {Role: "user", Content: diff}}
	s, e := getModelResponseTextWithOptions(msgs, false, nil)
	if e != nil {
		fmt.Println("❌", e)
		return
	}
	s = strings.TrimSpace(s)
	fmt.Println("Suggested commit:", s)
	fmt.Print("Apply git commit? [y/N]: ")
	r := bufio.NewReader(os.Stdin)
	v, _ := r.ReadString('\n')
	if !strings.EqualFold(strings.TrimSpace(v), "y") {
		return
	}
	_, e = runCommandStreaming(context.Background(), "git commit -am "+strconv.Quote(s), "90")
	if e != nil {
		fmt.Println("❌", e)
	}
}
