// nooty.go — NootyCLI v0.3.0 "Radin Pro" – Agentic Terminal Intelligence
// Single-file, zero external dependencies, cross-platform (macOS / Linux / Windows / WSL).
//
// 🚀 Compile & Build:
//   go build -ldflags="-s -w" -o nooty nooty.go
//
// 🛠 Commands:
//   /config          → Interactive configuration wizard
//   /model list      → Browse & select available models
//   /mode cli        → Switch to Autonomous Agent Mode
//   /dns             → View Anti-Sanction Smart DNS Shield chain
//   /doctor          → Run full network connection & provider diagnostic
//   /sessions        → List saved sessions
//   /resume <id>     → Resume a previous session
//   /undo            → Revert the last file write/delete via checkpoint
//
// What's new in v0.3.0:
//   1. True streaming responses in CLI (agent) mode + batched/parallel tool-call execution.
//   2. patch_file tool: search/replace based diffs instead of always rewriting whole files.
//   3. Token-budget aware context trimming with rolling summarization of older turns.
//   4. Session persistence to ~/.nooty/chats/*.json with /sessions and /resume.
//   5. Streaming run_command output, streaming line-based search_code, .gitignore-aware walking.
//   6. Retry with exponential backoff per-DNS-resolver in doWithFallback.
//   7. Checkpoint/undo system for write_file / patch_file / delete_file / create_file.

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- Cross-Platform ANSI Styling Engine ----------
var useColor = true

func init() {
	if runtime.GOOS == "windows" {
		_ = exec.Command("cmd", "/c", "color").Run()
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		useColor = false
	}
}

func c(code string) string {
	if useColor {
		return code
	}
	return ""
}

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

// ---------- Data Models ----------
type Config struct {
	ProviderEndpoint string `json:"provider_endpoint"`
	APIKey           string `json:"api_key"`
	Model            string `json:"model"`
	Safety           string `json:"safety"`
	Workspace        string `json:"workspace"`
}

