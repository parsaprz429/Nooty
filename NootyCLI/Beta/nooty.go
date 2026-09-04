// nooty.go — NootyCLI v0.3 "Radin Ultra" – Next-Gen Agentic Terminal Intelligence
// Single-file, zero external dependencies, cross-platform (macOS / Linux / Windows / WSL).
//
// 🚀 Compile & Build:
//   go build -ldflags="-s -w" -o nooty nooty.go
//
// 🛠 Quick Commands:
//   /help         → Full command reference
//   /mode cli     → Autonomous Agentic Workspace Mode
//   /undo         → Revert last file modification
//   /compact      → Summarize context & save tokens
//   /commit       → Auto-generate conventional git commit
//   /sessions     → Manage and resume previous sessions
//   /export       → Export conversation to Markdown

package main

import (
	"bufio"
	"bytes"
	"context"
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

// ==========================================
// 🎨 ANSI Styling & Terminal Engine
// ==========================================
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
	reset       = "\033[0m"
	bold        = "\033[1m"
	dim         = "\033[2m"
	italic      = "\033[3m"
	underline   = "\033[4m"
	red         = "\033[31m"
	green       = "\033[32m"
	yellow      = "\033[33m"
	blue        = "\033[34m"
	magenta     = "\033[35m"
	cyan        = "\033[36m"
	white       = "\033[37m"
	bgDarkGray  = "\033[100m"
)

