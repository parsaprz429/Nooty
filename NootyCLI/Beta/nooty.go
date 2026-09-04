// nooty.go — NootyCLI v0.3 "Radin Supercharged" – Agentic Terminal Intelligence
// Single‑file, zero external dependencies, cross-platform (macOS / Linux / Windows / WSL).
//
// 🚀 Compile & Build:
//    go build -ldflags="-s -w" -o nooty nooty.go
//
// 🛠 Usage:
//    nooty                             → Interactive REPL Mode
//    cat logs.txt | nooty "Find error" → Unix Pipe Mode
//    nooty -p "Refactor main.go"       → Direct Prompt Mode

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
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

type Checkpoint struct {
	FilePath   string    `json:"file_path"`
	BackupPath string    `json:"backup_path"`
	Timestamp  time.Time `json:"timestamp"`
}

type Session struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Messages []Message `json:"messages"`
	Mode     string    `json:"mode"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
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
	chatsDir        string
	checkpointsDir  string
	memFile         string
	configFile      string
	agentRunning    bool
	checkpoints     []Checkpoint
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
	activeDNSAddr = ""

	globalTransport *http.Transport
)

var ignoredDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
	".build":       true,
	"dist":         true,
	"target":       true,
	".idea":        true,
	".vscode":      true,
	"__pycache__":  true,
}

func main() {
	promptFlag := flag.String("p", "", "Direct prompt for non-interactive execution")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "NootyCLI v0.3 — Agentic Terminal Intelligence\n\nUsage:\n  nooty [options]\n\nOptions:\n")
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

	setupGlobalHTTPTransport()
	setupSignalHandler()
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
	currentSessionID = fmt.Sprintf("session_%d", time.Now().Unix())

	raceDNS()

	// Handle Pipe / Non-Interactive Mode
	stat, _ := os.Stdin.Stat()
	isPiped := (stat.Mode() & os.ModeCharDevice) == 0

	if isPiped || *promptFlag != "" {
		var fullPrompt string
		if isPiped {
			pipedData, _ := io.ReadAll(os.Stdin)
			fullPrompt = string(pipedData)
			if *promptFlag != "" {
				fullPrompt += "\n" + *promptFlag
			}
		} else {
			fullPrompt = *promptFlag
		}
		handleChat(strings.TrimSpace(fullPrompt))
		os.Exit(0)
	}

	drawHeader()
	repl()
}

// ---------- Connection Pooling & DNS Racing ----------
func setupGlobalHTTPTransport() {
	globalTransport = &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

func raceDNS() {
	type raceResult struct {
		dns DNSResolver
		dur time.Duration
	}
	ch := make(chan raceResult, len(fallbackDNS))
	var wg sync.WaitGroup

	for _, resolver := range fallbackDNS {
		wg.Add(1)
		go func(d DNSResolver) {
			defer wg.Done()
			start := time.Now()
			target := d.Address
			if target == "" {
				target = "8.8.8.8"
			}
			conn, err := net.DialTimeout("tcp", target+":53", 1200*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				ch <- raceResult{dns: d, dur: time.Since(start)}
			}
		}(resolver)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	select {
	case res, ok := <-ch:
		if ok {
			activeDNSName = res.dns.Name
			activeDNSAddr = res.dns.Address
		}
	case <-time.After(1300 * time.Millisecond):
		// Default to direct if race times out
	}
}

func httpClientForDNS(dns string) *http.Client {
	if dns == "" {
		return &http.Client{
			Transport: globalTransport,
			Timeout:   35 * time.Second,
		}
	}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, network, dns+":53")
		},
	}
	dialer := &net.Dialer{Resolver: resolver}
	t := globalTransport.Clone()
	t.DialContext = dialer.DialContext

	return &http.Client{
		Transport: t,
		Timeout:   35 * time.Second,
	}
}

func doWithRetry(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt < 3; attempt++ {
		resp, err = doWithFallback(method, url, body, headers)
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}
		if attempt < 2 {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
	}
	return resp, err
}

func doWithFallback(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	for i, dnsResolver := range fallbackDNS {
		client := httpClientForDNS(dnsResolver.Address)
		var req *http.Request
		var err error

		if body != nil {
			req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
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
		if err == nil && resp.StatusCode != 403 && resp.StatusCode != 451 {
			activeDNSName = dnsResolver.Name
			activeDNSAddr = dnsResolver.Address
			return resp, nil
		}

		if resp != nil {
			_ = resp.Body.Close()
		}
		if i < len(fallbackDNS)-1 {
			// Auto switch to next DNS
			activeDNSName = fallbackDNS[i+1].Name
			activeDNSAddr = fallbackDNS[i+1].Address
		}
	}
	return nil, fmt.Errorf("network error: all anti-sanction DNS resolvers failed")
}

// ---------- Signal Handling ----------
func setupSignalHandler() {
	cChan := make(chan os.Signal, 1)
	signal.Notify(cChan, os.Interrupt)
	go func() {
		for range cChan {
			if agentRunning {
				fmt.Println(c(yellow) + "\n⏸ Agent loop interrupted by user. Session preserved." + c(reset))
				agentRunning = false
			} else {
				fmt.Println(c(dim) + "\n👋 Goodbye!" + c(reset))
				os.Exit(0)
			}
		}
	}()
}

// ---------- Visual Header & UI Helpers ----------
func drawHeader() {
	width := 66
	line := strings.Repeat("─", width-2)

	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), centerText(" NOOTY CLI ", width-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), centerText("v0.3 Radin Supercharged — Agentic Terminal Intelligence", width-2), c(cyan), c(reset))
	fmt.Println(c(cyan) + "├" + line + "┤" + c(reset))

	prettyWorkspace := formatPath(workspace)
	tokensUsed := estimateTokens(sessionMessages)

	entries := [][]string{
		{"Provider", truncateString(config.ProviderEndpoint, 38)},
		{"Model", config.Model},
		{"API Key", maskAPIKey(config.APIKey)},
		{"Workspace", truncateString(prettyWorkspace, 38)},
		{"DNS Shield", activeDNSName},
		{"Context", fmt.Sprintf("~%d tokens (%d msgs)", tokensUsed, len(sessionMessages))},
		{"Mode", strings.ToUpper(currentMode) + " Mode"},
	}

	for _, e := range entries {
		val := fmt.Sprintf("%-38s", e[1])
		fmt.Printf("%s│%s %-12s: %s%s %s│%s\n",
			c(cyan), c(bold)+c(white), e[0], c(green), val, c(cyan), c(reset))
	}
	fmt.Println(c(cyan) + "└" + line + "┘" + c(reset))
	fmt.Printf("%s💡 Type %s/help%s for commands, %s/mode cli%s for Agent Mode.%s\n\n", c(dim), c(bold)+c(green), c(dim), c(bold)+c(cyan), c(dim), c(reset))
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

func showSpinner(done chan struct{}) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	for {
		select {
		case <-done:
			fmt.Print("\r\033[K")
			return
		default:
			fmt.Printf("\r%s%s Nooty thinking...%s", c(cyan), frames[i%len(frames)], c(reset))
			time.Sleep(80 * time.Millisecond)
			i++
		}
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
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Support Multiline mode with """ or <<<
		if strings.HasPrefix(line, `"""`) || strings.HasPrefix(line, `<<<`) {
			line = readMultilineInput(scanner, line[:3])
		}

		if strings.HasPrefix(line, "/") {
			handleSlashCommand(line)
		} else if strings.HasPrefix(line, "!") && currentMode == "cli" {
			handleShellBang(line[1:])
		} else {
			handleChat(line)
		}
	}
	fmt.Println(c(dim) + "\n👋 NootyCLI session ended. Goodbye!" + c(reset))
}

func readMultilineInput(scanner *bufio.Scanner, delim string) string {
	fmt.Println(c(dim) + " (Multiline input mode active. Type " + delim + " to submit)" + c(reset))
	var lines []string
	for {
		fmt.Print(c(cyan) + "  ... " + c(reset))
		if !scanner.Scan() {
			break
		}
		l := scanner.Text()
		if strings.TrimSpace(l) == delim {
			break
		}
		lines = append(lines, l)
	}
	return strings.Join(lines, "\n")
}

func prompt() string {
	if currentMode == "cli" {
		return c(bold) + c(cyan) + "🤖 nooty[agent]" + c(yellow) + " ❯ " + c(reset)
	}
	return c(bold) + c(green) + "⚡ nooty" + c(white) + " ❯ " + c(reset)
}

func handleShellBang(cmd string) {
	fmt.Printf("%s⚡ Executing direct shell command: %s%s\n", c(dim), cmd, c(reset))
	out, err := executeCommandStream(cmd, 120)
	if err != nil {
		fmt.Printf("%s❌ Error: %v%s\n", c(red), err, c(reset))
	}
	if out != "" {
		fmt.Println(out)
	}
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
			fmt.Println(c(green) + "🛠 Switched to Agent Mode (Autonomous Tools Enabled)." + c(reset))
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
		compactHistory()
	case "/undo":
		handleUndo()
	case "/sessions":
		listSessions()
	case "/session":
		handleSessionCommand(parts[1:])
	case "/resume":
		resumeLastSession()
	case "/export":
		exportChatMarkdown()
	case "/commit":
		generateGitCommit()
	case "/clear":
		sessionMessages = nil
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
		os.Exit(0)
	default:
		fmt.Printf("❌ Unknown command: %s. Type /help for assistance.\n", parts[0])
	}
}

func printHelp() {
	fmt.Println(c(bold) + "\n📌 NootyCLI v0.3 Command Reference:" + c(reset))
	fmt.Println(`
  /help                        Show command help overview
  /mode [chat|cli]             Toggle Chat or Autonomous Agent CLI Execution Mode
  /config                      Interactive wizard for API key, endpoint & model
  /workspace show|set <path>   Manage working directory
  /model show|set <name>|list  View, switch, or browse models interactively
  /compact                     Summarize older session messages to optimize context
  /undo                        Rollback the last file change from backup checkpoint
  /sessions                    List all saved conversation sessions
  /session save|load <name>    Save or restore session state
  /resume                      Resume the most recent conversation session
  /export                      Export chat session to Markdown file
  /commit                      Generate conventional git commit from diff
  /dns                         Display Anti-Sanction Smart DNS Shield status
  /doctor                      Run full connection and API health diagnostic
  /memory list|add|forget      Manage long-term persistent agent context
  /history                     Display conversation session log
  /clear                       Reset current screen & session memory
  /exit                        Terminate NootyCLI session

  💡 Tip: Use @filename in your prompt to embed file contents directly!
  💡 Multi-line: Start input with """ or <<< to enter multi-line mode.`)
}

// ---------- Config & Workspace Commands ----------
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

func handleWorkspace(args []string) {
	if len(args) == 0 || args[0] == "show" {
		fmt.Printf("📂 Current Workspace: %s\n", workspace)
		return
	}
	if args[0] == "set" && len(args) > 1 {
		target := args[1]
		if strings.HasPrefix(target, "~") {
			target = filepath.Join(homeDir, target[1:])
		}
		abs, err := filepath.Abs(target)
		if err != nil {
			fmt.Printf("❌ Invalid path: %v\n", err)
			return
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			fmt.Println("❌ Directory does not exist.")
			return
		}
		workspace = abs
		config.Workspace = abs
		saveConfig()
		fmt.Printf("✅ Workspace updated to: %s\n", workspace)
	}
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
	if len(args) == 0 || args[0] == "show" {
		fmt.Printf("🤖 Active Model: %s\n", config.Model)
		return
	}
	switch args[0] {
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

	pageSize := 12
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

// ---------- Session & Context Management ----------
func estimateTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)/4 + 4
	}
	return total
}

func autoCompactIfNeeded() {
	if len(sessionMessages) > 15 || estimateTokens(sessionMessages) > 3500 {
		compactHistory()
	}
}

func compactHistory() {
	if len(sessionMessages) < 5 {
		fmt.Println("ℹ️ Session history too short to compact.")
		return
	}

	fmt.Println(c(dim) + "🧠 Compacting context history into summary..." + c(reset))
	older := sessionMessages[:len(sessionMessages)-3]
	recent := sessionMessages[len(sessionMessages)-3:]

	var summaryText strings.Builder
	summaryText.WriteString("Previous session summary:\n")
	for _, m := range older {
		summaryText.WriteString(fmt.Sprintf("%s: %s\n", m.Role, truncateString(m.Content, 100)))
	}

	compactedMsg := Message{
		Role:    "system",
		Content: "Project & Chat Context Summary:\n" + summaryText.String(),
	}

	sessionMessages = append([]Message{compactedMsg}, recent...)
	fmt.Println(c(green) + "✅ Context compacted successfully." + c(reset))
}

func autoSaveSession() {
	if len(sessionMessages) == 0 {
		return
	}
	sess := Session{
		ID:       currentSessionID,
		Name:     "auto_latest",
		Messages: sessionMessages,
		Mode:     currentMode,
		Created:  time.Now(),
		Updated:  time.Now(),
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err == nil {
		_ = os.WriteFile(filepath.Join(chatsDir, "session_latest.json"), data, 0600)
	}
}

func handleSessionCommand(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: /session [save|load] <name>")
		return
	}
	name := args[1]
	filename := filepath.Join(chatsDir, name+".json")

	switch args[0] {
	case "save":
		sess := Session{
			ID:       name,
			Name:     name,
			Messages: sessionMessages,
			Mode:     currentMode,
			Created:  time.Now(),
			Updated:  time.Now(),
		}
		data, _ := json.MarshalIndent(sess, "", "  ")
		_ = os.WriteFile(filename, data, 0600)
		fmt.Printf("%s✅ Session saved as '%s'%s\n", c(green), name, c(reset))
	case "load":
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Printf("❌ Failed to load session '%s'\n", name)
			return
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			fmt.Println("❌ Corrupt session file.")
			return
		}
		sessionMessages = sess.Messages
		currentMode = sess.Mode
		currentSessionID = sess.ID
		fmt.Printf("%s✅ Session '%s' loaded (%d messages).%s\n", c(green), name, len(sessionMessages), c(reset))
	}
}

func listSessions() {
	files, err := os.ReadDir(chatsDir)
	if err != nil || len(files) == 0 {
		fmt.Println("ℹ️ No saved sessions found.")
		return
	}
	fmt.Println(c(bold) + "\n📁 Saved Sessions:" + c(reset))
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			name := strings.TrimSuffix(f.Name(), ".json")
			fmt.Printf("  • %s\n", name)
		}
	}
	fmt.Println()
}

func resumeLastSession() {
	handleSessionCommand([]string{"load", "latest"})
}

func exportChatMarkdown() {
	if len(sessionMessages) == 0 {
		fmt.Println("ℹ️ No conversation history to export.")
		return
	}
	filename := filepath.Join(chatsDir, fmt.Sprintf("export_%d.md", time.Now().Unix()))
	var sb strings.Builder
	sb.WriteString("# NootyCLI Conversation Export\n\n")
	sb.WriteString(fmt.Sprintf("**Date:** %s  \n", time.Now().Format(time.RFC1123)))
	sb.WriteString(fmt.Sprintf("**Model:** %s  \n\n---\n\n", config.Model))

	for _, msg := range sessionMessages {
		if msg.Role == "user" {
			sb.WriteString("### 👤 User:\n" + msg.Content + "\n\n")
		} else if msg.Role == "assistant" {
			sb.WriteString("### 🤖 Nooty:\n" + msg.Content + "\n\n---\n\n")
		}
	}

	_ = os.WriteFile(filename, []byte(sb.String()), 0644)
	fmt.Printf("%s✅ Exported session to: %s%s\n", c(green), filename, c(reset))
}

// ---------- Checkpoint & Undo Engine ----------
func createCheckpoint(relPath string) string {
	absPath := safeJoin(workspace, relPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "" // File doesn't exist yet
	}

	timestamp := time.Now().UnixNano()
	sanitized := strings.ReplaceAll(relPath, "/", "_")
	backupName := fmt.Sprintf("%d_%s.bak", timestamp, sanitized)
	backupPath := filepath.Join(checkpointsDir, backupName)

	if err := os.WriteFile(backupPath, data, 0600); err == nil {
		checkpoints = append(checkpoints, Checkpoint{
			FilePath:   absPath,
			BackupPath: backupPath,
			Timestamp:  time.Now(),
		})
		return backupPath
	}
	return ""
}

func handleUndo() {
	if len(checkpoints) == 0 {
		fmt.Println("ℹ️ No checkpoints available for rollback.")
		return
	}

	last := checkpoints[len(checkpoints)-1]
	checkpoints = checkpoints[:len(checkpoints)-1]

	data, err := os.ReadFile(last.BackupPath)
	if err != nil {
		fmt.Printf("❌ Backup file lost: %v\n", err)
		return
	}

	if err := os.WriteFile(last.FilePath, data, 0644); err != nil {
		fmt.Printf("❌ Undo restore failed: %v\n", err)
		return
	}

	_ = os.Remove(last.BackupPath)
	fmt.Printf("%s✅ Restored file from checkpoint: %s%s\n", c(green), formatPath(last.FilePath), c(reset))
}

// ---------- Diff & Patch Engine ----------
func showDiffPreview(oldContent, newContent string) {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	fmt.Println(c(bold) + "\n🔍 Diff Preview:" + c(reset))
	max := len(newLines)
	if len(oldLines) > max {
		max = len(oldLines)
	}

	for i := 0; i < max; i++ {
		var oldL, newL string
		if i < len(oldLines) {
			oldL = oldLines[i]
		}
		if i < len(newLines) {
			newL = newLines[i]
		}

		if oldL != newL {
			if oldL != "" {
				fmt.Printf("%s- %s%s\n", c(red), oldL, c(reset))
			}
			if newL != "" {
				fmt.Printf("%s+ %s%s\n", c(green), newL, c(reset))
			}
		}
	}
	fmt.Println()
}

// ---------- Smart @File Context Injection ----------
func preprocessUserPrompt(input string) string {
	re := regexp.MustCompile(`@([a-zA-Z0-9_\.\/\-]+)`)
	matches := re.FindAllStringSubmatch(input, -1)

	var injected string
	for _, match := range matches {
		target := match[1]
		path := safeJoin(workspace, target)

		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if info.IsDir() {
			files, _ := os.ReadDir(path)
			injected += fmt.Sprintf("\n--- Context Directory @%s ---\n", target)
			for _, f := range files {
				injected += f.Name() + "\n"
			}
			injected += "---\n"
		} else {
			data, err := os.ReadFile(path)
			if err == nil {
				injected += fmt.Sprintf("\n--- Context File @%s ---\n%s\n---\n", target, string(data))
			}
		}
	}
	return input + injected
}

// ---------- Main Chat & Agent Loop Engine ----------
func handleChat(input string) {
	processedInput := preprocessUserPrompt(input)
	messages := buildMessages(processedInput)

	autoCompactIfNeeded()

	if currentMode == "cli" {
		runAgentLoop(messages)
	} else {
		streamResponse(messages)
	}
	autoSaveSession()
}

func buildMessages(userInput string) []Message {
	var msgs []Message
	sysPrompt := `You are NootyCLI v0.3, an autonomous agentic terminal AI assistant.

