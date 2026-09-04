// nooty.go — NootyCLI v0.3 "Radin Pro" – Autonomous Agentic Intelligence
// Single-file, zero external dependencies, cross-platform (macOS / Linux / Windows / WSL).
//
// 🚀 Compile & Build:
//   go build -ldflags="-s -w" -o nooty nooty.go
//
// 🛠 Quick Commands:
//   /help         → Command reference
//   /mode cli     → Switch to Autonomous Agent Mode
//   /compact      → Compress and summarize message history
//   /sessions     → List saved sessions
//   /resume <id>  → Resume a previous session
//   /undo         → Revert last file modification
//   /commit       → Auto-generate conventional git commit

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	"syscall"
	"time"
)

// ---------- Cross-Platform ANSI Engine ----------
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
	MaxTokens        int    `json:"max_tokens"`
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
	Name string            `json:"name"`
	Args map[string]string `json:"args"`
}

type DNSResolver struct {
	Name    string
	Address string
	Latency time.Duration
}

type SessionMetadata struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
	Mode      string    `json:"mode"`
}

type Checkpoint struct {
	Timestamp time.Time
	FilePath  string
	Content   string
}

// ---------- Global State ----------
var (
	config          Config
	memories        []Memory
	sessionMessages []Message
	currentMode     = "chat" // "chat" or "cli"
	currentSession  SessionMetadata
	workspace       string
	homeDir         string
	nootyDir        string
	chatsDir        string
	checkpointsDir  string
	memFile         string
	configFile      string

	fallbackDNS = []DNSResolver{
		{Name: "Direct Connection", Address: ""},
		{Name: "Electro DNS", Address: "78.157.42.100"},
		{Name: "Shecan DNS #1", Address: "178.22.122.100"},
		{Name: "Shecan DNS #2", Address: "185.51.200.2"},
		{Name: "Begzar DNS #1", Address: "185.55.226.26"},
		{Name: "Begzar DNS #2", Address: "185.55.225.25"},
		{Name: "Cloudflare DNS", Address: "1.1.1.1"},
	}
	activeDNS = DNSResolver{Name: "Direct Connection", Address: ""}

	sharedTransport *http.Transport
	sharedClient    *http.Client
	checkpointStack []Checkpoint
	stateMutex      sync.Mutex

	agentRunning    bool
	agentCancelFunc context.CancelFunc
)

// ---------- Initialization & Main ----------
func main() {
	promptFlag := flag.String("p", "", "Direct prompt execution (Non-interactive mode)")
	modelFlag := flag.String("m", "", "Override model for this execution")
	modeFlag := flag.String("mode", "", "Execution mode: chat or cli")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "NootyCLI v0.3 Radin Pro — Agentic Terminal Intelligence\n\nUsage:\n  nooty [options]\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	initFileSystem()
	loadConfig()
	loadMemories()

	if *modelFlag != "" {
		config.Model = *modelFlag
	}
	if *modeFlag != "" {
		currentMode = *modeFlag
	}

	initSession()
	setupSignalHandler()
	raceDNSResolvers()
	initHTTPTransport()

	// Check for piped stdin or prompt flag
	stat, _ := os.Stdin.Stat()
	isPiped := (stat.Mode() & os.ModeCharDevice) == 0

	if isPiped || *promptFlag != "" {
		handleNonInteractive(isPiped, *promptFlag)
		return
	}

	drawHeader()
	repl()
}

func initFileSystem() {
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

	if config.Workspace == "" {
		cwd, err := os.Getwd()
		if err == nil {
			config.Workspace = cwd
		} else {
			config.Workspace = homeDir
		}
	}
	workspace = config.Workspace
}

func initSession() {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	currentSession = SessionMetadata{
		ID:        id,
		Title:     "Session " + id,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Mode:      currentMode,
	}
}

// ---------- Concurrent DNS Racing Engine ----------
func raceDNSResolvers() {
	type raceResult struct {
		resolver DNSResolver
		duration time.Duration
		err      error
	}

	results := make(chan raceResult, len(fallbackDNS))
	var wg sync.WaitGroup

	for _, res := range fallbackDNS {
		wg.Add(1)
		go func(r DNSResolver) {
			defer wg.Done()
			start := time.Now()
			target := "api.openai.com:443"

			var conn net.Conn
			var err error

			if r.Address == "" {
				d := net.Dialer{Timeout: 1200 * time.Millisecond}
				conn, err = d.Dial("tcp", target)
			} else {
				resolver := &net.Resolver{
					PreferGo: true,
					Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
						d := net.Dialer{Timeout: 1200 * time.Millisecond}
						return d.DialContext(ctx, "udp", r.Address+":53")
					},
				}
				ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
				defer cancel()
				ips, lookupErr := resolver.LookupHost(ctx, "api.openai.com")
				if lookupErr == nil && len(ips) > 0 {
					d := net.Dialer{Timeout: 1200 * time.Millisecond}
					conn, err = d.Dial("tcp", net.JoinHostPort(ips[0], "443"))
				} else {
					err = lookupErr
				}
			}

			dur := time.Since(start)
			if conn != nil {
				_ = conn.Close()
			}
			results <- raceResult{resolver: r, duration: dur, err: err}
		}(res)
	}

	wg.Wait()
	close(results)

	var valid []raceResult
	for res := range results {
		if res.err == nil {
			valid = append(valid, res)
		}
	}

	if len(valid) > 0 {
		sort.Slice(valid, func(i, j int) bool {
			return valid[i].duration < valid[j].duration
		})
		activeDNS = valid[0].resolver
		activeDNS.Latency = valid[0].duration
	} else {
		activeDNS = fallbackDNS[1] // Default to Electro if racing fails
	}
}