// ==========================================
// 📦 Data Models & Structures
// ==========================================
type Config struct {
	ProviderEndpoint string `json:"provider_endpoint"`
	APIKey           string `json:"api_key"`
	Model            string `json:"model"`
	Safety           string `json:"safety"`
	Workspace        string `json:"workspace"`
	AutoCompact      bool   `json:"auto_compact"`
	MaxTokensContext int    `json:"max_tokens_context"`
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

type ToolResult struct {
	Name   string
	Output string
	Err    error
}

type DNSResolver struct {
	Name    string
	Address string
	Latency time.Duration
	Working bool
}

type SessionData struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Mode      string    `json:"mode"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	Messages  []Message `json:"messages"`
	Workspace string    `json:"workspace"`
}

type Checkpoint struct {
	OriginalPath string
	BackupPath   string
	Timestamp    time.Time
}

// ==========================================
// 🌐 Global Runtime State
// ==========================================
var (
	config          Config
	memories        []Memory
	sessionMessages []Message
	currentSession  SessionData
	currentMode     = "chat" // "chat" or "cli"
	workspace       string
	homeDir         string
	nootyDir        string
	chatsDir        string
	checkpointsDir  string
	memFile         string
	configFile      string

	checkpoints     []Checkpoint
	agentRunning    bool
	agentCancelFunc context.CancelFunc

	dnsResolvers = []DNSResolver{
		{Name: "Direct Connection", Address: ""},
		{Name: "Electro DNS", Address: "78.157.42.100"},
		{Name: "Shecan DNS #1", Address: "178.22.122.100"},
		{Name: "Shecan DNS #2", Address: "185.51.200.2"},
		{Name: "Begzar DNS #1", Address: "185.55.226.26"},
		{Name: "Begzar DNS #2", Address: "185.55.225.25"},
		{Name: "Cloudflare (1.1.1.1)", Address: "1.1.1.1"},
		{Name: "Google DNS (8.8.8.8)", Address: "8.8.8.8"},
	}
	activeDNS = DNSResolver{Name: "Direct Connection", Address: ""}

	// Global HTTP Client with optimized connection pooling
	globalTransport *http.Transport
	globalHTTPClient *http.Client
)

// ==========================================
// 🚀 Main Entry Point
// ==========================================
func main() {
	promptFlag := flag.String("p", "", "Direct prompt execution (Non-interactive mode)")
	modeFlag := flag.String("mode", "", "Initial mode: 'chat' or 'cli'")
	sessionFlag := flag.String("resume", "", "Resume a specific session ID")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%sNootyCLI v0.3 Radin Ultra%s — Next-Gen Agentic Terminal Intelligence\n\nUsage:\n  nooty [flags]\n  cat file.txt | nooty -p \"explain this\"\n\nFlags:\n", c(bold)+c(cyan), c(reset))
		flag.PrintDefaults()
	}
	flag.Parse()

	initFileSystem()
	loadConfig()
	loadMemories()
	setupSignalHandler()

	if *modeFlag == "cli" || *modeFlag == "chat" {
		currentMode = *modeFlag
	}

	if config.Workspace == "" {
		if cwd, err := os.Getwd(); err == nil {
			config.Workspace = cwd
		} else {
			config.Workspace = homeDir
		}
	}
	workspace = config.Workspace

	// 1. Race fastest anti-sanction DNS in parallel
	raceFastestDNS()
	setupHTTPClient()

	// 2. Initialize Session
	if *sessionFlag != "" {
		if err := loadSession(*sessionFlag); err != nil {
			fmt.Printf("%s⚠️ Failed to load session '%s': %v%s\n", c(yellow), *sessionFlag, err, c(reset))
			createNewSession()
		}
	} else {
		createNewSession()
	}

	// 3. Handle Non-Interactive / Pipe Mode
	stat, _ := os.Stdin.Stat()
	isPiped := (stat.Mode() & os.ModeCharDevice) == 0

	if isPiped || *promptFlag != "" {
		var fullPrompt string
		if isPiped {
			pipedBytes, _ := io.ReadAll(os.Stdin)
			fullPrompt = string(pipedBytes)
			if *promptFlag != "" {
				fullPrompt = fullPrompt + "\n\n" + *promptFlag
			}
		} else {
			fullPrompt = *promptFlag
		}
		fullPrompt = strings.TrimSpace(fullPrompt)
		if fullPrompt != "" {
			fullPrompt = injectContextMentions(fullPrompt)
			handleChat(fullPrompt)
			saveCurrentSession()
			os.Exit(0)
		}
	}

	// 4. Interactive REPL Mode
	drawHeader()
	repl()
}

// ==========================================
// 📂 File System & Initialization
// ==========================================
func initFileSystem() {
	var err error
	homeDir, err = os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "⚠ Error: Unable to resolve user home directory.")
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
}

func setupSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		for range sigChan {
			if agentRunning && agentCancelFunc != nil {
				fmt.Printf("\n%s⏸ Action interrupted by user! Session context preserved.%s\n", c(bold)+c(yellow), c(reset))
				agentCancelFunc()
				agentRunning = false
			} else {
				saveCurrentSession()
				fmt.Println(c(dim) + "\n👋 NootyCLI session auto-saved. Goodbye!" + c(reset))
				os.Exit(0)
			}
		}
	}()
}

// ==========================================
// 🛡️⚡ Anti-Sanction Smart DNS Racing Engine
// ==========================================
func raceFastestDNS() {
	type raceResult struct {
		resolver DNSResolver
		latency  time.Duration
		err      error
	}

	resultChan := make(chan raceResult, len(dnsResolvers))
	var wg sync.WaitGroup

	for _, res := range dnsResolvers {
		wg.Add(1)
		go func(r DNSResolver) {
			defer wg.Done()
			start := time.Now()
			
			var dialer *net.Dialer
			if r.Address == "" {
				dialer = &net.Dialer{Timeout: 1200 * time.Millisecond}
			} else {
				customResolver := &net.Resolver{
					PreferGo: true,
					Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
						d := net.Dialer{Timeout: 1200 * time.Millisecond}
						return d.DialContext(ctx, network, r.Address+":53")
					},
				}
				dialer = &net.Dialer{Resolver: customResolver, Timeout: 1200 * time.Millisecond}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
			defer cancel()

			conn, err := dialer.DialContext(ctx, "tcp", "api.openai.com:443")
			lat := time.Since(start)

			if err == nil {
				_ = conn.Close()
				resultChan <- raceResult{resolver: r, latency: lat, err: nil}
			} else {
				resultChan <- raceResult{resolver: r, latency: lat, err: err}
			}
		}(res)
	}

	wg.Wait()
	close(resultChan)

	var workingResolvers []raceResult
	for res := range resultChan {
		if res.err == nil {
			workingResolvers = append(workingResolvers, res)
		}
	}

	if len(workingResolvers) > 0 {
		sort.Slice(workingResolvers, func(i, j int) bool {
			return workingResolvers[i].latency < workingResolvers[j].latency
		})
		activeDNS = workingResolvers[0].resolver
		activeDNS.Latency = workingResolvers[0].latency
		activeDNS.Working = true
	} else {
		activeDNS = dnsResolvers[0] // fallback to system default
	}
}

func setupHTTPClient() {
	var dialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

	if activeDNS.Address == "" {
		dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
		dialContextFunc = dialer.DialContext
	} else {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 3 * time.Second}
				return d.DialContext(ctx, network, activeDNS.Address+":53")
			},
		}
		dialer := &net.Dialer{Resolver: resolver, Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
		dialContextFunc = dialer.DialContext
	}

	globalTransport = &http.Transport{
		DialContext:         dialContextFunc,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression: false,
	}

	globalHTTPClient = &http.Client{
		Transport: globalTransport,
		Timeout:   120 * time.Second,
	}
}

func doWithRetry(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, url, reqBody)
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := globalHTTPClient.Do(req)
		if err == nil {
			if resp.StatusCode < 500 && resp.StatusCode != 403 && resp.StatusCode != 451 {
				return resp, nil
			}
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("HTTP status: %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		// Exponential backoff: 500ms, 1500ms
		if attempt < 2 {
			time.Sleep(time.Duration(1<<attempt*500) * time.Millisecond)
		}
	}
	return nil, fmt.Errorf("request failed after 3 retries: %v", lastErr)
}

// ==========================================
// 🖥️ UI & Minimalist Sci-Fi Header
// ==========================================
func drawHeader() {
	width := 68
	line := strings.Repeat("─", width-2)

	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), centerText("⚡ NOOTY CLI v0.3 ⚡", width-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), centerText("Radin Ultra — Agentic Terminal Intelligence Engine", width-2), c(cyan), c(reset))
	fmt.Println(c(cyan) + "├" + line + "┤" + c(reset))

	dnsInfo := activeDNS.Name
	if activeDNS.Latency > 0 {
		dnsInfo = fmt.Sprintf("%s (%dms)", activeDNS.Name, activeDNS.Latency.Milliseconds())
	}

	entries := [][]string{
		{"Model", config.Model},
		{"Provider", truncateString(config.ProviderEndpoint, 38)},
		{"Workspace", truncateString(formatPath(workspace), 38)},
		{"DNS Shield", dnsInfo},
		{"Tokens", fmt.Sprintf("~%d tokens in context", estimateTokens(sessionMessages))},
		{"Active Mode", strings.ToUpper(currentMode) + " Mode"},
	}

	for _, e := range entries {
		val := fmt.Sprintf("%-38s", e[1])
		fmt.Printf("%s│%s %-12s: %s%s %s│%s\n",
			c(cyan), c(bold)+c(white), e[0], c(green), val, c(cyan), c(reset))
	}
	fmt.Println(c(cyan) + "└" + line + "┘" + c(reset))
	fmt.Printf("%s💡 Type %s/help%s for commands | %s/mode cli%s for Agent Mode | Use %s\"\"\"%s for multiline.%s\n\n",
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

// ==========================================
// ⌨️ Interactive REPL with Multiline Support
// ==========================================
func repl() {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(prompt())
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()

		// Multiline parsing support: """ or <<<
		if strings.HasPrefix(strings.TrimSpace(line), `"""`) || strings.HasPrefix(strings.TrimSpace(line), `<<<`) {
			line = readMultilineInput(scanner, strings.TrimSpace(line)[:3])
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			handleSlashCommand(line)
		} else if strings.HasPrefix(line, "!") && currentMode == "cli" {
			handleShellBang(line[1:])
		} else {
			// Auto inject @file or @dir mentions
			expandedInput := injectContextMentions(line)
			handleChat(expandedInput)
			saveCurrentSession()
		}
	}
	saveCurrentSession()
	fmt.Println(c(dim) + "\n👋 NootyCLI session ended. State saved!" + c(reset))
}

func readMultilineInput(scanner *bufio.Scanner, delimiter string) string {
	var sb strings.Builder
	fmt.Println(c(dim) + "📝 [Multiline Input Mode - type " + delimiter + " to finish]" + c(reset))
	for {
		fmt.Print(c(yellow) + "... " + c(reset))
		if !scanner.Scan() {
			break
		}
		text := scanner.Text()
		if strings.TrimSpace(text) == delimiter {
			break
		}
		sb.WriteString(text + "\n")
	}
	return sb.String()
}

func prompt() string {
	if currentMode == "cli" {
		return c(bold) + c(cyan) + "🤖 nooty[agent]" + c(yellow) + " ❯ " + c(reset)
	}
	return c(bold) + c(green) + "⚡ nooty" + c(white) + " ❯ " + c(reset)
}

// ==========================================
// 🧠 Context Injection & Compaction
// ==========================================
func injectContextMentions(input string) string {
	re := regexp.MustCompile(`@([a-zA-Z0-9_.\-\/\\]+)`)
	matches := re.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return input
	}

	var additions strings.Builder
	seen := make(map[string]bool)

	for _, m := range matches {
		target := m[1]
		if seen[target] {
			continue
		}
		seen[target] = true

		fullPath := safeJoin(workspace, target)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			treeOut := dirTree(fullPath, "")
			additions.WriteString(fmt.Sprintf("\n\n--- Content of Directory @%s ---\n%s\n", target, treeOut))
			fmt.Printf("%s📁 Injected directory context: @%s%s\n", c(dim), target, c(reset))
		} else if info.Size() < 500_000 {
			data, err := os.ReadFile(fullPath)
			if err == nil {
				additions.WriteString(fmt.Sprintf("\n\n--- Content of File @%s ---\n
```\n%s\n
```\n", target, string(data)))
				fmt.Printf("%s📄 Injected file context: @%s (%d bytes)%s\n", c(dim), target, len(data), c(reset))
			}
		}
	}

	return input + additions.String()
}

func estimateTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += (len(m.Content) / 4) + 4
	}
	return total
}

func compactHistory(force bool) {
	if len(sessionMessages) < 6 && !force {
		return
	}
	fmt.Print(c(yellow) + "🧠 Compacting conversation context... " + c(reset))

	older := sessionMessages[:len(sessionMessages)-3]
	recent := sessionMessages[len(sessionMessages)-3:]

	var textToSummarize strings.Builder
	for _, m := range older {
		textToSummarize.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}

	summaryPrompt := []Message{
		{Role: "system", Content: "You are an expert summarizer. Compress the following conversation log into a highly dense context paragraph (max 200 words), highlighting key decisions, code changes, and task state."},
		{Role: "user", Content: textToSummarize.String()},
	}

	summaryText, err := getModelResponseText(summaryPrompt)
	if err != nil {
		fmt.Printf("%s❌ Compaction failed: %v%s\n", c(red), err, c(reset))
		return
	}

	sessionMessages = append([]Message{
		{Role: "system", Content: "Previous Context Summary:\n" + summaryText},
	}, recent...)

	fmt.Printf("%sDone! Token footprint reduced to ~%d tokens.%s\n", c(green), estimateTokens(sessionMessages), c(reset))
}

// ==========================================
// ⏪ Checkpoint & Undo Engine
// ==========================================
func createCheckpoint(targetPath string) {
	absPath := safeJoin(workspace, targetPath)
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return
	}

	timestamp := time.Now().Format("20060102_150405_000")
	backupName := fmt.Sprintf("%s_%s.bak", timestamp, filepath.Base(absPath))
	backupFullPath := filepath.Join(checkpointsDir, backupName)

	if err := os.WriteFile(backupFullPath, data, 0644); err == nil {
		checkpoints = append(checkpoints, Checkpoint{
			OriginalPath: absPath,
			BackupPath:   backupFullPath,
			Timestamp:    time.Now(),
		})
	}
}

func undoLastCheckpoint() {
	if len(checkpoints) == 0 {
		fmt.Println(c(yellow) + "⚠️ No recent file checkpoints found to undo." + c(reset))
		return
	}

	last := checkpoints[len(checkpoints)-1]
	checkpoints = checkpoints[:len(checkpoints)-1]

	data, err := os.ReadFile(last.BackupPath)
	if err != nil {
		fmt.Printf("%s❌ Undo error: Failed to read backup file: %v%s\n", c(red), err, c(reset))
		return
	}

	if err := os.WriteFile(last.OriginalPath, data, 0644); err != nil {
		fmt.Printf("%s❌ Undo error: Failed to restore file: %v%s\n", c(red), err, c(reset))
		return
	}

	_ = os.Remove(last.BackupPath)
	relPath, _ := filepath.Rel(workspace, last.OriginalPath)
	fmt.Printf("%s✅ Undo successful: Reverted %s to checkpoint from %s%s\n",
		c(green)+c(bold), relPath, last.Timestamp.Format("15:04:05"), c(reset))
}

// ==========================================
// 🎨 Interactive Diff Engine
// ==========================================
func showDiffPreview(oldContent, newContent, filePath string) {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	fmt.Printf("\n%s%s🔍 Proposed Diff for %s:%s\n", c(bold), c(yellow), filePath, c(reset))
	
	oldSet := make(map[string]bool)
	for _, l := range oldLines {
		oldSet[strings.TrimSpace(l)] = true
	}

	newSet := make(map[string]bool)
	for _, l := range newLines {
		newSet[strings.TrimSpace(l)] = true
	}

	for _, l := range oldLines {
		if strings.TrimSpace(l) != "" && !newSet[strings.TrimSpace(l)] {
			fmt.Printf("%s- %s%s\n", c(red), l, c(reset))
		}
	}
	for _, l := range newLines {
		if strings.TrimSpace(l) != "" && !oldSet[strings.TrimSpace(l)] {
			fmt.Printf("%s+ %s%s\n", c(green), l, c(reset))
		}
	}
	fmt.Println()
}

// ==========================================
// 🎮 Command Dispatcher
// ==========================================
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
			fmt.Println(c(green) + "🛠 Switched to Autonomous Agent Mode." + c(reset))
		} else {
			currentMode = "chat"
			fmt.Println(c(green) + "💬 Switched to Conversational Chat Mode." + c(reset))
		}
	case "/undo":
		undoLastCheckpoint()
	case "/compact":
		compactHistory(true)
	case "/commit":
		handleQuickCommit()
	case "/sessions":
		listSessions()
	case "/resume":
		if len(parts) < 2 {
			fmt.Println("Usage: /resume <session_id>")
			return
		}
		if err := loadSession(parts[1]); err == nil {
			fmt.Printf("%s✅ Session %s restored!%s\n", c(green), parts[1], c(reset))
		} else {
			fmt.Printf("%s❌ Session not found: %v%s\n", c(red), err, c(reset))
		}
	case "/export":
		exportSessionToMarkdown()
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
	case "/clear":
		sessionMessages = nil
		checkpoints = nil
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
		fmt.Printf("❌ Unknown command: %s. Type /help for options.\n", parts[0])
	}
}

func printHelp() {
	fmt.Println(c(bold) + "\n📌 NootyCLI v0.3 Command Reference:" + c(reset))
	fmt.Println(`
  /help                        Show full command reference overview
  /mode [chat|cli]             Toggle Conversational Chat or Agent Mode
  /undo                        Revert the last file modification
  /compact                     Force AI compaction on session history
  /commit                      Generate & execute Conventional Git Commit
  /sessions                    List all saved conversation sessions
  /resume <session_id>         Load and resume a previous conversation
  /export                      Export conversation to formatted Markdown
  /config                      Interactive wizard for API keys & model
  /workspace show|set <path>   Manage current working directory
  /model show|set|list         View, select, or browse models interactively
  /dns                         Display Anti-Sanction Smart DNS Shield status
  /doctor                      Run full connection & diagnostic check
  /memory list|add|forget      Manage long-term persistent agent memories
  /safety strict|balanced      Configure agent tool confirmation policies
  /history                     Display conversation session log
  /clear                       Clear screen & current session context
  /exit                        Safely save session and exit

  💡 Context Injection: Use @filename or @dirname inside any prompt!
  💡 Multiline Prompt : Start with """ or <<< and finish with matching tag.
  💡 Agent Shell Bang : Prefix shell commands with ! in CLI Mode.`)
}

func handleQuickCommit() {
	cmd := exec.Command("git", "diff", "--staged")
	cmd.Dir = workspace
	out, _ := cmd.Output()
	if len(out) == 0 {
		cmd = exec.Command("git", "diff")
		cmd.Dir = workspace
		out, _ = cmd.Output()
	}

	if len(out) == 0 {
		fmt.Println(c(yellow) + "⚠️ No changes detected in git workspace." + c(reset))
		return
	}

	diffText := string(out)
	if len(diffText) > 4000 {
		diffText = diffText[:4000]
	}

	prompt := []Message{
		{Role: "system", Content: "You are a git commit expert. Write a clean Conventional Commit message (e.g., feat:, fix:, refactor:) based on the git diff. Return ONLY the commit message."},
		{Role: "user", Content: "Git Diff:\n" + diffText},
	}

	msg, err := getModelResponseText(prompt)
	if err != nil {
		fmt.Printf("%s❌ Commit generation error: %v%s\n", c(red), err, c(reset))
		return
	}
	msg = strings.TrimSpace(msg)

	fmt.Printf("\n%s📝 Proposed Commit Message:%s\n%s\n\n", c(bold)+c(cyan), c(reset), msg)
	fmt.Print("Commit staged changes with this message? [Y/n]: ")
	reader := bufio.NewReader(os.Stdin)
	confirm, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(confirm)) != "n" {
		c1 := exec.Command("git", "add", "-A")
		c1.Dir = workspace
		_ = c1.Run()

		c2 := exec.Command("git", "commit", "-m", msg)
		c2.Dir = workspace
		c2.Stdout = os.Stdout
		c2.Stderr = os.Stderr
		if err := c2.Run(); err == nil {
			fmt.Println(c(green) + "✅ Changes committed successfully!" + c(reset))
		}
	}
}

// ==========================================
// 🌀 Thinking Spinner & Telemetry
// ==========================================
func showSpinner(done <-chan struct{}) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	for {
		select {
		case <-done:
			fmt.Print("\r\033[K")
			return
		default:
			fmt.Printf("\r%s%s Thinking...%s", c(cyan), frames[i%len(frames)], c(reset))
			time.Sleep(80 * time.Millisecond)
			i++
		}
	}
}

func printTelemetry(charCount int, elapsed time.Duration) {
	seconds := elapsed.Seconds()
	if seconds <= 0 {
		seconds = 0.01
	}
	approxTokens := charCount / 4
	tokPerSec := float64(approxTokens) / seconds

	fmt.Printf("%s⚡ ~%d tokens | %.1f tok/s | %.2fs | Model: %s%s\n\n",
		c(dim), approxTokens, tokPerSec, seconds, config.Model, c(reset))
}

// ==========================================
// 🤖 Agent Loop with Self-Correction & Batching
// ==========================================
func handleChat(input string) {
	messages := buildMessages(input)
	if currentMode == "cli" {
		runAgentLoop(messages)
		return
	}
	streamResponse(messages)
}

func buildMessages(userInput string) []Message {
	var msgs []Message
	sysPrompt := `You are NootyCLI v0.3 "Radin Ultra", an autonomous AI engineering assistant.

When in CHAT mode: Provide concise, high-impact terminal and software engineering answers.

When in CLI mode: Act as an autonomous coding agent.
To execute tools, reply STRICTLY using this exact syntax:
TOOL: tool_name key1="value1" key2="value2"

You can issue MULTIPLE TOOL calls in one response for parallel batch execution when independent (e.g. reading multiple files).

Available Tools:
- patch_file (path="relative_path", old_str="exact_match", new_str="replacement")
- replace_in_file (path="relative_path", old_str="exact_match", new_str="replacement")
- append_file (path="relative_path", content="text_to_append")
- write_file (path="relative_path", content="full_content")
- read_file (path="relative_path")
- list_files (path="relative_path")
- tree (path="relative_path")
- find_files (pattern="*.go")
- search_code (query="text", path="relative_path")
- delete_file (path="relative_path")
- file_info (path="relative_path")
- git_status
- git_diff
- run_command (command="shell_cmd", timeout="seconds")
- run_and_verify (command="shell_cmd")

PREFER 'patch_file' or 'replace_in_file' over 'write_file' for existing files.`

	relevant := getRelevantMemories(userInput)
	if len(relevant) > 0 {
		sysPrompt += "\n\nUser Context & Memories:\n"
		for _, m := range relevant {
			sysPrompt += fmt.Sprintf("- [%s] %s\n", m.Tag, m.Content)
		}
	}

	msgs = append(msgs, Message{Role: "system", Content: sysPrompt})

	// Check token budget and auto-compact if necessary
	if config.AutoCompact && estimateTokens(sessionMessages) > config.MaxTokensContext {
		compactHistory(false)
	}

	msgs = append(msgs, sessionMessages...)
	userMsg := Message{Role: "user", Content: userInput}
	msgs = append(msgs, userMsg)
	sessionMessages = append(sessionMessages, userMsg)

	return msgs
}

func streamResponse(messages []Message) {
	reqPayload := ChatRequest{Model: config.Model, Messages: messages, Stream: true}
	jsonData, _ := json.Marshal(reqPayload)
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}

	doneSpinner := make(chan struct{})
	go showSpinner(doneSpinner)

	start := time.Now()
	resp, err := doWithRetry("POST", endpoint, jsonData, headers)
	close(doneSpinner)

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
	charCount := 0

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
				content := choice.Delta.Content
				fmt.Print(content)
				fullContent.WriteString(content)
				charCount += len(content)
			}
		}
	}
	fmt.Print(c(reset) + "\n")
	printTelemetry(charCount, time.Since(start))

	sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: fullContent.String()})
}

func runAgentLoop(messages []Message) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s⚠️ Agent loop recovered from crash: %v. Session preserved.%s\n", c(red), r, c(reset))
		}
	}()

	agentRunning = true
	var ctx context.Context
	ctx, agentCancelFunc = context.WithCancel(context.Background())
	defer func() { agentRunning = false }()

	msgs := append([]Message{}, messages...)

	for step := 0; step < 12; step++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		doneSpinner := make(chan struct{})
		go showSpinner(doneSpinner)

		respText, err := getModelResponseText(msgs)
		close(doneSpinner)

		if err != nil {
			fmt.Printf("%s❌ Agent Model Error: %v%s\n", c(red), err, c(reset))
			return
		}

		toolCalls := extractAllToolCalls(respText)
		if len(toolCalls) == 0 {
			fmt.Println("\n" + c(green) + respText + c(reset) + "\n")
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText})
			return
		}

		fmt.Printf("\n%s⚡ Agent Actions Step [%d] (%d tools emitted):%s\n", c(bold)+c(yellow), step+1, len(toolCalls), c(reset))

		// Batch Parallel Execution for Read-Only tools; Sequential for Mutating tools
		results := executeToolBatch(toolCalls)

		var toolFeedback strings.Builder
		for _, res := range results {
			if res.Err != nil {
				fmt.Printf("   %s❌ Tool '%s' Error: %v%s\n", c(red), res.Name, res.Err, c(reset))
				toolFeedback.WriteString(fmt.Sprintf("TOOL '%s' ERROR: %v\n", res.Name, res.Err))
			} else {
				fmt.Printf("   %s✅ Tool '%s' executed successfully.%s\n", c(green), res.Name, c(reset))
				toolFeedback.WriteString(fmt.Sprintf("TOOL '%s' RESULT:\n%s\n", res.Name, res.Output))
			}
		}

		feedbackStr := toolFeedback.String()
		if len(feedbackStr) > 4000 {
			feedbackStr = feedbackStr[:4000] + "\n... (output truncated)"
		}

		msgs = append(msgs,
			Message{Role: "assistant", Content: respText},
			Message{Role: "user", Content: feedbackStr},
		)
	}

	fmt.Printf("%s⚠️ Agent reached maximum step threshold (12).%s\n", c(yellow), c(reset))
}

func extractAllToolCalls(text string) []*ToolCall {
	var calls []*ToolCall
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TOOL:") || strings.HasPrefix(line, "TOOL：") {
			if tc := parseToolLine(line); tc != nil {
				calls = append(calls, tc)
			}
		}
	}
	return calls
}

func executeToolBatch(tools []*ToolCall) []ToolResult {
	results := make([]ToolResult, len(tools))
	var wg sync.WaitGroup

	isReadOnly := func(name string) bool {
		switch name {
		case "read_file", "list_files", "tree", "find_files", "search_code", "file_info", "git_status", "git_diff":
			return true
		default:
			return false
		}
	}

	for i, tc := range tools {
		if isReadOnly(tc.Name) {
			wg.Add(1)
			go func(idx int, call *ToolCall) {
				defer wg.Done()
				out, err := runTool(call.Name, call.Args)
				results[idx] = ToolResult{Name: call.Name, Output: out, Err: err}
			}(i, tc)
		} else {
			// Wait for parallel reads before running state-mutating actions
			wg.Wait()
			approved := confirmToolExecution(tc)
			if !approved {
				results[i] = ToolResult{Name: tc.Name, Output: "Action rejected by user safety policy.", Err: nil}
				continue
			}
			out, err := runTool(tc.Name, tc.Args)
			results[i] = ToolResult{Name: tc.Name, Output: out, Err: err}
		}
	}

	wg.Wait()
	return results
}

func confirmToolExecution(tc *ToolCall) bool {
	if config.Safety == "balanced" && tc.Name != "delete_file" && tc.Name != "run_command" {
		return true
	}

	fmt.Printf("%s⚠️ Action Confirmation Needed [%s]:%s\n", c(bold)+c(yellow), tc.Name, c(reset))
	for k, v := range tc.Args {
		if len(v) > 60 {
			v = v[:57] + "..."
		}
		fmt.Printf("   %s%s%s: %s\n", c(dim), k, c(reset), v)
	}

	if tc.Name == "delete_file" {
		fmt.Print(c(red) + "Type 'DELETE' to confirm file removal: " + c(reset))
		reader := bufio.NewReader(os.Stdin)
		ans, _ := reader.ReadString('\n')
		return strings.TrimSpace(ans) == "DELETE"
	}

	fmt.Print("Approve execution? [Y/n]: ")
	reader := bufio.NewReader(os.Stdin)
	ans, _ := reader.ReadString('\n')
	ans = strings.TrimSpace(strings.ToLower(ans))
	return ans != "n" && ans != "no"
}

// ==========================================
// 🛠️ Tool Execution & Implementation
// ==========================================
func runTool(name string, args map[string]string) (string, error) {
	switch name {
	case "patch_file", "replace_in_file":
		relPath := args["path"]
		targetPath := safeJoin(workspace, relPath)
		oldStr := args["old_str"]
		if oldStr == "" {
			oldStr = args["search"]
		}
		newStr := args["new_str"]
		if newStr == "" {
			newStr = args["replace"]
		}

		data, err := os.ReadFile(targetPath)
		if err != nil {
			return "", err
		}
		content := string(data)
		if !strings.Contains(content, oldStr) {
			return "", fmt.Errorf("target old_str block not found in %s", relPath)
		}

		createCheckpoint(relPath)
		updated := strings.Replace(content, oldStr, newStr, 1)
		showDiffPreview(content, updated, relPath)

		if err := os.WriteFile(targetPath, []byte(updated), 0644); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Successfully patched %s", relPath), nil

	case "append_file":
		relPath := args["path"]
		targetPath := safeJoin(workspace, relPath)
		content := args["content"]

		createCheckpoint(relPath)
		f, err := os.OpenFile(targetPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return "", err
		}
		defer f.Close()
		if _, err := f.WriteString(content); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Appended %d bytes to %s", len(content), relPath), nil

	case "write_file", "create_file":
		relPath := args["path"]
		targetPath := safeJoin(workspace, relPath)
		content := args["content"]

		createCheckpoint(relPath)
		_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
		if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ File written (%d bytes): %s", len(content), relPath), nil

	case "read_file":
		targetPath := safeJoin(workspace, args["path"])
		data, err := os.ReadFile(targetPath)
		if err != nil {
			return "", err
		}
		return string(data), nil

	case "delete_file":
		relPath := args["path"]
		targetPath := safeJoin(workspace, relPath)
		createCheckpoint(relPath)
		if err := os.Remove(targetPath); err != nil {
			return "", err
		}
		return "✅ File removed: " + relPath, nil

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
			if shouldIgnorePath(e.Name()) {
				continue
			}
			if e.IsDir() {
				names = append(names, e.Name()+"/")
			} else {
				names = append(names, e.Name())
			}
		}
		return strings.Join(names, "\n"), nil

	case "tree":
		path := workspace
		if p, ok := args["path"]; ok && p != "" && p != "." {
			path = safeJoin(workspace, p)
		}
		return dirTree(path, ""), nil

	case "find_files":
		pattern := args["pattern"]
		if pattern == "" {
			pattern = "*"
		}
		var matched []string
		_ = filepath.Walk(workspace, func(p string, info os.FileInfo, err error) error {
			if err != nil || shouldIgnorePath(info.Name()) {
				if info != nil && info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(workspace, p)
			if match, _ := filepath.Match(pattern, info.Name()); match {
				matched = append(matched, rel)
			}
			return nil
		})
		if len(matched) == 0 {
			return "No files matched pattern.", nil
		}
		return strings.Join(matched, "\n"), nil

	case "search_code":
		query := args["query"]
		scope := workspace
		if s, ok := args["path"]; ok && s != "" {
			scope = safeJoin(workspace, s)
		}
		var results []string
		_ = filepath.Walk(scope, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || info.Size() > 1_500_000 || shouldIgnorePath(info.Name()) {
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
						return filepath.SkipAll
					}
				}
				lineNum++
			}
			return nil
		})
		if len(results) == 0 {
			return "No matching code references found.", nil
		}
		return strings.Join(results, "\n"), nil

	case "file_info":
		targetPath := safeJoin(workspace, args["path"])
		info, err := os.Stat(targetPath)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Path: %s\nSize: %d bytes\nMode: %s\nModified: %s",
			args["path"], info.Size(), info.Mode(), info.ModTime().Format(time.RFC3339)), nil

	case "git_status":
		cmd := exec.Command("git", "status", "--short")
		cmd.Dir = workspace
		out, err := cmd.Output()
		if err != nil {
			return "", err
		}
		if len(out) == 0 {
			return "(working directory clean)", nil
		}
		return string(out), nil

	case "git_diff":
		cmd := exec.Command("git", "diff")
		cmd.Dir = workspace
		out, err := cmd.Output()
		if err != nil {
			return "", err
		}
		if len(out) == 0 {
			return "(no uncommitted diffs)", nil
		}
		return string(out), nil

	case "run_command", "run_and_verify":
		cmdStr := args["command"]
		timeout := 90
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

		var fullOutput bytes.Buffer
		mw := io.MultiWriter(os.Stdout, &fullOutput)
		cmd.Stdout = mw
		cmd.Stderr = mw

		fmt.Printf("%s🚀 [Live Running]: %s%s\n", c(dim), cmdStr, c(reset))
		if err := cmd.Start(); err != nil {
			return "", err
		}

		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		select {
		case err := <-done:
			out := fullOutput.String()
			if err != nil {
				return out + fmt.Sprintf("\n[Exit Code Error: %v]", err), nil
			}
			if out == "" {
				out = "(command finished with exit status 0, no output)"
			}
			return out, nil
		case <-time.After(time.Duration(timeout) * time.Second):
			_ = cmd.Process.Kill()
			return "", fmt.Errorf("command execution timed out (%d sec)", timeout)
		}
	}

	return "", fmt.Errorf("unknown tool: %s", name)
}

func shouldIgnorePath(name string) bool {
	ignoreList := []string{".git", "node_modules", "vendor", ".idea", ".vscode", "dist", "build", "bin", "obj", ".DS_Store"}
	for _, ign := range ignoreList {
		if name == ign {
			return true
		}
	}
	return false
}

func dirTree(root, indent string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err.Error()
	}
	var out string
	for i, e := range entries {
		if shouldIgnorePath(e.Name()) {
			continue
		}
		prefix := indent + "├── "
		childIndent := indent + "│   "
		if i == len(entries)-1 {
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

// ==========================================
// ⚡ Model Communication Engine
// ==========================================
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
		return "", fmt.Errorf("empty choices array received")
	}

	return result.Choices[0].Message.Content, nil
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

	args := map[string]string{}
	re := regexp.MustCompile(`(\w+)=("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|\S+)`)
	matches := re.FindAllStringSubmatch(argsStr, -1)

	for _, match := range matches {
		if len(match) == 3 {
			key := match[1]
			val := match[2]
			if (strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`)) ||
				(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				val = val[1 : len(val)-1]
			}
			val = strings.ReplaceAll(val, "\\n", "\n")
			val = strings.ReplaceAll(val, "\\t", "\t")
			val = strings.ReplaceAll(val, `\"`, `"`)
			args[key] = val
		}
	}

	if len(args) == 0 && strings.TrimSpace(argsStr) != "" {
		args["path"] = strings.TrimSpace(argsStr)
	}

	return &ToolCall{Name: name, Args: args}
}

// ==========================================
// 💾 Session Persistence & Export
// ==========================================
func createNewSession() {
	id := time.Now().Format("20060102-150405")
	currentSession = SessionData{
		ID:        id,
		Name:      "Session-" + id,
		Mode:      currentMode,
		Created:   time.Now(),
		Updated:   time.Now(),
		Workspace: workspace,
		Messages:  []Message{},
	}
	sessionMessages = []Message{}
}

func saveCurrentSession() {
	if len(sessionMessages) == 0 {
		return
	}
	currentSession.Messages = sessionMessages
	currentSession.Updated = time.Now()
	currentSession.Workspace = workspace
	currentSession.Mode = currentMode

	filePath := filepath.Join(chatsDir, currentSession.ID+".json")
	data, err := json.MarshalIndent(currentSession, "", "  ")
	if err == nil {
		_ = os.WriteFile(filePath, data, 0600)
	}
}

func loadSession(id string) error {
	filePath := filepath.Join(chatsDir, id+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	var s SessionData
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	currentSession = s
	sessionMessages = s.Messages
	currentMode = s.Mode
	if s.Workspace != "" {
		workspace = s.Workspace
	}
	return nil
}

func listSessions() {
	files, err := os.ReadDir(chatsDir)
	if err != nil || len(files) == 0 {
		fmt.Println("💾 No saved sessions found.")
		return
	}

	fmt.Println(c(bold) + "\n📂 Saved Sessions:" + c(reset))
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			id := strings.TrimSuffix(f.Name(), ".json")
			fmt.Printf("  • %s%s%s (Resume: /resume %s)\n", c(cyan), id, c(reset), id)
		}
	}
	fmt.Println()
}

func exportSessionToMarkdown() {
	if len(sessionMessages) == 0 {
		fmt.Println("⚠️ Session is empty, nothing to export.")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# NootyCLI Chat Export - %s\n\n", currentSession.ID))
	sb.WriteString(fmt.Sprintf("**Date:** %s | **Model:** %s | **Workspace:** `%s`\n\n---\n\n",
		time.Now().Format(time.RFC822), config.Model, workspace))

	for _, m := range sessionMessages {
		if m.Role == "user" {
			sb.WriteString("### 👤 User:\n" + m.Content + "\n\n")
		} else if m.Role == "assistant" {
			sb.WriteString("### 🤖 Nooty:\n" + m.Content + "\n\n---\n\n")
		}
	}

	exportFile := filepath.Join(chatsDir, fmt.Sprintf("export_%s.md", currentSession.ID))
	if err := os.WriteFile(exportFile, []byte(sb.String()), 0644); err == nil {
		fmt.Printf("%s✅ Exported chat to: %s%s\n", c(green), exportFile, c(reset))
	}
}

// ==========================================
// 🛠️ Shell Bang & Utility Handlers
// ==========================================
func handleShellBang(cmd string) {
	cmd = strings.TrimSpace(cmd)
	fmt.Printf("\n%s⚡ Direct Shell Exec: %s%s\n", c(yellow), cmd, c(reset))
	var cExec *exec.Cmd
	if runtime.GOOS == "windows" {
		cExec = exec.Command("cmd", "/C", cmd)
	} else {
		cExec = exec.Command("sh", "-c", cmd)
	}
	cExec.Dir = workspace
	cExec.Stdout = os.Stdout
	cExec.Stderr = os.Stderr
	_ = cExec.Run()
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
		fmt.Println("❌ Unknown subcommand. Use: show | set <path>")
	}
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
	}
}

func selectModelInteractive() {
	fmt.Println("🔍 Fetching available models...")
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/models"
	headers := map[string]string{}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}

	resp, err := doWithRetry("GET", endpoint, nil, headers)
	if err != nil {
		fmt.Printf("%s❌ Model fetch error: %v%s\n", c(red), err, c(reset))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Data) == 0 {
		fmt.Println("⚠️ Provider returned no models.")
		return
	}

	fmt.Println(c(bold) + "\n📋 Available Models:" + c(reset))
	for i, m := range result.Data {
		if i >= 20 {
			break
		}
		fmt.Printf("  [%2d] %s\n", i+1, m.ID)
	}
	fmt.Print("\nSelect model number (or press Enter to cancel): ")
	reader := bufio.NewReader(os.Stdin)
	in, _ := reader.ReadString('\n')
	if num, err := strconv.Atoi(strings.TrimSpace(in)); err == nil && num >= 1 && num <= len(result.Data) {
		config.Model = result.Data[num-1].ID
		saveConfig()
		fmt.Printf("%s✅ Model switched to: %s%s\n", c(green), config.Model, c(reset))
	}
}

func handleConfig() {
	fmt.Println(c(bold) + "\n⚙️ Nooty Configuration Wizard" + c(reset))
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Provider endpoint [%s]: ", config.ProviderEndpoint)
	if ep, _ := reader.ReadString('\n'); strings.TrimSpace(ep) != "" {
		config.ProviderEndpoint = strings.TrimSpace(ep)
	}

	fmt.Printf("API key [%s]: ", maskAPIKey(config.APIKey))
	if key, _ := reader.ReadString('\n'); strings.TrimSpace(key) != "" {
		config.APIKey = strings.TrimSpace(key)
	}

	fmt.Printf("Model [%s]: ", config.Model)
	if mod, _ := reader.ReadString('\n'); strings.TrimSpace(mod) != "" {
		config.Model = strings.TrimSpace(mod)
	}

	saveConfig()
	fmt.Println(c(green) + "✅ Configuration saved!\n" + c(reset))
}

func showDNSStatus() {
	fmt.Println(c(bold) + "\n🛡️ Anti-Sanction Smart DNS Status:" + c(reset))
	for i, dns := range dnsResolvers {
		status := ""
		if dns.Name == activeDNS.Name {
			status = c(green) + " [ACTIVE]" + c(reset)
		}
		addr := dns.Address
		if addr == "" {
			addr = "System Default"
		}
		fmt.Printf("  %d. %-24s (%s)%s\n", i+1, dns.Name, addr, status)
	}
	fmt.Println()
}

func runDoctor() {
	fmt.Println(c(bold) + "\n🏥 Nooty Diagnostic Doctor" + c(reset))
	fmt.Printf("• Provider Endpoint : %s\n", config.ProviderEndpoint)
	fmt.Printf("• Active Model      : %s\n", config.Model)
	fmt.Printf("• API Key           : %s\n", maskAPIKey(config.APIKey))
	fmt.Printf("• Workspace         : %s\n", formatPath(workspace))
	fmt.Printf("• Active DNS Shield : %s (%s)\n", activeDNS.Name, activeDNS.Address)
	fmt.Print("• Provider Ping     : ")

	start := time.Now()
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/models"
	headers := map[string]string{}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}

	resp, err := doWithRetry("GET", endpoint, nil, headers)
	if err != nil {
		fmt.Printf("%sFAILED (%v)%s\n\n", c(red), err, c(reset))
	} else {
		defer resp.Body.Close()
		fmt.Printf("%sHEALTHY (Latency: %dms, Status: %d)%s\n\n", c(green), time.Since(start).Milliseconds(), resp.StatusCode, c(reset))
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
			fmt.Println("🧠 No memories stored.")
			return
		}
		fmt.Println(c(bold) + "\n🧠 Persistent Agent Memories:" + c(reset))
		for _, m := range memories {
			fmt.Printf("  [%d] (%s) %s\n", m.ID, m.Tag, m.Content)
		}
		fmt.Println()
	case "add":
		if len(args) < 2 {
			fmt.Println("Usage: /memory add <text>")
			return
		}
		m := Memory{
			ID:      len(memories) + 1,
			Tag:     "context",
			Content: strings.Join(args[1:], " "),
			Added:   time.Now().Format(time.RFC3339),
		}
		memories = append(memories, m)
		saveMemories()
		fmt.Printf("✅ Memory [%d] recorded.\n", m.ID)
	case "forget":
		if len(args) < 2 {
			fmt.Println("Usage: /memory forget <id>")
			return
		}
		id, _ := strconv.Atoi(args[1])
		var filtered []Memory
		for _, m := range memories {
			if m.ID != id {
				filtered = append(filtered, m)
			}
		}
		memories = filtered
		saveMemories()
		fmt.Printf("✅ Memory [%d] removed.\n", id)
	}
}

func getRelevantMemories(query string) []Memory {
	q := strings.ToLower(query)
	var res []Memory
	for _, m := range memories {
		if strings.Contains(strings.ToLower(m.Content), q) || strings.Contains(strings.ToLower(m.Tag), q) {
			res = append(res, m)
		}
	}
	return res
}

func handleSafety(args []string) {
	if len(args) == 0 {
		fmt.Printf("🛡️ Safety Policy: %s\n", config.Safety)
		return
	}
	if args[0] == "strict" || args[0] == "balanced" {
		config.Safety = args[0]
		saveConfig()
		fmt.Printf("✅ Safety updated to: %s\n", config.Safety)
	}
}

func showHistory() {
	if len(sessionMessages) == 0 {
		fmt.Println("💬 History is empty.")
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

// ==========================================
// ⚙️ Configurations & Persistence
// ==========================================
func loadConfig() {
	config = Config{
		ProviderEndpoint: "https://api.openai.com/v1",
		APIKey:           os.Getenv("OPENAI_API_KEY"),
		Model:            "gpt-4o-mini",
		Safety:           "strict",
		AutoCompact:      true,
		MaxTokensContext: 8000,
	}
	if config.APIKey == "" {
		config.APIKey = os.Getenv("NOOTY_API_KEY")
	}

	data, err := os.ReadFile(configFile)
	if err == nil {
		_ = json.Unmarshal(data, &config)
	}
}

func saveConfig() {
	data, _ := json.MarshalIndent(config, "", "  ")
	_ = os.WriteFile(configFile, data, 0600)
}

func loadMemories() {
	data, err := os.ReadFile(memFile)
	if err == nil {
		_ = json.Unmarshal(data, &memories)
	}
}

func saveMemories() {
	data, _ := json.MarshalIndent(memories, "", "  ")
	_ = os.WriteFile(memFile, data, 0600)
}