When in CHAT mode: Provide concise, expert technical answers.

When in CLI mode: You act as an autonomous workspace agent.
To execute tools, reply STRICTLY using this exact syntax:
TOOL: tool_name key1="value1" key2="value2"

Available Workspace Tools:
- list_files (path="relative_path")
- tree (path="relative_path")
- read_file (path="relative_path")
- patch_file (path="relative_path", search="exact_text", replace="new_text")
- append_file (path="relative_path", content="text_to_append")
- write_file (path="relative_path", content="full_content")
- create_file (path="relative_path", content="initial_content")
- delete_file (path="relative_path")
- find_files (pattern="*.go")
- search_code (query="text", path="relative_path")
- file_info (path="relative_path")
- git_status
- git_diff
- lint_or_check
- count_tokens (path="relative_path")
- run_command (command="shell_cmd", timeout="seconds")
- run_and_verify (command="shell_cmd")

IMPORTANT: Use EXACT tool format. You can issue multiple TOOL: lines in a single response for parallel batch execution.`

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
	done := make(chan struct{})
	go showSpinner(done)

	reqPayload := ChatRequest{Model: config.Model, Messages: messages, Stream: true}
	jsonData, _ := json.Marshal(reqPayload)
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
	headers := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}

	start := time.Now()
	resp, err := doWithRetry("POST", endpoint, jsonData, headers)
	close(done)

	if err != nil {
		fmt.Printf("%s❌ Network Request Failed: %v%s\n", c(red), err, c(reset))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("%s❌ API Error (HTTP %d): %s%s\n", c(red), resp.StatusCode, string(body), c(reset))
		return
	}

	reader := bufio.NewReader(resp.Body)
	var fullContent strings.Builder
	charCount := 0

	fmt.Print(c(bold) + c(cyan) + "🤖 Nooty: " + c(reset))
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := strings.TrimPrefix(line, "data: ")
		if dataStr == "[DONE]" {
			break
		}

		var chunk ChatStreamChunk
		if err := json.Unmarshal([]byte(dataStr), &chunk); err == nil {
			if len(chunk.Choices) > 0 {
				content := chunk.Choices[0].Delta.Content
				fmt.Print(content)
				fullContent.WriteString(content)
				charCount += len(content)
			}
		}
	}
	fmt.Println()

	dur := time.Since(start).Seconds()
	tokEst := charCount / 4
	tokSec := float64(tokEst) / math.Max(dur, 0.1)

	fmt.Printf("%s⚡ %d tokens | %.1f tok/s | %.2fs | Model: %s%s\n\n",
		c(dim), tokEst, tokSec, dur, config.Model, c(reset))

	sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: fullContent.String()})
}

// ---------- Agent Loop with Reflection & Parallel Tools ----------
func runAgentLoop(messages []Message) {
	agentRunning = true
	defer func() {
		agentRunning = false
		if r := recover(); r != nil {
			fmt.Printf("%s⚠️ Agent loop crashed gracefully: %v. Session preserved.%s\n", c(red), r, c(reset))
		}
	}()

	maxSteps := 8
	for step := 0; step < maxSteps && agentRunning; step++ {
		done := make(chan struct{})
		go showSpinner(done)

		reqPayload := ChatRequest{Model: config.Model, Messages: messages, Stream: true}
		jsonData, _ := json.Marshal(reqPayload)
		endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
		headers := map[string]string{"Content-Type": "application/json"}
		if config.APIKey != "" {
			headers["Authorization"] = "Bearer " + config.APIKey
		}

		start := time.Now()
		resp, err := doWithRetry("POST", endpoint, jsonData, headers)
		close(done)

		if err != nil {
			fmt.Printf("%s❌ Agent request failed: %v%s\n", c(red), err, c(reset))
			return
		}

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			fmt.Printf("%s❌ Agent API Error (HTTP %d): %s%s\n", c(red), resp.StatusCode, string(body), c(reset))
			return
		}

		reader := bufio.NewReader(resp.Body)
		var fullResponse strings.Builder
		charCount := 0

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			dataStr := strings.TrimPrefix(line, "data: ")
			if dataStr == "[DONE]" {
				break
			}

			var chunk ChatStreamChunk
			if err := json.Unmarshal([]byte(dataStr), &chunk); err == nil {
				if len(chunk.Choices) > 0 {
					content := chunk.Choices[0].Delta.Content
					fmt.Print(content)
					fullResponse.WriteString(content)
					charCount += len(content)
				}
			}
		}
		resp.Body.Close()
		fmt.Println()

		dur := time.Since(start).Seconds()
		tokEst := charCount / 4
		tokSec := float64(tokEst) / math.Max(dur, 0.1)
		fmt.Printf("%s⚡ %d tokens | %.1f tok/s | %.2fs%s\n", c(dim), tokEst, tokSec, dur, c(reset))

		responseText := fullResponse.String()
		messages = append(messages, Message{Role: "assistant", Content: responseText})
		sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: responseText})

		toolCalls := parseToolCalls(responseText)
		if len(toolCalls) == 0 {
			break // No tools to execute, agent finished!
		}

		// Parallel or Concurrent Tool Execution
		toolResults := executeParallelTools(toolCalls)

		var combinedResults strings.Builder
		combinedResults.WriteString("TOOL_RESULTS:\n")
		for _, res := range toolResults {
			combinedResults.WriteString(res + "\n")
		}

		messages = append(messages, Message{Role: "user", Content: combinedResults.String()})
	}
}

func parseToolCalls(response string) []ToolCall {
	var calls []ToolCall
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TOOL:") {
			line = strings.TrimPrefix(line, "TOOL:")
			line = strings.TrimSpace(line)

			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 0 {
				continue
			}
			name := parts[0]
			args := make(map[string]string)

			if len(parts) > 1 {
				re := regexp.MustCompile(`([a-zA-Z0-9_]+)="([^"]*)"`)
				matches := re.FindAllStringSubmatch(parts[1], -1)
				for _, match := range matches {
					args[match[1]] = match[2]
				}
			}
			calls = append(calls, ToolCall{Name: name, Args: args})
		}
	}
	return calls
}