// ---------- High-Performance Connection Pool ----------
func initHTTPTransport() {
	var dialContext func(ctx context.Context, network, addr string) (net.Conn, error)

	if activeDNS.Address == "" {
		dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		dialContext = dialer.DialContext
	} else {
		customResolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, "udp", activeDNS.Address+":53")
			},
		}
		dialer := &net.Dialer{
			Resolver:  customResolver,
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		dialContext = dialer.DialContext
	}

	sharedTransport = &http.Transport{
		DialContext:         dialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression:  false,
	}

	sharedClient = &http.Client{
		Transport: sharedTransport,
		Timeout:   120 * time.Second,
	}
}

func doWithRetry(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
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

		resp, err := sharedClient.Do(req)
		if err == nil {
			if resp.StatusCode < 500 && resp.StatusCode != 429 && resp.StatusCode != 403 {
				return resp, nil
			}
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("HTTP status: %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		// Exponential Backoff: 1s, 2s, 4s
		backoff := time.Duration(1<<attempt) * time.Second
		time.Sleep(backoff)
	}

	return nil, fmt.Errorf("request failed after %d retries: %v", maxRetries, lastErr)
}

// ---------- Signal Handling ----------
func setupSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		for range sigChan {
			stateMutex.Lock()
			if agentRunning && agentCancelFunc != nil {
				fmt.Println("\n" + c(yellow) + "⏸ Agent execution interrupted by user (Ctrl+C). Session preserved." + c(reset))
				agentCancelFunc()
				agentRunning = false
				stateMutex.Unlock()
				fmt.Print(prompt())
				continue
			}
			stateMutex.Unlock()

			fmt.Println(c(dim) + "\n👋 NootyCLI session ended. Goodbye!" + c(reset))
			saveCurrentSession()
			os.Exit(0)
		}
	}()
}

// ---------- UI Header & Helpers ----------
func drawHeader() {
	width := 66
	line := strings.Repeat("─", width-2)

	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), centerText(" NOOTY CLI ", width-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), centerText("v0.3 Radin Pro — Agentic Terminal Intelligence", width-2), c(cyan), c(reset))
	fmt.Println(c(cyan) + "├" + line + "┤" + c(reset))

	prettyWorkspace := formatPath(workspace)
	dnsInfo := activeDNS.Name
	if activeDNS.Latency > 0 {
		dnsInfo += fmt.Sprintf(" (%dms)", activeDNS.Latency.Milliseconds())
	}

	entries := [][]string{
		{"Provider", truncateString(config.ProviderEndpoint, 40)},
		{"Model", config.Model},
		{"API Key", maskAPIKey(config.APIKey)},
		{"Workspace", truncateString(prettyWorkspace, 40)},
		{"DNS Shield", dnsInfo},
		{"Mode", strings.ToUpper(currentMode) + " Mode"},
	}

	for _, e := range entries {
		val := fmt.Sprintf("%-40s", e[1])
		fmt.Printf("%s│%s %-12s: %s%s %s│%s\n",
			c(cyan), c(bold)+c(white), e[0], c(green), val, c(cyan), c(reset))
	}
	fmt.Println(c(cyan) + "└" + line + "┘" + c(reset))
	fmt.Printf("%s💡 Type %s/help%s for commands, %s/mode cli%s for Agent Mode, %s\"\"\"%s for multiline.%s\n\n",
		c(dim), c(bold)+c(green), c(dim), c(bold)+c(cyan), c(dim), c(bold)+c(yellow), c(dim), c(reset))
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

// ---------- Non-Interactive / Pipe Mode ----------
func handleNonInteractive(isPiped bool, promptText string) {
	var input string
	if isPiped {
		pipedData, _ := io.ReadAll(os.Stdin)
		input = string(pipedData)
		if promptText != "" {
			input = promptText + "\n\nAttached Context:\n" + input
		}
	} else {
		input = promptText
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return
	}

	input = expandContextTags(input)
	messages := buildMessages(input)

	if currentMode == "cli" {
		runAgentLoop(messages)
	} else {
		streamResponse(messages)
	}
}