type Memory struct {
	ID      int    `json:"id"`
	Tag     string `json:"tag"`
	Content string `json:"content"`
	Added   string `json:"added"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type ChatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

type ToolCall struct {
	Name string
	Args map[string]string
}

type DNSResolver struct {
	Name    string
	Address string
}

// Session is the persisted, on-disk representation of a chat/agent session.
type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

// Checkpoint records a pre-mutation snapshot of a file so it can be restored via /undo.
type Checkpoint struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Tool      string    `json:"tool"`
	RelPath   string    `json:"rel_path"`
	Existed   bool      `json:"existed"`   // whether the file existed before the op
	BackupRel string    `json:"backup_rel"` // path (inside checkpoints dir) holding the pre-op content
}

// ---------- Global State ----------
var (
	config          Config
	memories        []Memory
	sessionMessages []Message
	currentMode     = "chat" // "chat" or "cli"
	workspace       string
	homeDir         string
	nootyDir        string
	memFile         string
	configFile      string
	chatsDir        string
	checkpointsDir  string

	currentSessionID string

	fallbackDNS = []DNSResolver{
		{Name: "Direct Connection", Address: ""},
		{Name: "Electro DNS", Address: "78.157.42.100"},
		{Name: "Shecan DNS #1", Address: "178.22.122.100"},
		{Name: "Shecan DNS #2", Address: "185.51.200.2"},
		{Name: "Begzar DNS #1", Address: "185.55.226.26"},
		{Name: "Begzar DNS #2", Address: "185.55.225.25"},
	}
	activeDNSName = "Direct Connection"

	// Approximate token budget for context sent to the model.
	// (character-count / 4 heuristic — good enough without a real tokenizer)
	tokenBudget = 6000
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "NootyCLI v0.3.0 — Agentic Terminal Intelligence\n\nUsage:\n  nooty [options]\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	var err error
	homeDir, err = os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "⚠ Error: Cannot locate user home directory.")
		os.Exit(1)
	}

	nootyDir = filepath.Join(homeDir, ".nooty")
	chatsDir = filepath.Join(nootyDir, "chats")
	checkpointsDir = filepath.Join(nootyDir, "checkpoints")
	_ = os.MkdirAll(nootyDir, 0700)
	_ = os.MkdirAll(chatsDir, 0700)
	_ = os.MkdirAll(checkpointsDir, 0700)
	configFile = filepath.Join(nootyDir, "config.json")
	memFile = filepath.Join(nootyDir, "memories.json")

	loadConfig()
	loadMemories()

	if config.Workspace == "" {
		cwd, err := os.Getwd()
		if err == nil {
			config.Workspace = cwd
		} else {
			config.Workspace = homeDir
		}
	}
	workspace = config.Workspace

	currentSessionID = newSessionID()

	drawHeader()
	repl()
}

// ---------- Minimal Sleek Header ----------
func drawHeader() {
	width := 64
	line := strings.Repeat("─", width-2)

	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), centerText(" NOOTY CLI ", width-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), centerText("v0.3.0 Radin Pro — Agentic Terminal Intelligence", width-2), c(cyan), c(reset))
	fmt.Println(c(cyan) + "├" + line + "┤" + c(reset))

	prettyWorkspace := formatPath(workspace)

	entries := [][]string{
		{"Provider", truncateString(config.ProviderEndpoint, 38)},
		{"Model", config.Model},
		{"API Key", maskAPIKey(config.APIKey)},
		{"Workspace", truncateString(prettyWorkspace, 38)},
		{"DNS Shield", activeDNSName},
		{"Mode", strings.ToUpper(currentMode) + " Mode"},
	}

	for _, e := range entries {
		val := fmt.Sprintf("%-38s", e[1])
		fmt.Printf("%s│%s %-12s: %s%s %s│%s\n",
			c(cyan), c(bold)+c(white), e[0], c(green), val, c(cyan), c(reset))
	}
	fmt.Println(c(cyan) + "└" + line + "┘" + c(reset))
	fmt.Printf("%s💡 Type %s/help%s for options, %s/mode cli%s for Agent Mode.%s\n\n", c(dim), c(bold)+c(green), c(dim), c(bold)+c(cyan), c(dim), c(reset))
}

func formatPath(path string) string {
	if strings.HasPrefix(path, homeDir) {
		return "~" + strings.TrimPrefix(path, homeDir)
	}
	return path
}

func centerText(text string, width int) string {
	if len(text) >= width {
		return text[:width]
	}
	left := (width - len(text)) / 2
	right := width - len(text) - left
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "..." + s[len(s)-max+3:]
}

func maskAPIKey(key string) string {
	if key == "" {
		return "(not configured)"
	}
	if len(key) <= 8 {
		return key[:1] + strings.Repeat("*", len(key)-1)
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// ---------- Interactive REPL Engine ----------
func repl() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for {
		fmt.Print(prompt())
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			handleSlashCommand(line)
		} else if strings.HasPrefix(line, "!") && currentMode == "cli" {
			handleShellBang(line[1:])
		} else {
			handleChat(line)
		}
	}
	persistSession()
	fmt.Println(c(dim) + "\n👋 NootyCLI session ended. Goodbye!" + c(reset))
}

func prompt() string {
	if currentMode == "cli" {
		return c(bold) + c(cyan) + "🤖 nooty[agent]" + c(yellow) + " ❯ " + c(reset)
	}
	return c(bold) + c(green) + "⚡ nooty" + c(white) + " ❯ " + c(reset)
}

// ---------- Command Dispatcher ----------
func handleSlashCommand(cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "/help":
		printHelp()
	case "/mode":
		if len(parts) > 1 && parts[1] == "cli" {
			currentMode = "cli"
			fmt.Println(c(green) + "🛠 Switched to Agent Mode (Tool Execution Enabled)." + c(reset))
		} else {
			currentMode = "chat"
			fmt.Println(c(green) + "💬 Switched to Conversational Chat Mode." + c(reset))
		}
	case "/workspace":
		handleWorkspace(parts[1:])
	case "/model":
		handleModelCommand(parts[1:])
	case "/config":
		handleConfig()
	case "/dns":
		showDNSStatus()
	case "/doctor":
		runDoctor()
	case "/memory":
		handleMemory(parts[1:])
	case "/safety":
		handleSafety(parts[1:])
	case "/history":
		showHistory()
	case "/sessions":
		listSessions()
	case "/resume":
		resumeSession(parts[1:])
	case "/undo":
		handleUndo()
	case "/clear":
		persistSession()
		sessionMessages = nil
		currentSessionID = newSessionID()
		if runtime.GOOS == "windows" {
			cmd := exec.Command("cmd", "/c", "cls")
			cmd.Stdout = os.Stdout
			_ = cmd.Run()
		} else {
			fmt.Print("\033[H\033[2J")
		}
		drawHeader()
		fmt.Println(c(green) + "✨ Session history & screen cleared." + c(reset))
	case "/exit":
		persistSession()
		os.Exit(0)
	default:
		fmt.Printf("❌ Unknown command: %s. Type /help for assistance.\n", parts[0])
	}
}

func printHelp() {
	fmt.Println(c(bold) + "\n📌 NootyCLI Command Reference:" + c(reset))
	fmt.Println(`
  /help                        Show command help overview
  /mode [chat|cli]             Toggle Chat or Agentic CLI Execution Mode
  /config                      Interactive wizard to setup API key, endpoint & model
  /workspace show|set <path>   Manage current working directory
  /model show|set <name>|list  View, switch, or browse models interactively
  /dns                         Display Anti-Sanction Smart DNS Shield status
  /doctor                      Run full connection and API health check
  /memory list|add|forget      Manage long-term persistent agent context
  /safety strict|balanced      Set command safety confirmation policies
  /history                     Display conversation session log
  /sessions                    List saved sessions on disk
  /resume <id|latest>          Resume a previous saved session
  /undo                        Revert the last file write/delete checkpoint
  /clear                       Reset current screen & session memory
  /exit                        Terminate NootyCLI session

  💡 In Agent CLI Mode: Prefix commands with ! for direct shell execution.
  💡 In Agent CLI Mode: the model may issue several TOOL: calls in one reply;
     independent read-only tools run in parallel automatically.`)
}

func handleConfig() {
	fmt.Println(c(bold) + "\n⚙️ Nooty Configuration Wizard" + c(reset))
	fmt.Println("Press Enter to keep existing settings.")
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Provider endpoint [%s]: ", config.ProviderEndpoint)
	ep, _ := reader.ReadString('\n')
	ep = strings.TrimSpace(ep)
	if ep != "" {
		config.ProviderEndpoint = ep
	}

	fmt.Printf("API key [%s]: ", maskAPIKey(config.APIKey))
	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)
	if key != "" {
		config.APIKey = key
	}

	fmt.Printf("Model [%s]: ", config.Model)
	mod, _ := reader.ReadString('\n')
	mod = strings.TrimSpace(mod)
	if mod != "" {
		config.Model = mod
	}

	saveConfig()
	fmt.Println(c(green) + "✅ Configuration saved successfully!\n" + c(reset))
}

func showDNSStatus() {
	fmt.Println(c(bold) + "\n🛡️ Anti-Sanction Smart DNS Fallback Chain:" + c(reset))
	for i, dns := range fallbackDNS {
		status := ""
		if dns.Name == activeDNSName {
			status = c(green) + " [ACTIVE]" + c(reset)
		}
		if dns.Address == "" {
			fmt.Printf("  %d. %-24s (System Default)%s\n", i+1, dns.Name, status)
		} else {
			fmt.Printf("  %d. %-24s (%s)%s\n", i+1, dns.Name, dns.Address, status)
		}
	}
	fmt.Println()
}

func handleModelCommand(args []string) {
	if len(args) == 0 {
		fmt.Printf("🤖 Active Model: %s\n", config.Model)
		return
	}
	switch args[0] {
	case "show":
		fmt.Printf("🤖 Active Model: %s\n", config.Model)
	case "set":
		if len(args) < 2 {
			fmt.Println("Usage: /model set <model-name>")
			return
		}
		config.Model = args[1]
		saveConfig()
		fmt.Printf("✅ Model set to: %s\n", config.Model)
	case "list":
		selectModelInteractive()
	default:
		fmt.Println("❌ Unknown subcommand. Use: show | set | list")
	}
}

func selectModelInteractive() {
	fmt.Println("🔍 Fetching available models...")
	models, err := fetchAvailableModels()
	if err != nil {
		fmt.Printf("%s❌ Model list error: %v%s\n", c(red), err, c(reset))
		return
	}
	if len(models) == 0 {
		fmt.Println("⚠️ Provider returned zero models.")
		return
	}

	pageSize := 15
	totalPages := (len(models) + pageSize - 1) / pageSize
	page := 0

	for {
		fmt.Printf("\n%s📋 Available Provider Models (Page %d/%d):%s\n", c(bold), page+1, totalPages, c(reset))
		start := page * pageSize
		end := start + pageSize
		if end > len(models) {
			end = len(models)
		}

		for i, m := range models[start:end] {
			fmt.Printf("  %s[%2d]%s %s\n", c(bold)+c(cyan), start+i+1, c(reset), m)
		}

		fmt.Print("\nSelect number, [n]ext, [p]rev, or [q]uit: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "q":
			return
		case "n":
			if page < totalPages-1 {
				page++
			}
		case "p":
			if page > 0 {
				page--
			}
		default:
			num, err := strconv.Atoi(input)
			if err != nil || num < 1 || num > len(models) {
				fmt.Println("❌ Invalid selection.")
				continue
			}
			selected := models[num-1]
			config.Model = selected
			saveConfig()
			fmt.Printf("%s✅ Active Model updated to: %s%s\n", c(green), selected, c(reset))
			return
		}
	}
}

// ---------- Network Transport Engine ----------
func dnsDialer(dnsServer string) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{}
		return d.DialContext(ctx, network, dnsServer+":53")
	}
}

func httpClientForDNS(dns string) *http.Client {
	if dns == "" {
		return &http.Client{Timeout: 35 * time.Second}
	}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial:     dnsDialer(dns),
	}
	dialer := &net.Dialer{Resolver: resolver}
	return &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext},
		Timeout:   35 * time.Second,
	}
}

// isRetryableStatus reports whether an HTTP status code represents a transient
// failure worth retrying (5xx / 429) rather than a permanent one (4xx other than 429).
func isRetryableStatus(code int) bool {
	return code == 429 || (code >= 500 && code < 600)
}

// doRequestWithRetry performs a single resolver's request with exponential
// backoff retries for transient network errors or 5xx/429 responses.
func doRequestWithRetry(client *http.Client, method, url string, body []byte, headers map[string]string, maxAttempts int) (*http.Response, error) {
	var lastErr error
	backoff := 400 * time.Millisecond

	for attempt := 0; attempt < maxAttempts; attempt++ {
		var req *http.Request
		var err error
		if body != nil {
			req, err = http.NewRequest(method, url, bytes.NewReader(body))
		} else {
			req, err = http.NewRequest(method, url, nil)
		}
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err == nil {
			if !isRetryableStatus(resp.StatusCode) {
				return resp, nil
			}
			// transient status — close body, retry
			lastErr = fmt.Errorf("transient HTTP status %d", resp.StatusCode)
			_ = resp.Body.Close()
		} else {
			lastErr = err
		}

		if attempt < maxAttempts-1 {
			jitter := time.Duration(rand.Intn(150)) * time.Millisecond
			time.Sleep(backoff + jitter)
			backoff *= 2
		}
	}
	return nil, lastErr
}

func doWithFallback(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	const attemptsPerResolver = 3
	var lastErr error

	for i, dnsResolver := range fallbackDNS {
		client := httpClientForDNS(dnsResolver.Address)

		resp, err := doRequestWithRetry(client, method, url, body, headers, attemptsPerResolver)
		if err == nil && resp.StatusCode != 403 && resp.StatusCode != 451 {
			activeDNSName = dnsResolver.Name
			return resp, nil
		}

		if err != nil {
			lastErr = err
		} else if resp != nil {
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, dnsResolver.Name)
			_ = resp.Body.Close()
		}

		if i < len(fallbackDNS)-1 {
			fmt.Printf("%s⚠️ Connection issue via %s (%v). Bypassing via %s...%s\n",
				c(yellow), dnsResolver.Name, lastErr, fallbackDNS[i+1].Name, c(reset))
		}
	}
	return nil, fmt.Errorf("network connection failed after retries: all anti-sanction resolvers exhausted (last: %v)", lastErr)
}

func fetchAvailableModels() ([]string, error) {
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/models"
	headers := map[string]string{}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}

	resp, err := doWithFallback("GET", endpoint, nil, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse models payload")
	}

	var models []string
	for _, d := range result.Data {
		models = append(models, d.ID)
	}
	return models, nil
}

// ---------- Chat Execution ----------
func handleChat(input string) {
	messages := buildMessages(input)
	if currentMode == "cli" {
		runAgentLoop(messages)
		return
	}
	reply := streamResponse(messages)
	sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: reply})
	persistSession()
}

const systemPromptBase = `You are NootyCLI, an autonomous agentic terminal AI assistant.

When in CHAT mode: Provide concise, expert terminal and software engineering responses.

When in CLI mode: You act as an autonomous workspace agent.
To execute tools, reply using one or more lines of this exact syntax:
TOOL: tool_name key1="value1" key2="value2"

You MAY issue multiple TOOL: lines in a single reply when the actions are
independent of each other (for example several read_file calls, or a
read_file plus a git_status). Independent read-only tools will be executed
in parallel. If a later tool call depends on the result of an earlier one,
only issue the first one and wait for its output.

Available Workspace Tools:
- list_files (path="relative_path")
- tree (path="relative_path")
- read_file (path="relative_path")
- write_file (path="relative_path", content="full_content")
- create_file (path="relative_path", content="initial_content")
- patch_file (path="relative_path", search="exact_text_to_find", replace="replacement_text")
  Use patch_file instead of write_file whenever you are changing part of an
  existing file — it is faster and cheaper than rewriting the whole file.
  "search" must match exactly once in the file.
- delete_file (path="relative_path")
- search_code (query="text", path="relative_path")
- file_info (path="relative_path")
- git_status
- git_diff
- run_command (command="shell_cmd", timeout="seconds")

IMPORTANT: Use EXACT tool format shown above.`

func buildMessages(userInput string) []Message {
	var msgs []Message
	sysPrompt := systemPromptBase

	relevant := getRelevantMemories(userInput)
	if len(relevant) > 0 {
		sysPrompt += "\n\nUser Context & Memories:\n"
		for _, m := range relevant {
			sysPrompt += fmt.Sprintf("- [%s] %s\n", m.Tag, m.Content)
		}
	}

	msgs = append(msgs, Message{Role: "system", Content: sysPrompt})
	msgs = append(msgs, budgetHistory(sessionMessages, tokenBudget)...)

	userMsg := Message{Role: "user", Content: userInput}
	msgs = append(msgs, userMsg)
	sessionMessages = append(sessionMessages, userMsg)
	persistSession()

	return msgs
}

// ---------- Context Management (token-budget aware) ----------

// estimateTokens is a cheap heuristic: ~4 chars per token, no real tokenizer needed.
func estimateTokens(s string) int {
	n := len(s) / 4
	if n < 1 {
		n = 1
	}
	return n
}

func messageTokens(m Message) int {
	return estimateTokens(m.Content) + 4 // small per-message overhead
}

// budgetHistory returns as much recent history as fits in the token budget.
// Anything older that doesn't fit is collapsed into a single synthetic
// "summary" system-ish message (role=user, clearly marked) instead of being
// silently dropped, so the model keeps some notion of earlier context.
func budgetHistory(history []Message, budget int) []Message {
	if len(history) == 0 {
		return nil
	}

	used := 0
	cut := len(history)
	for i := len(history) - 1; i >= 0; i-- {
		t := messageTokens(history[i])
		if used+t > budget {
			cut = i + 1
			break
		}
		used += t
		cut = i
	}

	if cut == 0 {
		return history
	}

	older := history[:cut]
	recent := history[cut:]

	summary := summarizeMessages(older)
	if summary == "" {
		return recent
	}

	summaryMsg := Message{
		Role:    "user",
		Content: "[Context summary of " + strconv.Itoa(len(older)) + " earlier turn(s), condensed to save space]\n" + summary,
	}
	out := make([]Message, 0, len(recent)+1)
	out = append(out, summaryMsg)
	out = append(out, recent...)
	return out
}

// summarizeMessages produces a lightweight extractive summary: one short
// line per message, truncated, rather than dropping the content entirely.
func summarizeMessages(msgs []Message) string {
	if len(msgs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range msgs {
		line := strings.ReplaceAll(strings.TrimSpace(m.Content), "\n", " ")
		if len(line) > 160 {
			line = line[:160] + "…"
		}
		if line == "" {
			continue
		}
		b.WriteString("- (" + m.Role + ") " + line + "\n")
	}
	return b.String()
}

func getRelevantMemories(query string) []Memory {
	q := strings.ToLower(query)
	var res []Memory
	for _, m := range memories {
		if strings.Contains(strings.ToLower(m.Content), q) || strings.Contains(strings.ToLower(m.Tag), q) {
			res = append(res, m)
		}
	}
	if len(res) > 5 {
		res = res[:5]
	}
	return res
}

// streamResponse streams a chat completion to stdout and returns the full text.
func streamResponse(messages []Message) string {
	reqPayload := ChatRequest{Model: config.Model, Messages: messages, Stream: true}
	jsonData, _ := json.Marshal(reqPayload)
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
	headers := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}

	resp, err := doWithFallback("POST", endpoint, jsonData, headers)
	if err != nil {
		fmt.Printf("%s❌ Request error: %v%s\n", c(red), err, c(reset))
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("%s❌ Provider Error %d: %s%s\n", c(red), resp.StatusCode, string(body), c(reset))
		return ""
	}

	reader := bufio.NewReader(resp.Body)
	var fullContent strings.Builder
	fmt.Print(c(cyan))

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk ChatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err == nil {
			for _, choice := range chunk.Choices {
				fmt.Print(choice.Delta.Content)
				fullContent.WriteString(choice.Delta.Content)
			}
		}
	}

	fmt.Print(c(reset) + "\n\n")
	return fullContent.String()
}

// ---------- Agentic Plan & Execute Loop ----------
func runAgentLoop(messages []Message) {
	planPrompt := append(append([]Message{}, messages...), Message{Role: "user", Content: "Provide a clear, numbered execution plan to fulfill this request."})
	fmt.Print(c(yellow) + "🤔 Analyzing & planning action sequence...\n" + c(reset))

	planText := streamResponse(planPrompt)
	if planText == "" {
		fmt.Printf("%s❌ Planning failed (empty response).%s\n", c(red), c(reset))
		return
	}

	fmt.Print(c(bold) + "\nApprove plan execution? [Y/n]: " + c(reset))
	reader := bufio.NewReader(os.Stdin)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))
	if confirm == "n" || confirm == "no" {
		fmt.Println("🛑 Execution cancelled by user.")
		return
	}

	msgs := append(append([]Message{}, messages...),
		Message{Role: "assistant", Content: planText},
		Message{Role: "user", Content: "Plan approved. Proceed step by step using TOOL commands. You may batch independent TOOL: calls in one reply."},
	)

	for i := 0; i < 10; i++ {
		fmt.Print(c(dim) + "⏳ Thinking...\n" + c(reset))
		respText := streamResponse(msgs)
		if respText == "" {
			fmt.Printf("%s❌ Agent Execution Error: empty model response%s\n", c(red), c(reset))
			return
		}

		toolCalls := extractToolCalls(respText)
		if len(toolCalls) == 0 {
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText})
			persistSession()
			return
		}

		results := executeToolCallsBatch(toolCalls)

		var toolReport strings.Builder
		for idx, tc := range toolCalls {
			res := results[idx]
			label := res.Output
			if len(label) > 2500 {
				label = label[:2500] + "\n... (output truncated)"
			}
			toolReport.WriteString(fmt.Sprintf("Tool '%s' output:\n%s\n\n", tc.Name, label))
		}

		msgs = append(msgs, Message{Role: "assistant", Content: respText}, Message{Role: "user", Content: toolReport.String()})
	}
	fmt.Printf("%s⚠️ Agent loop step limit reached (10 steps).%s\n", c(yellow), c(reset))
}

// getModelResponseText performs a non-streaming completion (used for legacy/internal calls).
func getModelResponseText(messages []Message) (string, error) {
	reqPayload := ChatRequest{Model: config.Model, Messages: messages, Stream: false}
	jsonData, _ := json.Marshal(reqPayload)
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
	headers := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}

	resp, err := doWithFallback("POST", endpoint, jsonData, headers)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty choices array in response")
	}

	return result.Choices[0].Message.Content, nil
}

// ---------- Tool-call parsing (supports multiple TOOL: lines per reply) ----------

func extractToolCalls(text string) []*ToolCall {
	var calls []*ToolCall
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "TOOL:") || strings.HasPrefix(trimmed, "TOOL：") {
			if tc := parseToolLine(trimmed); tc != nil {
				calls = append(calls, tc)
			}
		}
	}
	if len(calls) > 0 {
		return calls
	}

	re := regexp.MustCompile(`(?i)TOOL:\s*(\w+)\s+(.*)`)
	matches := re.FindStringSubmatch(text)
	if len(matches) >= 3 {
		return []*ToolCall{parseToolArgs(matches[1], matches[2])}
	}
	return nil
}

func parseToolLine(line string) *ToolCall {
	line = strings.TrimPrefix(line, "TOOL:")
	line = strings.TrimPrefix(line, "TOOL：")
	line = strings.TrimSpace(line)

	parts := strings.SplitN(line, " ", 2)
	if len(parts) < 1 || parts[0] == "" {
		return nil
	}
	name := parts[0]
	argsStr := ""
	if len(parts) > 1 {
		argsStr = parts[1]
	}
	return parseToolArgs(name, argsStr)
}

func parseToolArgs(name, argsStr string) *ToolCall {
	args := map[string]string{}
	re := regexp.MustCompile(`(\w+)=("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|\S+)`)
	matches := re.FindAllStringSubmatch(argsStr, -1)

	for _, match := range matches {
		if len(match) == 3 {
			key := match[1]
			val := match[2]
			if (strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`)) || (strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				val = val[1 : len(val)-1]
			}
			val = strings.ReplaceAll(val, "\\n", "\n")
			val = strings.ReplaceAll(val, "\\t", "\t")
			args[key] = val
		}
	}

	if len(args) == 0 && strings.TrimSpace(argsStr) != "" {
		args["path"] = strings.TrimSpace(argsStr)
	}

	return &ToolCall{Name: name, Args: args}
}

// ---------- Batched / parallel tool execution ----------

type toolResult struct {
	Output   string
	Approved bool
}

// readOnlyTools never mutate state and are safe to run concurrently.
var readOnlyTools = map[string]bool{
	"list_files": true, "tree": true, "read_file": true,
	"search_code": true, "file_info": true, "git_status": true, "git_diff": true,
}

// executeToolCallsBatch runs a slice of tool calls. Consecutive read-only
// calls are executed in parallel via goroutines; any call that mutates state
// (write_file, patch_file, create_file, delete_file, run_command) is executed
// serially and safely, in order, since later calls may depend on it.
func executeToolCallsBatch(calls []*ToolCall) []toolResult {
	results := make([]toolResult, len(calls))

	i := 0
	for i < len(calls) {
		if readOnlyTools[calls[i].Name] {
			// gather a contiguous run of read-only calls and run them in parallel
			j := i
			for j < len(calls) && readOnlyTools[calls[j].Name] {
				j++
			}
			var wg sync.WaitGroup
			for k := i; k < j; k++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					printToolHeader(idx+1, calls[idx])
					out, err := runTool(calls[idx].Name, calls[idx].Args)
					if err != nil {
						results[idx] = toolResult{Output: fmt.Sprintf("Tool Error: %v", err), Approved: true}
						return
					}
					results[idx] = toolResult{Output: out, Approved: true}
				}(k)
			}
			wg.Wait()
			for k := i; k < j; k++ {
				fmt.Printf("%s📄 [%s] Output:%s\n%s\n", c(dim), calls[k].Name, c(reset), truncateForDisplay(results[k].Output))
			}
			i = j
			continue
		}

		// mutating / sequential tool
		printToolHeader(i+1, calls[i])
		out, approved := executeAgentTool(calls[i])
		results[i] = toolResult{Output: out, Approved: approved}
		fmt.Printf("%s📄 [%s] Output:%s\n%s\n", c(dim), calls[i].Name, c(reset), truncateForDisplay(out))
		i++
	}

	return results
}

func printToolHeader(step int, tc *ToolCall) {
	fmt.Printf("\n%s🔧 Agent Action [%d]: %s%s\n", c(bold)+c(yellow), step, tc.Name, c(reset))
	for k, v := range tc.Args {
		disp := v
		if len(disp) > 120 {
			disp = disp[:120] + "…"
		}
		fmt.Printf("   %s%s%s: %s\n", c(dim), k, c(reset), disp)
	}
}

func truncateForDisplay(s string) string {
	if len(s) > 2500 {
		return s[:2500] + "\n... (output truncated)"
	}
	return s
}

func executeAgentTool(tc *ToolCall) (string, bool) {
	needsApproval := true
	switch tc.Name {
	case "list_files", "tree", "read_file", "search_code", "file_info", "git_status", "git_diff":
		needsApproval = false
	}

	if needsApproval {
		if tc.Name == "delete_file" {
			fmt.Printf("%s⚠️ SAFETY WARNING: %s will permanently delete target file!%s\n", c(red), tc.Name, c(reset))
			fmt.Print("Type DELETE to confirm action: ")
			reader := bufio.NewReader(os.Stdin)
			confirm, _ := reader.ReadString('\n')
			if strings.TrimSpace(confirm) != "DELETE" {
				return "Operation aborted by safety check.", false
			}
		} else {
			fmt.Print("Confirm execution? [Y/n]: ")
			reader := bufio.NewReader(os.Stdin)
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(strings.ToLower(confirm))
			if confirm == "n" || confirm == "no" {
				return "Operation cancelled by user.", false
			}
		}
	}

	result, err := runTool(tc.Name, tc.Args)
	if err != nil {
		return fmt.Sprintf("Tool Error: %v", err), true
	}
	return result, true
}

// ---------- .gitignore-aware walking ----------

// ignoreDefaults are directory/file names always skipped during tree/search_code walks.
var ignoreDefaults = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".hg": true,
	".svn": true, ".idea": true, ".vscode": true, "dist": true,
	"build": true, "__pycache__": true, ".venv": true, "venv": true,
	".DS_Store": true,
}

// loadGitignore reads a .gitignore at the workspace root (best-effort, simple
// glob-style matching — not a full gitignore spec implementation) and returns
// a matcher function.
func loadGitignorePatterns(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return nil
	}
	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, strings.Trim(line, "/"))
	}
	return patterns
}

func matchesGitignore(name string, patterns []string) bool {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if ok, _ := filepath.Match(p, name); ok {
			return true
		}
	}
	return false
}

func shouldSkipEntry(name string, patterns []string) bool {
	if ignoreDefaults[name] {
		return true
	}
	if strings.HasPrefix(name, ".") && name != "." && name != ".." {
		return true
	}
	return matchesGitignore(name, patterns)
}

func runTool(name string, args map[string]string) (string, error) {
	switch name {
	case "list_files":
		path := workspace
		if p, ok := args["path"]; ok && p != "" && p != "." {
			path = safeJoin(workspace, p)
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return "", err
		}
		patterns := loadGitignorePatterns(workspace)
		var names []string
		for _, e := range entries {
			if shouldSkipEntry(e.Name(), patterns) {
				continue
			}
			if e.IsDir() {
				names = append(names, e.Name()+"/")
			} else {
				names = append(names, e.Name())
			}
		}
		if len(names) == 0 {
			return "(directory empty or fully ignored)", nil
		}
		return strings.Join(names, "\n"), nil

	case "tree":
		path := workspace
		if p, ok := args["path"]; ok && p != "" && p != "." {
			path = safeJoin(workspace, p)
		}
		patterns := loadGitignorePatterns(workspace)
		return dirTree(path, "", patterns), nil

	case "read_file":
		path := safeJoin(workspace, args["path"])
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil

	case "write_file", "create_file":
		path := safeJoin(workspace, args["path"])
		content := args["content"]
		saveCheckpoint(name, args["path"], path)
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ File saved (%d bytes): %s  [checkpoint saved — use /undo to revert]", len(content), args["path"]), nil

	case "patch_file":
		path := safeJoin(workspace, args["path"])
		search := args["search"]
		replace := args["replace"]
		if search == "" {
			return "", fmt.Errorf("patch_file requires a non-empty 'search' argument")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		original := string(data)
		count := strings.Count(original, search)
		if count == 0 {
			return "", fmt.Errorf("patch_file: search text not found in %s", args["path"])
		}
		if count > 1 {
			return "", fmt.Errorf("patch_file: search text matches %d times in %s; must match exactly once — provide more surrounding context", count, args["path"])
		}
		saveCheckpoint("patch_file", args["path"], path)
		updated := strings.Replace(original, search, replace, 1)
		if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
			return "", err
		}
		delta := len(updated) - len(original)
		return fmt.Sprintf("✅ Patched %s (%+d bytes)  [checkpoint saved — use /undo to revert]", args["path"], delta), nil

	case "delete_file":
		path := safeJoin(workspace, args["path"])
		saveCheckpoint("delete_file", args["path"], path)
		if err := os.Remove(path); err != nil {
			return "", err
		}
		return "✅ File removed: " + args["path"] + "  [checkpoint saved — use /undo to revert]", nil

	case "search_code":
		query := args["query"]
		scope := workspace
		if s, ok := args["path"]; ok && s != "" {
			scope = safeJoin(workspace, s)
		}
		return searchCodeStreaming(scope, query), nil

	case "file_info":
		path := safeJoin(workspace, args["path"])
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Path: %s\nSize: %d bytes\nMode: %s\nModTime: %s", path, info.Size(), info.Mode(), info.ModTime().Format(time.RFC3339)), nil

	case "git_status":
		cmd := exec.Command("git", "status", "--short")
		cmd.Dir = workspace
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git status error: %v", err)
		}
		s := string(out)
		if s == "" {
			s = "(working directory clean)"
		}
		return s, nil

	case "git_diff":
		cmd := exec.Command("git", "diff")
		cmd.Dir = workspace
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git diff error: %v", err)
		}
		s := string(out)
		if s == "" {
			s = "(no uncommitted changes)"
		}
		return s, nil

	case "run_command":
		return runCommandStreaming(args)
	}
	return "", fmt.Errorf("unknown tool: %s", name)
}

// searchCodeStreaming performs a line-by-line, low-memory grep-style search
// (bufio.Scanner instead of reading whole files into memory) and skips
// ignored directories/files.
func searchCodeStreaming(scope, query string) string {
	patterns := loadGitignorePatterns(workspace)
	var results []string

	_ = filepath.Walk(scope, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		name := info.Name()
		if info.IsDir() {
			if p != scope && shouldSkipEntry(name, patterns) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipEntry(name, patterns) {
			return nil
		}
		if info.Size() > 5_000_000 {
			return nil
		}

		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		matched := false
		for scanner.Scan() {
			lineNo++
			if strings.Contains(scanner.Text(), query) {
				matched = true
				break
			}
		}
		if matched {
			rel, _ := filepath.Rel(workspace, p)
			results = append(results, fmt.Sprintf("%s:%d", rel, lineNo))
		}
		return nil
	})

	if len(results) == 0 {
		return "No matches found."
	}
	return strings.Join(results, "\n")
}

// runCommandStreaming runs a shell command, streaming stdout/stderr to the
// terminal live (useful for long build/test commands) while also capturing
// the output to return to the model.
func runCommandStreaming(args map[string]string) (string, error) {
	cmdStr := args["command"]
	timeout := 60
	if t, ok := args["timeout"]; ok {
		_, _ = fmt.Sscanf(t, "%d", &timeout)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", cmdStr)
	} else {
		cmd = exec.Command("sh", "-c", cmdStr)
	}
	cmd.Dir = workspace

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	var captured bytes.Buffer
	var mu sync.Mutex
	var wg sync.WaitGroup

	streamCopy := func(prefix string, r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Printf("%s%s│%s %s\n", c(dim), prefix, c(reset), line)
			mu.Lock()
			captured.WriteString(line)
			captured.WriteString("\n")
			mu.Unlock()
		}
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	wg.Add(2)
	go streamCopy("out", stdoutPipe)
	go streamCopy("err", stderrPipe)

	done := make(chan error, 1)
	go func() {
		wg.Wait()
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		out := captured.String()
		if err != nil {
			return out + fmt.Sprintf("\nExit status: %v", err), nil
		}
		if out == "" {
			out = "(command executed successfully with no output)"
		}
		return out, nil
	case <-time.After(time.Duration(timeout) * time.Second):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("execution timed out (%d sec)", timeout)
	}
}

func dirTree(root, indent string, patterns []string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err.Error()
	}
	var filtered []os.DirEntry
	for _, e := range entries {
		if !shouldSkipEntry(e.Name(), patterns) {
			filtered = append(filtered, e)
		}
	}
	var out string
	for i, e := range filtered {
		prefix := indent + "├── "
		childIndent := indent + "│   "
		if i == len(filtered)-1 {
			prefix = indent + "└── "
			childIndent = indent + "    "
		}
		out += prefix + e.Name() + "\n"
		if e.IsDir() {
			out += dirTree(filepath.Join(root, e.Name()), childIndent, patterns)
		}
	}
	return out
}

func safeJoin(base, rel string) string {
	abs := filepath.Join(base, rel)
	abs = filepath.Clean(abs)
	if !strings.HasPrefix(abs, base) {
		return base
	}
	return abs
}

// ---------- Checkpoint / Undo System ----------

func checkpointIndexFile() string {
	return filepath.Join(checkpointsDir, "index.json")
}

func loadCheckpoints() []Checkpoint {
	data, err := os.ReadFile(checkpointIndexFile())
	if err != nil {
		return nil
	}
	var list []Checkpoint
	_ = json.Unmarshal(data, &list)
	return list
}

func saveCheckpoints(list []Checkpoint) {
	data, _ := json.MarshalIndent(list, "", "  ")
	_ = os.WriteFile(checkpointIndexFile(), data, 0600)
}

// saveCheckpoint snapshots the pre-mutation state of a file before
// write_file / patch_file / delete_file / create_file touches it.
func saveCheckpoint(tool, relPath, absPath string) {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	existed := false
	backupRel := ""

	if data, err := os.ReadFile(absPath); err == nil {
		existed = true
		backupRel = id + ".bak"
		_ = os.WriteFile(filepath.Join(checkpointsDir, backupRel), data, 0600)
	}

	cp := Checkpoint{
		ID:        id,
		Timestamp: time.Now(),
		Tool:      tool,
		RelPath:   relPath,
		Existed:   existed,
		BackupRel: backupRel,
	}

	list := loadCheckpoints()
	list = append(list, cp)
	// keep only the most recent 50 checkpoints to bound disk usage
	if len(list) > 50 {
		removed := list[:len(list)-50]
		list = list[len(list)-50:]
		for _, r := range removed {
			if r.BackupRel != "" {
				_ = os.Remove(filepath.Join(checkpointsDir, r.BackupRel))
			}
		}
	}
	saveCheckpoints(list)
}

func handleUndo() {
	list := loadCheckpoints()
	if len(list) == 0 {
		fmt.Println(c(yellow) + "⚠️ No checkpoints available to undo." + c(reset))
		return
	}
	last := list[len(list)-1]
	absPath := safeJoin(workspace, last.RelPath)

	if last.Existed {
		data, err := os.ReadFile(filepath.Join(checkpointsDir, last.BackupRel))
		if err != nil {
			fmt.Printf("%s❌ Undo failed: could not read backup: %v%s\n", c(red), err, c(reset))
			return
		}
		if err := os.WriteFile(absPath, data, 0644); err != nil {
			fmt.Printf("%s❌ Undo failed: could not restore file: %v%s\n", c(red), err, c(reset))
			return
		}
		fmt.Printf("%s✅ Restored %s to its state before '%s' (checkpoint %s).%s\n", c(green), last.RelPath, last.Tool, last.ID, c(reset))
	} else {
		// file did not exist before the op (e.g. create_file) — undo means delete it
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("%s❌ Undo failed: could not remove file: %v%s\n", c(red), err, c(reset))
			return
		}
		fmt.Printf("%s✅ Removed %s (it was created by '%s', checkpoint %s).%s\n", c(green), last.RelPath, last.Tool, last.ID, c(reset))
	}

	if last.BackupRel != "" {
		_ = os.Remove(filepath.Join(checkpointsDir, last.BackupRel))
	}
	list = list[:len(list)-1]
	saveCheckpoints(list)
}

// ---------- Direct Shell Command Engine ----------
func handleShellBang(cmd string) {
	cmd = strings.TrimSpace(cmd)
	fmt.Printf("\n%s⚡ Direct Shell Command:%s %s\n", c(yellow), c(reset), cmd)
	fmt.Print("Execute? [Y/n]: ")
	reader := bufio.NewReader(os.Stdin)
	resp, _ := reader.ReadString('\n')
	resp = strings.TrimSpace(strings.ToLower(resp))
	if resp != "n" && resp != "no" {
		executeShell(cmd)
	} else {
		fmt.Println("Cancelled.")
	}
}

func executeShell(command string) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Dir = workspace
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("%s❌ Command failed: %v%s\n", c(red), err, c(reset))
	}
}

// ---------- Utilities & Handlers ----------
func handleWorkspace(args []string) {
	if len(args) == 0 {
		fmt.Printf("📁 Workspace: %s\n", formatPath(workspace))
		return
	}
	switch args[0] {
	case "show":
		fmt.Printf("📁 Workspace: %s\n", formatPath(workspace))
	case "set":
		if len(args) < 2 {
			fmt.Println("Usage: /workspace set <path>")
			return
		}
		path, _ := filepath.Abs(args[1])
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			fmt.Println("❌ Directory does not exist.")
			return
		}
		workspace = path
		config.Workspace = path
		saveConfig()
		fmt.Printf("✅ Workspace set to: %s\n", formatPath(workspace))
	default:
		fmt.Println("❌ Subcommand unknown. Use: show | set <path>")
	}
}

func runDoctor() {
	fmt.Println(c(bold) + "\n🏥 NootyCLI Diagnostic Doctor" + c(reset))
	fmt.Printf("• Provider Endpoint : %s\n", config.ProviderEndpoint)
	fmt.Printf("• Active Model      : %s\n", config.Model)
	fmt.Printf("• API Key           : %s\n", maskAPIKey(config.APIKey))
	fmt.Printf("• Active Workspace  : %s\n", formatPath(workspace))
	fmt.Printf("• Saved Sessions    : %d\n", len(listSessionFiles()))
	fmt.Printf("• Checkpoints       : %d\n", len(loadCheckpoints()))
	fmt.Print("• Provider Status   : ")

	models, err := fetchAvailableModels()
	if err != nil {
		fmt.Printf("%sFAILED (%v)%s\n\n", c(red), err, c(reset))
	} else {
		fmt.Printf("%sOK (%d models accessible via %s)%s\n\n", c(green), len(models), activeDNSName, c(reset))
	}
}

func handleMemory(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: /memory list | add <text> | forget <id>")
		return
	}
	switch args[0] {
	case "list":
		if len(memories) == 0 {
			fmt.Println("🧠 No persistent memories found.")
			return
		}
		fmt.Println(c(bold) + "\n🧠 Persistent Memories:" + c(reset))
		for _, m := range memories {
			fmt.Printf("  [%d] (%s) %s\n", m.ID, m.Tag, m.Content)
		}
		fmt.Println()
	case "add":
		if len(args) < 2 {
			fmt.Println("Usage: /memory add <text>")
			return
		}
		text := strings.Join(args[1:], " ")
		m := Memory{
			ID:      len(memories) + 1,
			Tag:     "context",
			Content: text,
			Added:   time.Now().Format(time.RFC3339),
		}
		memories = append(memories, m)
		saveMemories()
		fmt.Printf("✅ Saved memory ID [%d]\n", m.ID)
	case "forget":
		if len(args) < 2 {
			fmt.Println("Usage: /memory forget <id>")
			return
		}
		id, _ := strconv.Atoi(args[1])
		var newMem []Memory
		for _, m := range memories {
			if m.ID != id {
				newMem = append(newMem, m)
			}
		}
		memories = newMem
		saveMemories()
		fmt.Printf("✅ Memory [%d] removed.\n", id)
	}
}

func handleSafety(args []string) {
	if len(args) == 0 {
		fmt.Printf("🛡️ Safety Mode: %s\n", config.Safety)
		return
	}
	if args[0] == "strict" || args[0] == "balanced" {
		config.Safety = args[0]
		saveConfig()
		fmt.Printf("✅ Safety updated to %s.\n", config.Safety)
	} else {
		fmt.Println("Usage: /safety strict|balanced")
	}
}

func showHistory() {
	if len(sessionMessages) == 0 {
		fmt.Println("💬 History clean.")
		return
	}
	fmt.Println(c(bold) + "\n📜 Session Log:" + c(reset))
	for _, msg := range sessionMessages {
		role := "👤 User"
		if msg.Role == "assistant" {
			role = "🤖 Nooty"
		}
		fmt.Printf("%s%s:%s %s\n", c(bold), role, c(reset), msg.Content)
	}
	fmt.Println()
}

// ---------- Session Persistence ----------

func newSessionID() string {
	return time.Now().Format("20060102-150405")
}

func sessionFilePath(id string) string {
	return filepath.Join(chatsDir, id+".json")
}

// persistSession writes the current in-memory session to disk. Cheap enough
// to call after every turn; last-write-wins is fine for a single-user CLI.
func persistSession() {
	if len(sessionMessages) == 0 {
		return
	}
	title := sessionTitle()
	sess := Session{
		ID:        currentSessionID,
		Title:     title,
		CreatedAt: sessionCreatedAt(),
		UpdatedAt: time.Now(),
		Messages:  sessionMessages,
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(sessionFilePath(currentSessionID), data, 0600)
}

func sessionCreatedAt() time.Time {
	if existing, err := os.ReadFile(sessionFilePath(currentSessionID)); err == nil {
		var s Session
		if json.Unmarshal(existing, &s) == nil && !s.CreatedAt.IsZero() {
			return s.CreatedAt
		}
	}
	return time.Now()
}

func sessionTitle() string {
	for _, m := range sessionMessages {
		if m.Role == "user" {
			t := strings.TrimSpace(strings.ReplaceAll(m.Content, "\n", " "))
			if len(t) > 60 {
				t = t[:60] + "…"
			}
			return t
		}
	}
	return "(empty session)"
}

func listSessionFiles() []string {
	entries, err := os.ReadDir(chatsDir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files
}

func listSessions() {
	files := listSessionFiles()
	if len(files) == 0 {
		fmt.Println("📭 No saved sessions found.")
		return
	}
	fmt.Println(c(bold) + "\n🗂  Saved Sessions:" + c(reset))
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(chatsDir, f))
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal(data, &s) != nil {
			continue
		}
		marker := ""
		if s.ID == currentSessionID {
			marker = c(green) + " [current]" + c(reset)
		}
		fmt.Printf("  %s%-20s%s %s  (%d msgs, updated %s)%s\n",
			c(cyan), s.ID, c(reset), s.Title, len(s.Messages), s.UpdatedAt.Format("2006-01-02 15:04"), marker)
	}
	fmt.Println(c(dim) + "\nUse /resume <id> or /resume latest to continue one." + c(reset))
}

func resumeSession(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: /resume <session-id> | /resume latest")
		return
	}
	id := args[0]
	files := listSessionFiles()
	if len(files) == 0 {
		fmt.Println("📭 No saved sessions found.")
		return
	}

	var target string
	if id == "latest" {
		latest := files[len(files)-1]
		target = strings.TrimSuffix(latest, ".json")
	} else {
		target = id
	}

	data, err := os.ReadFile(sessionFilePath(target))
	if err != nil {
		fmt.Printf("%s❌ Session '%s' not found.%s\n", c(red), target, c(reset))
		return
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		fmt.Printf("%s❌ Failed to parse session '%s'.%s\n", c(red), target, c(reset))
		return
	}

	persistSession() // save current before switching
	sessionMessages = s.Messages
	currentSessionID = s.ID
	fmt.Printf("%s✅ Resumed session %s (%d messages) — \"%s\"%s\n", c(green), s.ID, len(s.Messages), s.Title, c(reset))
}

// ---------- Persistence Methods ----------
func loadConfig() {
	data, err := os.ReadFile(configFile)
	if err != nil {
		config = Config{
			ProviderEndpoint: "https://api.openai.com/v1",
			APIKey:           os.Getenv("OPENAI_API_KEY"),
			Model:            "gpt-4o-mini",
			Safety:           "strict",
		}
		if config.APIKey == "" {
			config.APIKey = os.Getenv("NOOTY_API_KEY")
		}
		return
	}
	_ = json.Unmarshal(data, &config)
	if config.APIKey == "" {
		config.APIKey = os.Getenv("OPENAI_API_KEY")
		if config.APIKey == "" {
			config.APIKey = os.Getenv("NOOTY_API_KEY")
		}
	}
}

func saveConfig() {
	data, _ := json.MarshalIndent(config, "", "  ")
	_ = os.WriteFile(configFile, data, 0600)
}

func loadMemories() {
	data, err := os.ReadFile(memFile)
	if err != nil {
		memories = []Memory{}
		return
	}
	_ = json.Unmarshal(data, &memories)
}

func saveMemories() {
	data, _ := json.MarshalIndent(memories, "", "  ")
	_ = os.WriteFile(memFile, data, 0600)
}