func executeParallelTools(calls []ToolCall) []string {
	results := make([]string, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc ToolCall) {
			defer wg.Done()
			results[idx] = executeSingleTool(tc)
		}(i, call)
	}

	wg.Wait()
	return results
}

func executeSingleTool(call ToolCall) string {
	fmt.Printf("%s🛠 Executing Tool: %s %v%s\n", c(yellow), call.Name, call.Args, c(reset))

	switch call.Name {
	case "list_files":
		rel := call.Args["path"]
		p := safeJoin(workspace, rel)
		files, err := os.ReadDir(p)
		if err != nil {
			return fmt.Sprintf("[%s Error: %v]", call.Name, err)
		}
		var names []string
		for _, f := range files {
			if !ignoredDirs[f.Name()] {
				names = append(names, f.Name())
			}
		}
		return fmt.Sprintf("[%s Output]: %s", call.Name, strings.Join(names, ", "))

	case "tree":
		rel := call.Args["path"]
		p := safeJoin(workspace, rel)
		var sb strings.Builder
		_ = filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() && ignoredDirs[info.Name()] {
				return filepath.SkipDir
			}
			relP, _ := filepath.Rel(p, path)
			if relP != "." {
				sb.WriteString(relP + "\n")
			}
			return nil
		})
		return fmt.Sprintf("[%s Output]:\n%s", call.Name, sb.String())

	case "read_file":
		rel := call.Args["path"]
		p := safeJoin(workspace, rel)
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Sprintf("[%s Error: %v]", call.Name, err)
		}
		return fmt.Sprintf("[%s Output]:\n%s", call.Name, string(data))

	case "patch_file":
		rel := call.Args["path"]
		searchStr := call.Args["search"]
		replaceStr := call.Args["replace"]
		p := safeJoin(workspace, rel)

		createCheckpoint(rel)

		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Sprintf("[%s Error: %v]", call.Name, err)
		}
		content := string(data)
		if !strings.Contains(content, searchStr) {
			return fmt.Sprintf("[%s Error: search target string not found in file]", call.Name)
		}

		updated := strings.Replace(content, searchStr, replaceStr, 1)
		if err := os.WriteFile(p, []byte(updated), 0644); err != nil {
			return fmt.Sprintf("[%s Error: %v]", call.Name, err)
		}

		showDiffPreview(content, updated)
		return fmt.Sprintf("[%s Success]: File %s patched successfully.", call.Name, rel)

	case "append_file":
		rel := call.Args["path"]
		content := call.Args["content"]
		p := safeJoin(workspace, rel)

		createCheckpoint(rel)

		f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Sprintf("[%s Error: %v]", call.Name, err)
		}
		defer f.Close()
		_, _ = f.WriteString("\n" + content)
		return fmt.Sprintf("[%s Success]: Content appended to %s", call.Name, rel)

	case "write_file", "create_file":
		rel := call.Args["path"]
		content := call.Args["content"]
		p := safeJoin(workspace, rel)

		createCheckpoint(rel)

		_ = os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			return fmt.Sprintf("[%s Error: %v]", call.Name, err)
		}
		return fmt.Sprintf("[%s Success]: Wrote file %s", call.Name, rel)

	case "delete_file":
		rel := call.Args["path"]
		p := safeJoin(workspace, rel)

		createCheckpoint(rel)

		if err := os.Remove(p); err != nil {
			return fmt.Sprintf("[%s Error: %v]", call.Name, err)
		}
		return fmt.Sprintf("[%s Success]: Deleted %s", call.Name, rel)

	case "find_files":
		pattern := call.Args["pattern"]
		if pattern == "" {
			pattern = "*"
		}
		var matched []string
		_ = filepath.Walk(workspace, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() && ignoredDirs[info.Name()] {
				return filepath.SkipDir
			}
			if !info.IsDir() {
				matchedName, _ := filepath.Match(pattern, info.Name())
				if matchedName {
					relP, _ := filepath.Rel(workspace, path)
					matched = append(matched, relP)
				}
			}
			return nil
		})
		return fmt.Sprintf("[%s Output]: Found files: %s", call.Name, strings.Join(matched, ", "))

	case "search_code":
		query := call.Args["query"]
		rel := call.Args["path"]
		p := safeJoin(workspace, rel)

		var matches []string
		_ = filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				if info != nil && info.IsDir() && ignoredDirs[info.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			file, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			lineNum := 1
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, query) {
					relP, _ := filepath.Rel(workspace, path)
					matches = append(matches, fmt.Sprintf("%s:%d: %s", relP, lineNum, strings.TrimSpace(line)))
				}
				lineNum++
			}
			return nil
		})
		return fmt.Sprintf("[%s Output]:\n%s", call.Name, strings.Join(matches, "\n"))

	case "file_info":
		rel := call.Args["path"]
		p := safeJoin(workspace, rel)
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Sprintf("[%s Error: %v]", call.Name, err)
		}
		return fmt.Sprintf("[%s Output]: Size: %d bytes, Mode: %s, ModTime: %s", call.Name, info.Size(), info.Mode(), info.ModTime())

	case "git_status":
		out, err := executeCommandStream("git status --short", 30)
		if err != nil {
			return fmt.Sprintf("[%s Error: %v]", call.Name, err)
		}
		return fmt.Sprintf("[%s Output]:\n%s", call.Name, out)

	case "git_diff":
		out, err := executeCommandStream("git diff", 30)
		if err != nil {
			return fmt.Sprintf("[%s Error: %v]", call.Name, err)
		}
		return fmt.Sprintf("[%s Output]:\n%s", call.Name, out)

	case "lint_or_check":
		var cmd string
		if _, err := os.Stat(safeJoin(workspace, "go.mod")); err == nil {
			cmd = "go vet ./..."
		} else if _, err := os.Stat(safeJoin(workspace, "package.json")); err == nil {
			cmd = "npm test"
		} else {
			return fmt.Sprintf("[%s Output]: No standard project manifest found to lint.", call.Name)
		}
		out, err := executeCommandStream(cmd, 60)
		return fmt.Sprintf("[%s Output]: Command: %s\nOutput:\n%s\nErr: %v", call.Name, cmd, out, err)

	case "count_tokens":
		rel := call.Args["path"]
		p := safeJoin(workspace, rel)
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Sprintf("[%s Error: %v]", call.Name, err)
		}
		tokens := len(data) / 4
		return fmt.Sprintf("[%s Output]: File %s estimated tokens: ~%d", call.Name, rel, tokens)

	case "run_command", "run_and_verify":
		cmdStr := call.Args["command"]
		out, err := executeCommandStream(cmdStr, 120)
		if err != nil {
			return fmt.Sprintf("[%s Error/Non-zero Exit]: %v\nOutput:\n%s", call.Name, err, out)
		}
		return fmt.Sprintf("[%s Output]:\n%s", call.Name, out)

	default:
		return fmt.Sprintf("[%s Error]: Unknown tool call name", call.Name)
	}
}