// ---------- Interactive REPL Engine ----------
func repl() {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(prompt())
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Multiline Mode (starts with """ or <<<)
		if trimmed == `"""` || trimmed == `<<<` {
			line = readMultilineInput(scanner, trimmed)
			trimmed = strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
		}

		if strings.HasPrefix(trimmed, "/") {
			handleSlashCommand(trimmed)
		} else if strings.HasPrefix(trimmed, "!") && currentMode == "cli" {
			handleShellBang(trimmed[1:])
		} else {
			processedInput := expandContextTags(trimmed)
			handleChat(processedInput)
		}
	}
	saveCurrentSession()
	fmt.Println(c(dim) + "\n👋 NootyCLI session ended. Goodbye!" + c(reset))
}

func readMultilineInput(scanner *bufio.Scanner, delimiter string) string {
	fmt.Printf("%s📝 Multi-line mode enabled. Type '%s' on a new line to finish.%s\n", c(dim), delimiter, c(reset))
	var lines []string
	for {
		fmt.Print(c(dim) + "... " + c(reset))
		if !scanner.Scan() {
			break
		}
		text := scanner.Text()
		if strings.TrimSpace(text) == delimiter {
			break
		}
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n")
}

func prompt() string {
	if currentMode == "cli" {
		return c(bold) + c(cyan) + "🤖 nooty[agent]" + c(yellow) + " ❯ " + c(reset)
	}
	return c(bold) + c(green) + "⚡ nooty" + c(white) + " ❯ " + c(reset)
}

// ---------- Smart @File and @Dir Context Injection ----------
func expandContextTags(input string) string {
	re := regexp.MustCompile(`@([a-zA-Z0-9_./\-]+)`)
	matches := re.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return input
	}

	var attachments strings.Builder
	for _, m := range matches {
		targetPath := safeJoin(workspace, m[1])
		info, err := os.Stat(targetPath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			attachments.WriteString(fmt.Sprintf("\n\n--- Attached Directory Structure: %s ---\n", m[1]))
			attachments.WriteString(dirTree(targetPath, ""))
		} else if info.Size() < 500_000 {
			data, err := os.ReadFile(targetPath)
			if err == nil {
				attachments.WriteString(fmt.Sprintf("\n\n--- Attached File: %s ---\n%s\n", m[1], string(data)))
			}
		}
	}

	if attachments.Len() > 0 {
		return input + "\n" + attachments.String()
	}
	return input
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
	case "/compact":
		compactHistory(true)
	case "/undo":
		restoreLastCheckpoint()
	case "/commit":
		handleGitCommit()
	case "/sessions":
		listSessions()
	case "/resume":
		if len(parts) > 1 {
			resumeSession(parts[1])
		} else {
			fmt.Println("Usage: /resume <session_id>")
		}
	case "/export":
		exportSessionToMarkdown()
	case "/clear":
		sessionMessages = nil
		initSession()
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
		saveCurrentSession()
		os.Exit(0)
	default:
		fmt.Printf("❌ Unknown command: %s. Type /help for assistance.\n", parts[0])
	}
}

func printHelp() {
	fmt.Println(c(bold) + "\n📌 NootyCLI v0.3 Command Reference:" + c(reset))
	fmt.Println(`
  /help                        Show command help overview
  /mode [chat|cli]             Toggle Chat or Autonomous Agent CLI Mode
  /compact                     Summarize & compress context tokens
  /undo                        Revert last modified file from checkpoint
  /commit                      Generate & execute conventional git commit
  /sessions                    List stored conversation sessions
  /resume <id>                 Restore and continue a previous session
  /export                      Export current chat session to Markdown
  /config                      Interactive wizard for API key, model & endpoint
  /workspace show|set <path>   Manage current working directory
  /model show|set <name>|list  View, switch, or browse models interactively
  /dns                         Display Anti-Sanction Smart DNS Shield status
  /doctor                      Run full connection and API diagnostic
  /memory list|add|forget      Manage long-term persistent agent context
  /safety strict|balanced      Set command confirmation safety policies
  /history                     Display conversation session log
  /clear                       Reset screen & initialize fresh session
  /exit                        Save session and exit

  💡 In Agent CLI Mode: Prefix commands with ! for direct shell execution.
  💡 Context Injection: Use @filename or @dirname inside your prompt.
  💡 Multi-line Input: Type """ or <<< to enter multi-line mode.`)
}

// ---------- Token Estimation & Metrics ----------
func estimateTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)/4 + 4
	}
	return total
}

func printMetrics(charCount int, duration time.Duration) {
	estimatedTokens := charCount / 4
	if estimatedTokens <= 0 {
		estimatedTokens = 1
	}
	sec := duration.Seconds()
	if sec <= 0.05 {
		sec = 0.05
	}
	tokPerSec := float64(estimatedTokens) / sec

	fmt.Printf("%s⚡ %d tokens | %.1f tok/s | %.2fs | Model: %s%s\n\n",
		c(dim), estimatedTokens, tokPerSec, sec, config.Model, c(reset))
}

// ---------- Diff & Checkpoint System ----------
func showDiffPreview(filePath, oldContent, newContent string) {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	fmt.Printf("%s\n🔍 Proposed Changes for: %s%s\n", c(bold)+c(yellow), filePath, c(reset))

	oldMap := make(map[string]bool)
	for _, l := range oldLines {
		oldMap[l] = true
	}
	newMap := make(map[string]bool)
	for _, l := range newLines {
		newMap[l] = true
	}

	// Show simple line diff
	for _, l := range oldLines {
		if !newMap[l] && strings.TrimSpace(l) != "" {
			fmt.Printf("%s- %s%s\n", c(red), l, c(reset))
		}
	}
	for _, l := range newLines {
		if !oldMap[l] && strings.TrimSpace(l) != "" {
			fmt.Printf("%s+ %s%s\n", c(green), l, c(reset))
		}
	}
}

func saveCheckpoint(filePath string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return // New file, nothing to backup
	}
	cp := Checkpoint{
		Timestamp: time.Now(),
		FilePath:  filePath,
		Content:   string(data),
	}
	checkpointStack = append(checkpointStack, cp)

	// Save to ~/.nooty/checkpoints/
	filename := fmt.Sprintf("%d_%s", time.Now().Unix(), filepath.Base(filePath))
	_ = os.WriteFile(filepath.Join(checkpointsDir, filename), data, 0600)
}

func restoreLastCheckpoint() {
	if len(checkpointStack) == 0 {
		fmt.Println("⚠️ No checkpoints available to undo.")
		return
	}
	last := checkpointStack[len(checkpointStack)-1]
	checkpointStack = checkpointStack[:len(checkpointStack)-1]

	err := os.WriteFile(last.FilePath, []byte(last.Content), 0644)
	if err != nil {
		fmt.Printf("%s❌ Undo failed: %v%s\n", c(red), err, c(reset))
		return
	}
	fmt.Printf("%s✅ Successfully restored %s to checkpoint from %s%s\n",
		c(green), formatPath(last.FilePath), last.Timestamp.Format("15:04:05"), c(reset))
}

// ---------- Context Compaction & Summarization ----------
func compactHistory(manual bool) {
	if len(sessionMessages) < 4 {
		if manual {
			fmt.Println("ℹ️ Session history is too short to compact.")
		}
		return
	}

	fmt.Print(c(yellow) + "🧠 Compacting conversation context... " + c(reset))

	older := sessionMessages[:len(sessionMessages)-2]
	recent := sessionMessages[len(sessionMessages)-2:]

	var summaryBuilder strings.Builder
	for _, m := range older {
		summaryBuilder.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}

	summaryPrompt := []Message{
		{Role: "system", Content: "Summarize this technical development history into a concise context block (max 150 words), preserving all architectural decisions, code changes, and open tasks:"},
		{Role: "user", Content: summaryBuilder.String()},
	}

	summaryText, err := getModelResponseText(summaryPrompt)
	if err != nil {
		fmt.Printf("%s❌ Compaction failed: %v%s\n", c(red), err, c(reset))
		return
	}

	sessionMessages = append([]Message{
		{Role: "system", Content: "Previous Conversation Context Summary:\n" + summaryText},
	}, recent...)

	fmt.Println(c(green) + "✅ Compacted!" + c(reset))
}

// ---------- Session Persistence & Markdown Export ----------
func saveCurrentSession() {
	if len(sessionMessages) == 0 {
		return
	}
	currentSession.Messages = sessionMessages
	currentSession.UpdatedAt = time.Now()
	currentSession.Mode = currentMode

	 ---------- Context Compaction & Summarization ----------
func compactHistory(manual bool) {
	if len(sessionMessages) < 4 {
		if manual {
			fmt.Println("ℹ️ Session history is too short to compact.")
		}
		return
	}

	fmt.Print(c(yellow) + "🧠 Compacting conversation context... " + c(reset))

	older := sessionMessages[:len(sessionMessages)-2]
	recent := sessionMessages[len(sessionMessages)-2:]

	var summaryBuilder strings.Builder
	for _, m := range older {
		summaryBuilder.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}

	summaryPrompt := []Message{
		{Role: "system", Content: "Summarize this technical development history into a concise context block (max 150 words), preserving all architectural decisions, code changes, and open tasks:"},
		{Role: "user", Content: summaryBuilder.String()},
	}

	summaryText, err := getModelResponseText(summaryPrompt)
	if err != nil {
		fmt.Printf("%s❌ Compaction failed: %v%s\n", c(red), err, c(reset))
		return
	}

	sessionMessages = append([]Message{
		{Role: "system", Content: "Previous Conversation Context Summary:\n" + summaryText},
	}, recent...)

	fmt.Println(c(green) + "✅ Compacted!" + c(reset))
}

// ---------- Session Persistence & Markdown Export ----------
func saveCurrentSession() {
	if len(sessionMessages) == 0 {
		return
	}
	currentSession.Messages = sessionMessages
	currentSession.UpdatedAt = time.Now()
	currentSession.Mode = currentMode

	filePath := filepath.Join(chatsDir, currentSession.ID+".json")
	data, _ := json.MarshalIndent(currentSession, "", "  ")
	_ = os.WriteFile(filePath, data, 0600)
}

func listSessions() {
	entries, err := os.ReadDir(chatsDir)
	if err != nil || len(entries) == 0 {
		fmt.Println("📁 No saved sessions found.")
		return
	}

	fmt.Println(cPrintln("⚠️ Session is empty, nothing to export.")
		return
	}

	filename := fmt.Sprintf("export_%s_%d.md", currentSession.ID, time.Now().Unix())
	exportPath := filepath.Join(workspace, filename)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# NootyCLI Session Export: %s\n\n", currentSession.ID))
	sb.WriteString(fmt.Sprintf("**Date:** %s  \n**Model:** %s  \n**Workspace:** `%s`\n\n---\n\n",
		time.Now().Format(time.RFC1123), config.Model, workspace))

	for _, m := range sessionMessages {
		if m.Role == "user" {
			sb.WriteString("### 👤 User:\n" + m.Content + "\n\n")
		} else if m.Role == "assistant" {
			sb.WriteString("### 🤖 Nooty:\n" + m.Content + "\n\n---\n\n")
		}
	}

	_ = os.WriteFile(exportPath, []byte(sb.String()), 0644)
	fmt.Printf("%s✅ Exported session to: %s%s\n", c(green), filename, c(reset))
}

// ---------- Git Quick Commit Helper ----------
func handleGitCommit() {
	cmdDiff := exec.Command("git", "diff", "--staged")
	cmdDiff.Dir = workspace
	out, err := cmdDiff.Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		// Try non-staged diff
		cmdDiff = exec.Command("git", "diff")
		cmdDiff.Dir = workspace
		out, _ = cmdDiff.Output()
	}

	diffStr := strings.TrimSpace(string(out))
	if diffStr == "" {
		fmt.Println("ℹ️ No git changes detected to commit.")
		return
	}

	fmt.Print(c(yellow) + "🤖 Generating conventional commit message... " + c(reset))
	promptMsgs := []Message{
		{Role: "system", Content: "Generate a concise, conventional git commit message (e.g. feat:, fix:, refactor:) based strictly on this diff. Output ONLY the commit message line."},
		{Role: "user", Content: diffStr},
	}

	commitMsg, err := get m.Role == "user" {
			sb.WriteString("### 👤 User:\n" + m.Content + "\n\n")
		} else if m.Role == "assistant" {
			sb.WriteString("### 🤖 Nooty:\n" + m.Content + "\n\n---\n\n")
		}
	}

	_ = os.WriteFile(exportPath, []byte(sb.String()), 0644)
	fmt.Printf("%s✅ Exported session to: %s%s\n", c(green), filename, c(reset))
}

// ---------- Git Quick Commit Helper ----------
func handleGitCommit() {
	cmdDiff := exec.Command("git", "diff", "--staged")
	cmdDiff.Dir = workspace
	out, err := cmdDiff.Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		// Try non-staged diff
		cmdDiff = exec.Command("git", "diff")
		cmdDiff.Dir = workspace
		out, _ = cmdDiff.Output()
	}

	diffStr := strings.TrimSpace(string(out))
	if diffStr == "" {
		fmt.Println("ℹ️ No git changes detected to commit.")
		return
	}

	fmt.Print(c(yellow) + "🤖 Generating conventional commit message... " + c(reset))
	promptMsgs := []Message{
		{Role: "system", Content: "Generate a concise, conventional git commit message (e.g. feat:, fix:, refactor:) based strictly on this diff. Output ONLY the commit message line."},
		{Role: "user", Content: diffStr},
	}

	commitMsg, err := getModelResponseText(promptMsgs)
	if err != nil {
		fmt.Printf("%s❌ Failed: %v%s\n", c(red), err, c(reset))
		return
	}
	commitMsg = strings.TrimSpace(commitMsg)
	fmt.Printf("\n%s📝 Proposed Commit:%s %s\n", c(bold), c(green), commitMsg)

	fmt.Print("Execute git- list_files (path="relative_path")
- tree (path="relative_path")
- read_file (path="relative_path")
- write_file (path="relative_path", content="full_content")
- patch_file (path="relative_path", search="target_code_block", replace="replacement_code_block")
- append_file (path="relative_path", content="text_to_append")
- delete_file (path="relative_path")
- search_code (query="text", path="relative_path")
- find_files (pattern="glob_pattern")
- count_tokens (path="relative_path")
- file_info (path="relative_path")
- git_status
- git_diff
- run_command (command="shell_cmd", timeout="seconds")

IMPORTANT: Prefer 'patch_file' over 'write_file' for existing files to save tokens. Ensure exact matches in 'search'.`

	relevant := getRelevantMemories(userInput)
	if len(relevant) > 0 {
		sysPrompt += "\n\nUser Context & Memories:\n"
		for _, m := range relevant {
			sysPrompt += fmt.Sprintf("- [%s] %s\n", m.Tag, m.Content)
		}
	}

	msgs = append(msgs, Message{Role: "system", Content: sysPrompt})
	msgs = append(msgs, sessionMessages...)

	userMsg := Message{Role: "user", Content: userInput}
	msgs = append(msgs, userMsg)
	sessionMessages = append(sessionMessages, userMsg)

	return msgs
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

func streamResponse(messages []Message) {
	reqPayload := ChatRequest{Model: config.Model, Messages: messages, Stream: true}
	jsonData, _ := json.Marshal(reqPayload)
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
	headers := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}

	start := time.Now()
	resp, err := doWithRetry("POST", endpoint, jsonData, headers)
	if err != nil {
		fmt.Printf("%s❌ Request error: %v%s\n", c(red), err, c(reset))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("%s❌ Provider Error %d: %s%s\n", c(red), resp.StatusCode, string(body), c(reset))
		return
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

	duration := time.Since(start)
	fmt.Print(c(reset) + "\n\n")

	out := fullContent.String()
	sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: out})
	saveCurrentSession()
	printMetrics(len(out), duration)
}

// ---------- Agentic Loop with Self-Correction & Reflection ----------
func runAgentLoop(messages []Message) {
	stateMutex.Lock()
	agentRunning = true
	var ctx context.Context
	ctx", c(red), err, c(reset))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("%s❌ Provider Error %d: %s%s\n", c(red), resp.StatusCode, string(body), c(reset))
		return
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

	duration := time.Since(start)
	fmt.Print(c(reset) + "\n\n")

	out := fullContent.String()
	sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: out})
	saveCurrentSession()
	printMetrics(len(out), duration)
}

// ---------- Agentic Loop with Self-Correction & Reflection ----------
func runAgentLoop(messages []Message) {
	stateMutex.Lock()
	agentRunning = true
	var ctx context.Context
	ctx, agentCancelFunc = context.WithCancel(context.Background())
	stateMutex.Unlock()

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s⚠️ Agent loop recovered from crash: %v. Session preserved.%s\n", c(red), r, c(reset))
		}
		stateMutex.Lock()		}

		respText, err := getModelResponseText(msgs)
		if err != nil {
			fmt.Printf("%s❌ Agent Step Error: %v%s\n", c(red), err, c(reset))
			return
		}

		toolCalls := extractMultipleToolCalls(respText)
		if len(toolCalls) == 0 {
			fmt.Println("\n" + c(green) + respText + c(reset) + "\n")
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText})
			return
		}

		// Execute Batch Tools (Parallel for Read-Only, Sequential for Mutations)
		results := executeBatchTools(toolCalls)

		var combinedFeedback strings.Builder
		for i, res := range results {
			tc := toolCalls[i]
			fmt.Printf("\n%s🔧 Action [%d/%d]: %s%s\n", c(bold)+c(yellow), i+1, len(toolCalls), tc.Name, c(reset))
			for k, v := range tc.Args {
				fmt.Printf("   %s%s%s: %s\n", c(dim), k, c(reset), truncateString(v, 60))
			}
			if len(res.Output) > 3000 {
				res.Output = res.Output[:3000] + "\n... (output truncated)"
			}
			fmt.Printf("%s📄 Output:%s\n%s\n", c(dim), c(reset), res.Output)

			combinedFeedback.WriteString(fmt.Sprintf("Tool '%s' result:\n%s\n\n", tc.Name, res.Output))
		}

		msgs = append(msgs,
			Message{Role: "assistant", Content: respText},
			Message{Role: "user", Content: strings.TrimSpace(combinedFeedback.String())},
		)
	}

	fmt.Printf("%s⚠️ Agent loop step limit reached (%d steps).%s\n", c(yellow), maxSteps, c(reset))
}

type toolExecResult struct {
	Output   string
	Approved bool
	Error    error
}

func executeBatchTools(calls []ToolCall) []toolExecResult {
	results := make([]toolExecResult, len(calls))

	// Check if all are read-only
	allReadOnly := true
	for _, tc := range calls {
		if !isReadOnlyTool(tc.Name) {
			allReadOnly = false
			break
		}
	}

	if allReadOnly && len(calls) > 1 {
		// Parallel execution
		var wg sync.WaitGroup
		for i, tc := range calls {
			wg.Add(1)
			go func(idx int, call ToolCall) {
				defer wg.Done()
				out, err := runTool(call.Name, call.Args)
				if err != nil {
					results[idx] = toolExecResult{Output: fmt.Sprintf("Error: %v", err), Error: err}
				} else {
					results[idx] = toolExecResult{Output: out, Approved: true}
				}
			}(i, tc)
		}
		wg.Wait()
		return results
	}

	// Sequential execution for writes / mixed calls
	for i, tc := range calls {
		out, approved := executeAgentTool(&tc)
		results[i] = toolExecResult{Output: out, Approved: approved}
		if !approved {
			break
		}
	}
	return results
}

func isReadOnlyTool(name string) bool {
	switch name {
	case "list_files", "tree", "read_file", "search_code", "find_files", "file_info", "count_tokens", "git_status", "git_diff":
		return true
	default:
		return false
	}
}

func getModelResponseText(messages []Message) (string, error) {
	reqPayload := ChatRequest{Model: config.Model, Messages: messages, Stream: false}
	jsonData, _ := json.Marshal(reqPayload)
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
	headers := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}

	resp, err := doWithRetry("POST", endpoint, jsonData, headers)
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
		return "", fmt.Errorf("empty response choices from provider")
	}

	return result.Choices[0].Message.Content, nil
}

func extractMultipleToolCalls(text string) []ToolCall {
	var calls []ToolCall
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TOOL:") || strings.HasPrefix(line, "TOOL：") {
			tc := parseToolLine(line)
			if tc != nil {
				calls = append(calls, *tc)
			}
		}
	}
	return calls
}

func parseToolLine(line string) *ToolCall {
	line = strings.TrimPrefix(line, "TOOL:")
	line = strings.TrimPrefix(line, "TOOL：")
	line = strings.TrimSpace(line)

	parts := strings.SplitN(line, " ", 2)
	if len(parts) < 1 {
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
	re := regexp.MustCompile(`(\w+)=("(?:[^"\\\\]|\\\\.)*"|'(?:[^'\\\\]|\\\\.)*'|` + "`" + `(?:[^` + "`" + `\\\\]|\\\\.)*` + "`" + `|\S+)`)
	matches := re.FindAllStringSubmatch(argsStr, -1)

	for _, match := range matches {
		if len(match) == 3 {
			key := match[1]
			val := match[2]
			if (strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`)) ||
				(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) ||
				(strings.HasPrefix(val, "`") && strings.HasSuffix(val, "`")) {
				val = val[1 : len(val)-1]
			}
			val = strings.ReplaceAll(val, `\n`, "\n")
			val = strings.ReplaceAll(val, `\t`, "\t")
			val = strings.ReplaceAll(val, `\"`, `"`)
			args[key] = val
		}
	}

	if len(args) == 0 && strings.TrimSpace(argsStr) != "" {
		args["path"] = strings.TrimSpace(argsStr)
	}

	return &ToolCall{Name: name, Args: args}
}

func executeAgentTool(tc *ToolCall) (string, bool) {
	needsApproval := !isReadOnlyTool(tc.Name)

	if needsApproval {
		if tc.Name == "delete_file" {
			fmt.Printf("%s⚠️ SAFETY WARNING: %s will permanently remove %s!%s\n", c(red), tc.Name, tc.Args["path"], c(reset))
			fmt.Print("Type DELETE to confirm action: ")
			reader := bufio.NewReader(os.Stdin)
			confirm, _ := reader.ReadString('\n')
			if strings.TrimSpace(confirm) != "DELETE" {
				return "Operation aborted by user safety policy.", false
			}
		} else if tc.Name == "patch_file" || tc.Name == "replace_in_file" {
			targetPath := safeJoin(workspace, tc.Args["path"])
			data, err := os.ReadFile(targetPath)
			if err == nil {
				search := tc.Args["search"]
				if search == "" {
					search = tc.Args["old_str"]
				}
				replace := tc.Args["replace"]
				if replace == "" {
					replace = tc.Args["new_str"]
				}
				showDiffPreview(tc.Args["path"], search, replace)
			}
			fmt.Print("Apply patch? [Y/n]: ")
			reader := bufio.NewReader(os.Stdin)
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(strings.ToLower(confirm))
			if confirm == "n" || confirm == "no" {
				return "Patch rejected by user.", false
			}
		} else {
			fmt.Printf("Confirm execution of %s? [Y/n]: ", tc.Name)
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
		var names []string
		for _, e := range entries {
			if isIgnored(e.Name()) {
				continue
			}
			if e.IsDir() {
				names = append(names, e.Name()+"/")
			} else {
				names = append(names, e.Name())
			}
		}
		if len(names) == 0 {
			return "(directory empty)", nil
		}
		return strings.Join(names, "\n"), nil

	case "tree":
		path := workspace
		if p, ok := args["path"]; ok && p != "" && p != "." {
			path = safeJoin(workspace, p)
		}
		return dirTree(path, ""), nil

	case "read_file":
		path := safeJoin(workspace, args["path"])
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil

	case "write_file", "create_file":
		path := safeJoin(workspace, args["path"])
		saveCheckpoint(path) // Backup before write
		content := args["content"]
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ File saved (%d bytes): %s", len(content), args["path"]), nil

	case "patch_file", "replace_in_file":
		path := safeJoin(workspace, args["path"])
		searchBlock := args["search"]
		if searchBlock == "" {
			searchBlock = args["old_str"]
		}
		replaceBlock := args["replace"]
		if replaceBlock == "" {
			replaceBlock = args["new_str"]
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}

		content := string(data)
		if !strings.Contains(content, searchBlock) {
			return "❌ Error: Target search block not found in file. Ensure exact match.", nil
		}

		saveCheckpoint(path) // Backup before patch
		newContent := strings.Replace(content, searchBlock, replaceBlock, 1)
		if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Successfully patched: %s", args["path"]), nil

	case "append_file":
		path := safeJoin(workspace, args["path"])
		saveCheckpoint(path)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return "", err
		}
		defer f.Close()
		content := args["content"]
		if _, err := f.WriteString(content); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Appended %d bytes to %s", len(content), args["path"]), nil

	case "delete_file":
		path := safeJoin(workspace, args["path"])
		saveCheckpoint(path)
		if err := os.Remove(path); err != nil {
			return "", err
		}
		return "✅ File removed: " + args["path"], nil

	case "search_code":
		query := args["query"]
		scope := workspace
		if s, ok := args["path"]; ok && s != "" {
			scope = safeJoin(workspace, s)
		}
		var results []string
		_ = filepath.Walk(scope, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || info.Size() > 1_000_000 || isIgnored(info.Name()) {
				return nil
			}
			f, err := os.Open(p)
			if err != nil {
				return nil
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			lineNum := 1
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, query) {
					rel, _ := filepath.Rel(workspace, p)
					results = append(results, fmt.Sprintf("%s:%d: %s", rel, lineNum, strings.TrimSpace(line)))
					if len(results) >= 50 {
						return fmt.Errorf("limit")
					}
				}
				lineNum++
			}
			return nil
		})
		if len(results) == 0 {
			return "No matches found.", nil
		}
		return strings.Join(results, "\n"), nil

	case "find_files":
		pattern := args["pattern"]
		if pattern == "" {
			pattern = "*"
		}
		var matches []string
		_ = filepath.Walk(workspace, func(p string, info os.FileInfo, err error) error {
			if err != nil || isIgnored(info.Name()) {
				if info != nil && info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(workspace, p)
			matched, _ := filepath.Match(pattern, info.Name())
			if matched {
				matches = append(matches, rel)
			}
			return nil
		})
		if len(matches) == 0 {
			return "No files matched pattern.", nil
		}
		return strings.Join(matches, "\n"), nil

	case "count_tokens":
		path := safeJoin(workspace, args["path"])
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		toks := len(data) / 4
		return fmt.Sprintf("Estimated tokens for %s: ~%d tokens (%d bytes)", args["path"], toks, len(data)), nil

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

		if err := cmd.Start(); err != nil {
			return "", err
		}

		var outputBuf bytes.Buffer
		var streamWg sync.WaitGroup
		streamWg.Add(2)

		// Live Subprocess Streaming with Dim Color
		streamScanner := func(r io.Reader, prefix string) {
			defer streamWg.Done()
			sc := bufio.NewScanner(r)
			for sc.Scan() {
				line := sc.Text()
				outputBuf.WriteString(line + "\n")
				fmt.Printf("%s%s%s\n", c(dim), line, c(reset))
			}
		}

		go streamScanner(stdoutPipe, "")
		go streamScanner(stderrPipe, "[err] ")

		done := make(chan error)
		go func() {
			streamWg.Wait()
			done <- cmd.Wait()
		}()

		select {
		case err := <-done:
			out := outputBuf.String()
			if err != nil {
				return out + fmt.Sprintf("\nCommand exited with error: %v", err), nil
			}
			if strings.TrimSpace(out) == "" {
				out = "(command executed successfully with no output)"
			}
			return out, nil
		case <-time.After(time.Duration(timeout) * time.Second):
			_ = cmd.Process.Kill()
			return outputBuf.String() + fmt.Sprintf("\n[Execution timed out after %d seconds]", timeout), nil
		}
	}
	return "", fmt.Errorf("unknown tool: %s", name)
}

func isIgnored(name string) bool {
	ignored := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "dist": true,
		".idea": true, ".vscode": true, ".DS_Store": true, "build": true,
	}
	return ignored[name] || strings.HasPrefix(name, ".")
}

func dirTree(root, indent string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err.Error()
	}
	var out string
	var validEntries []os.DirEntry
	for _, e := range entries {
		if !isIgnored(e.Name()) {
			validEntries = append(validEntries, e)
		}
	}

	for i, e := range validEntries {
		prefix := indent + "├── "
		childIndent := indent + "│   "
		if i == len(validEntries)-1 {
			prefix = indent + "└── "
			childIndent = indent + "    "
		}
		out += prefix + e.Name() + "\n"
		if e.IsDir() {
			out += dirTree(filepath.Join(root, e.Name()), childIndent)
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

// ---------- Direct Shell Execution ----------
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

// ---------- Diagnostic & Settings Wizard ----------
func handleConfig() {
	fmt.Println(c(bold) + "\n⚙️ Nooty Configuration Wizard" + c(reset))
	fmt.Println("Press Enter to keep existing settings.\n")
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
	fmt.Println(c(bold) + "\n🛡️ Anti-Sanction Smart DNS Status:" + c(reset))
	for i, dns := range fallbackDNS {
		status := ""
		if dns.Name == activeDNS.Name {
			status = c(green) + fmt.Sprintf(" [ACTIVE - %dms]", activeDNS.Latency.Milliseconds()) + c(reset)
		}
		if dns.Address == "" {
			fmt.Printf("  %d. %-24s (System Default)%s\n", i+1, dns.Name, status)
		} else {
			fmt.Printf("  %d. %-24s (%s)%s\n", i+1, dns.Name, dns.Address, status)
		}
	}
	fmt.Println()
}

func runDoctor() {
	fmt.Println(c(bold) + "\n🏥 Nooty Diagnostic Doctor" + c(reset))
	fmt.Printf("• Provider Endpoint : %s\n", config.ProviderEndpoint)
	fmt.Printf("• Active Model      : %s\n", config.Model)
	fmt.Printf("• API Key           : %s\n", maskAPIKey(config.APIKey))
	fmt.Printf("• Active Workspace  : %s\n", formatPath(workspace))
	fmt.Printf("• Active DNS Shield : %s (%dms)\n", activeDNS.Name, activeDNS.Latency.Milliseconds())
	fmt.Print("• Provider Status   : ")

	models, err := fetchAvailableModels()
	if err != nil {
		fmt.Printf("%sFAILED (%v)%s\n\n", c(red), err, c(reset))
	} else {
		fmt.Printf("%sOK (%d models accessible)%s\n\n", c(green), len(models), c(reset))
	}
}

func fetchAvailableModels() ([]string, error) {
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/models"
	headers := map[string]string{}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}

	resp, err := doWithRetry("GET", endpoint, nil, headers)
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
		fmt.Printf("\n%s📋 Available Provider Models (Page %d/%d):%s\n", c(bold), page+1, totalPages)
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

// ---------- Config & Memory Persistence ----------
func loadConfig() {
	data, err := os.ReadFile(configFile)
	if err != nil {
		config = Config{
			ProviderEndpoint: "https://api.openai.com/v1",
			APIKey:           os.Getenv("OPENAI_API_KEY"),
			Model:            "gpt-4o-mini",
			Safety:           "strict",
			MaxTokens:        4096,
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