func executeCommandStream(cmdStr string, timeoutSec int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}

	cmd.Dir = workspace

	var buf bytes.Buffer
	mw := io.MultiWriter(&buf, os.Stdout)
	cmd.Stdout = mw
	cmd.Stderr = mw

	err := cmd.Run()
	return buf.String(), err
}

func generateGitCommit() {
	diff, err := executeCommandStream("git diff", 30)
	if err != nil || strings.TrimSpace(diff) == "" {
		fmt.Println("ℹ️ No git changes detected to commit.")
		return
	}

	promptMsg := "Generate a single line Conventional Commit message for this diff:\n" + diff
	messages := []Message{
		{Role: "system", Content: "You generate concise conventional commit messages."},
		{Role: "user", Content: promptMsg},
	}
	streamResponse(messages)
}

func safeJoin(base, rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(base, rel)
}

// ---------- Long-term Memory & Config Loaders ----------
func loadConfig() {
	data, err := os.ReadFile(configFile)
	if err != nil {
		config = Config{
			ProviderEndpoint: "https://api.openai.com/v1",
			Model:            "gpt-4o",
			Safety:           "balanced",
		}
		saveConfig()
		return
	}
	_ = json.Unmarshal(data, &config)
}

func saveConfig() {
	data, _ := json.MarshalIndent(config, "", "  ")
	_ = os.WriteFile(configFile, data, 0600)
}

func loadMemories() {
	data, err := os.ReadFile(memFile)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &memories)
}

func saveMemories() {
	data, _ := json.MarshalIndent(memories, "", "  ")
	_ = os.WriteFile(memFile, data, 0600)
}

func handleMemory(args []string) {
	if len(args) == 0 || args[0] == "list" {
		if len(memories) == 0 {
			fmt.Println("ℹ️ No memories stored.")
			return
		}
		fmt.Println(c(bold) + "\n🧠 Persistent Agent Memories:" + c(reset))
		for _, m := range memories {
			fmt.Printf("  [%d] (%s) %s\n", m.ID, m.Tag, m.Content)
		}
		fmt.Println()
		return
	}

	switch args[0] {
	case "add":
		if len(args) < 3 {
			fmt.Println("Usage: /memory add <tag> <content>")
			return
		}
		tag := args[1]
		content := strings.Join(args[2:], " ")
		m := Memory{
			ID:      len(memories) + 1,
			Tag:     tag,
			Content: content,
			Added:   time.Now().Format(time.RFC3339),
		}
		memories = append(memories, m)
		saveMemories()
		fmt.Printf("%s✅ Memory added under tag [%s]%s\n", c(green), tag, c(reset))

	case "forget":
		if len(args) < 2 {
			fmt.Println("Usage: /memory forget <id>")
			return
		}
		id, _ := strconv.Atoi(args[1])
		var updated []Memory
		for _, m := range memories {
			if m.ID != id {
				updated = append(updated, m)
			}
		}
		memories = updated
		saveMemories()
		fmt.Println(c(green) + "✅ Memory removed." + c(reset))
	}
}

func handleSafety(args []string) {
	if len(args) == 0 {
		fmt.Printf("🛡️ Safety Policy: %s\n", config.Safety)
		return
	}
	config.Safety = args[0]
	saveConfig()
	fmt.Printf("✅ Safety policy set to: %s\n", config.Safety)
}

func showHistory() {
	if len(sessionMessages) == 0 {
		fmt.Println("ℹ️ History is empty.")
		return
	}
	fmt.Println(c(bold) + "\n📜 Session History Log:" + c(reset))
	for i, m := range sessionMessages {
		fmt.Printf(" [%2d] %s%s%s: %s\n", i+1, c(bold), strings.ToUpper(m.Role), c(reset), truncateString(m.Content, 80))
	}
	fmt.Println()
}

func runDoctor() {
	fmt.Println(c(bold) + "\n🩺 Running Nooty Network & Provider Diagnostics..." + c(reset))

	// Test DNS Racing
	fmt.Printf("1. Smart DNS Active Shield: %s (%s)\n", activeDNSName, activeDNSAddr)

	// Test Endpoint Connectivity
	start := time.Now()
	_, err := fetchAvailableModels()
	dur := time.Since(start)

	if err != nil {
		fmt.Printf("%s❌ Provider API Check Failed (%v)%s\n", c(red), err, c(reset))
	} else {
		fmt.Printf("%s✅ Provider API Connection Healthy (Latency: %v)%s\n", c(green), dur, c(reset))
	}

	// Workspace Permission Check
	testFile := filepath.Join(workspace, ".nooty_test")
	if err := os.WriteFile(testFile, []byte("ok"), 0600); err == nil {
		_ = os.Remove(testFile)
		fmt.Printf("%s✅ Workspace Write Permission: OK%s\n", c(green), c(reset))
	} else {
		fmt.Printf("%s❌ Workspace Write Permission Denied%s\n", c(red), c(reset))
	}
	fmt.Println()
}
