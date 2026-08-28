// nooty.go — NootyCLI v0.3.0 "Radin Agent Core" – Agentic Terminal Intelligence
// Single‑file, zero external dependencies, cross-platform (macOS / Linux / Windows / WSL).
//
// 🚀 Compile & Build:
//   go build -ldflags="-s -w" -o nooty nooty.go
//
// 🛠 Commands (inside REPL):
//   /config       → Interactive configuration wizard
//   /model list   → Browse & select available models
//   /mode cli     → Switch to Autonomous Agent Mode
//   /dns          → View Anti-Sanction Smart DNS Shield chain
//   /doctor       → Run full network connection & provider diagnostic
//   /init         → Initialize .nooty project workspace with rules & config
//   /undo         → Restore last snapshot (file recovery)
//   /test         → Run project tests (auto‑detect)
//   /review       → AI code review of git diff
//   /session      → Save/load/reset conversation sessions

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
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
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
	Safety           string `json:"safety"` // strict | balanced | auto
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

// ---------- Global State ----------
var (
	config          Config
	memories        []Memory
	projectMemories []Memory
	sessionMessages []Message
	currentMode     = "chat" // "chat" or "cli"
	workspace       string
	homeDir         string
	nootyDir        string
	memFile         string
	projMemFile     string
	configFile      string
	snapshotDir     string
	lastSnapshot    string

	fallbackDNS = []DNSResolver{
		{Name: "Direct Connection", Address: ""},
		{Name: "Electro DNS", Address: "78.157.42.100"},
		{Name: "Shecan DNS #1", Address: "178.22.122.100"},
		{Name: "Shecan DNS #2", Address: "185.51.200.2"},
		{Name: "Begzar DNS #1", Address: "185.55.226.26"},
		{Name: "Begzar DNS #2", Address: "185.55.225.25"},
	}
	activeDNSName = "Direct Connection"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "NootyCLI v0.3.0 — Agent Core\n\nUsage:\n  nooty [options] [command] [arguments]\n\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nCommands (when not in interactive mode):\n  ask <question>\n  agent <task>\n  review\n  test\n  init\n  doctor\n")
	}
	flag.Parse()

	// Command-line mode?
	args := flag.Args()
	if len(args) > 0 {
		initGlobalState()
		handleCommandLine(args)
		return
	}

	initGlobalState()
	drawHeader()
	repl()
}

func initGlobalState() {
	var err error
	homeDir, err = os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "⚠ Error: Cannot locate user home directory.")
		os.Exit(1)
	}

	nootyDir = filepath.Join(homeDir, ".nooty")
	_ = os.MkdirAll(nootyDir, 0700)
	_ = os.MkdirAll(filepath.Join(nootyDir, "chats"), 0700)
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
	snapshotDir = filepath.Join(workspace, ".nooty", "snapshots")
	projMemFile = filepath.Join(workspace, ".nooty", "project_memory.json")
	loadProjectMemories()
}

func handleCommandLine(args []string) {
	switch args[0] {
	case "ask":
		if len(args) < 2 {
			fmt.Println("Usage: nooty ask <question>")
			return
		}
		query := strings.Join(args[1:], " ")
		messages := buildMessages(query)
		streamResponse(messages)
	case "agent":
		if len(args) < 2 {
			fmt.Println("Usage: nooty agent <task>")
			return
		}
		query := strings.Join(args[1:], " ")
		messages := buildMessages(query)
		runAgentLoop(messages)
	case "review":
		runReview()
	case "test":
		runTest()
	case "init":
		initializeProject()
	case "doctor":
		runDoctor()
	default:
		fmt.Printf("Unknown command: %s\n", args[0])
		flag.Usage()
	}
}

// ---------- Minimal Sleek Header ----------
func drawHeader() {
	width := 70
	line := strings.Repeat("─", width-2)

	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), centerText(" NOOTY CLI ", width-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), centerText("v0.3.0 Radin Agent Core — Agentic Terminal Intelligence", width-2), c(cyan), c(reset))
	fmt.Println(c(cyan) + "├" + line + "┤" + c(reset))

	prettyWorkspace := formatPath(workspace)

	entries := [][]string{
		{"Provider", truncateString(config.ProviderEndpoint, 40)},
		{"Model", config.Model},
		{"API Key", maskAPIKey(config.APIKey)},
		{"Workspace", truncateString(prettyWorkspace, 40)},
		{"DNS Shield", activeDNSName},
		{"Mode", strings.ToUpper(currentMode) + " Mode"},
		{"Safety", config.Safety},
	}

	// Project detection
	projInfo := detectProjectInfo(workspace)
	if projInfo != "" {
		entries = append(entries, []string{"Project", projInfo})
	}

	for _, e := range entries {
		val := fmt.Sprintf("%-40s", e[1])
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

// ---------- Project Detection ----------
func detectProjectInfo(path string) string {
	checks := []struct {
		file string
		desc string
	}{
		{"go.mod", "Go"},
		{"package.json", "Node.js"},
		{"pyproject.toml", "Python"},
		{"requirements.txt", "Python"},
		{"Cargo.toml", "Rust"},
		{"composer.json", "PHP"},
		{"pom.xml", "Java/Maven"},
		{"build.gradle", "Java/Gradle"},
		{"CMakeLists.txt", "C/C++"},
		{"Dockerfile", "Docker"},
	}
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(path, c.file)); err == nil {
			return c.desc
		}
	}
	return ""
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
	case "/init":
		initializeProject()
	case "/undo":
		undoLastSnapshot()
	case "/test":
		runTest()
	case "/review":
		runReview()
	case "/session":
		handleSession(parts[1:])
	case "/exit":
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
  /safety strict|balanced|auto Set command safety confirmation policies
  /history                     Display conversation session log
  /clear                       Reset current screen & session memory
  /init                        Initialize .nooty project workspace (rules, snapshots)
  /undo                        Restore most recent file snapshot
  /test                        Run project tests (auto‑detect)
  /review                      AI code review of git diff
  /session new|save|load|list  Manage conversation sessions
  /exit                        Terminate NootyCLI session

  💡 In Agent CLI Mode: Prefix commands with ! for direct shell execution.`)
}

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
			return resp, nil
		}

		if i < len(fallbackDNS)-1 {
			fmt.Printf("%s⚠️ Direct connection/DNS blocked (%s). Bypassing via %s...%s\n",
				c(yellow), dnsResolver.Name, fallbackDNS[i+1].Name, c(reset))
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	}
	return nil, fmt.Errorf("network connection failed: all anti-sanction resolvers exhausted")
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
	streamResponse(messages)
}

func buildMessages(userInput string) []Message {
	var msgs []Message
	sysPrompt := `You are NootyCLI, an autonomous agentic terminal AI assistant.

When in CHAT mode: Provide concise, expert terminal and software engineering responses.

When in CLI mode: You act as an autonomous workspace agent.
To execute tools, reply STRICTLY using this exact syntax:
TOOL: tool_name key1="value1" key2="value2"

Available Workspace Tools:
- list_files (path="relative_path")
- tree (path="relative_path")
- read_file (path="relative_path")
- write_file (path="relative_path", content="full_content")
- create_file (path="relative_path", content="initial_content")
- delete_file (path="relative_path")
- search_code (query="text", path="relative_path")
- file_info (path="relative_path")
- git_status
- git_diff
- run_command (command="shell_cmd", timeout="seconds")
- apply_patch (path="relative_path", patch="unified_diff")

IMPORTANT: Use EXACT tool format. Issue only ONE tool call per interaction step.`

	// Add project rules
	rules := loadProjectRules()
	if rules != "" {
		sysPrompt += "\n\nProject Rules:\n" + rules
	}

	// Add memories (global + project)
	relevant := getRelevantMemories(userInput)
	if len(relevant) > 0 {
		sysPrompt += "\n\nUser Context & Memories:\n"
		for _, m := range relevant {
			sysPrompt += fmt.Sprintf("- [%s] %s\n", m.Tag, m.Content)
		}
	}

	msgs = append(msgs, Message{Role: "system", Content: sysPrompt})

	histLimit := 10
	start := 0
	if len(sessionMessages) > histLimit {
		start = len(sessionMessages) - histLimit
	}
	msgs = append(msgs, sessionMessages[start:]...)

	userMsg := Message{Role: "user", Content: userInput}
	msgs = append(msgs, userMsg)
	sessionMessages = append(sessionMessages, userMsg)

	return msgs
}

func getRelevantMemories(query string) []Memory {
	q := strings.ToLower(query)
	var res []Memory

	// Search global memories
	for _, m := range memories {
		if strings.Contains(strings.ToLower(m.Content), q) || strings.Contains(strings.ToLower(m.Tag), q) {
			res = append(res, m)
		}
	}
	// Search project memories
	for _, m := range projectMemories {
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

	resp, err := doWithFallback("POST", endpoint, jsonData, headers)
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

	fmt.Print(c(reset) + "\n\n")
	sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: fullContent.String()})
}

// ---------- Agentic Plan & Execute Loop ----------
func runAgentLoop(messages []Message) {
	planPrompt := append(messages, Message{Role: "user", Content: "Provide a clear, numbered execution plan to fulfill this request."})
	fmt.Print(c(yellow) + "🤔 Analyzing & planning action sequence... " + c(reset))

	planText, err := getModelResponseText(planPrompt)
	if err != nil {
		fmt.Printf("%s❌ Planning failed: %v%s\n", c(red), err, c(reset))
		return
	}

	fmt.Println("\n" + c(cyan) + c(bold) + "📋 Proposed Execution Plan:" + c(reset))
	fmt.Println(c(cyan) + planText + c(reset) + "\n")

	fmt.Print(c(bold) + "Approve plan execution? [Y/n]: " + c(reset))
	reader := bufio.NewReader(os.Stdin)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))
	if confirm == "n" || confirm == "no" {
		fmt.Println("🛑 Execution cancelled by user.")
		return
	}

	msgs := append(messages,
		Message{Role: "assistant", Content: planText},
		Message{Role: "user", Content: "Plan approved. Proceed step by step using TOOL commands."},
	)

	for i := 0; i < 15; i++ { // increased steps for agentic loop
		respText, err := getModelResponseText(msgs)
		if err != nil {
			fmt.Printf("%s❌ Agent Execution Error: %v%s\n", c(red), err, c(reset))
			return
		}

		toolCall := extractToolCall(respText)
		if toolCall == nil {
			fmt.Println("\n" + c(green) + respText + c(reset) + "\n")
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText})
			return
		}

		fmt.Printf("\n%s🔧 Agent Action [%d]: %s%s\n", c(bold)+c(yellow), i+1, toolCall.Name, c(reset))
		for k, v := range toolCall.Args {
			fmt.Printf("   %s%s%s: %s\n", c(dim), k, c(reset), v)
		}

		toolResult, approved := executeAgentTool(toolCall)
		if !approved {
			msgs = append(msgs, Message{Role: "assistant", Content: respText}, Message{Role: "user", Content: "Action denied by user safety policy."})
			continue
		}

		// Summarize tool output for context
		summary := summarizeOutput(toolResult)
		fmt.Printf("%s📄 Tool Output:%s\n%s\n", c(dim), c(reset), summary)

		msgs = append(msgs, Message{Role: "assistant", Content: respText}, Message{Role: "user", Content: fmt.Sprintf("Tool '%s' output:\n%s", toolCall.Name, summary)})
	}
	fmt.Printf("%s⚠️ Agent loop step limit reached (15 steps).%s\n", c(yellow), c(reset))
}

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

func extractToolCall(text string) *ToolCall {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TOOL:") || strings.HasPrefix(line, "TOOL：") {
			return parseToolLine(line)
		}
	}

	re := regexp.MustCompile(`(?i)TOOL:\s*(\w+)\s+(.*)`)
	matches := re.FindStringSubmatch(text)
	if len(matches) >= 3 {
		return parseToolArgs(matches[1], matches[2])
	}
	return nil
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
	re := regexp.MustCompile(`(\w+)=("(?:[^"\\\]|\\.)*"|'(?:[^'\\\]|\\.)*'|\S+)`)
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

func executeAgentTool(tc *ToolCall) (string, bool) {
	// Permission handling based on safety mode
	needsApproval := false
	toolName := tc.Name

	// Define tool categories
	readTools := map[string]bool{"list_files": true, "tree": true, "read_file": true, "search_code": true, "file_info": true, "git_status": true, "git_diff": true}
	writeTools := map[string]bool{"write_file": true, "create_file": true, "apply_patch": true}
	destructiveTools := map[string]bool{"delete_file": true, "run_command": true}

	switch config.Safety {
	case "auto":
		needsApproval = false
	case "balanced":
		if writeTools[toolName] || destructiveTools[toolName] {
			needsApproval = true
		}
	case "strict":
		if !readTools[toolName] {
			needsApproval = true
		}
	default:
		needsApproval = true
	}

	if needsApproval {
		if destructiveTools[toolName] {
			fmt.Printf("%s⚠️ SAFETY WARNING: %s can be destructive!%s\n", c(red), toolName, c(reset))
			fmt.Print("Type CONFIRM to proceed: ")
			reader := bufio.NewReader(os.Stdin)
			confirm, _ := reader.ReadString('\n')
			if strings.TrimSpace(confirm) != "CONFIRM" {
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

func summarizeOutput(output string) string {
	const maxLen = 5000
	if len(output) <= maxLen {
		return output
	}
	// Simple summarization: first 2000 + last 2000
	head := output[:2000]
	tail := output[len(output)-2000:]
	return fmt.Sprintf("(truncated from %d chars)\n%s\n...\n%s", len(output), head, tail)
}

// ---------- Snapshot & Undo ----------
func createSnapshot(path string) error {
	// Ensure snapshot dir exists
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		return err
	}
	timestamp := time.Now().Format("2006-01-02-150405")
	dest := filepath.Join(snapshotDir, timestamp+"_"+filepath.Base(path))
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return err
	}
	lastSnapshot = dest
	return nil
}

func undoLastSnapshot() {
	if lastSnapshot == "" {
		fmt.Println("No snapshot available to restore.")
		return
	}
	// Find original path from snapshot name
	base := filepath.Base(lastSnapshot)
	// Format: timestamp_originalfilename
	parts := strings.SplitN(base, "_", 2)
	if len(parts) < 2 {
		fmt.Println("Invalid snapshot filename.")
		return
	}
	originalName := parts[1]
	originalPath := filepath.Join(workspace, originalName) // assume root of workspace? better to find actual path
	// For simplicity, we assume snapshot is of file in workspace root; we can improve later
	if _, err := os.Stat(originalPath); os.IsNotExist(err) {
		// Try to find by walking workspace
		_ = filepath.Walk(workspace, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && filepath.Base(p) == originalName {
				originalPath = p
				return filepath.SkipDir
			}
			return nil
		})
	}
	data, err := os.ReadFile(lastSnapshot)
	if err != nil {
		fmt.Printf("Error reading snapshot: %v\n", err)
		return
	}
	if err := os.WriteFile(originalPath, data, 0644); err != nil {
		fmt.Printf("Error restoring snapshot: %v\n", err)
		return
	}
	fmt.Printf("✅ Restored %s from snapshot %s\n", originalPath, filepath.Base(lastSnapshot))
	lastSnapshot = ""
}

// ---------- Tool Implementations ----------
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
		content := args["content"]
		// Snapshot before write if file exists
		if _, err := os.Stat(path); err == nil {
			createSnapshot(path)
		} else {
			// Ensure parent dir exists and snapshot of parent dir? skip snapshot for new file
			_ = os.MkdirAll(filepath.Dir(path), 0755)
		}
		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ File saved (%d bytes): %s", len(content), args["path"]), nil

	case "delete_file":
		path := safeJoin(workspace, args["path"])
		// Snapshot before delete
		createSnapshot(path)
		if err := os.Remove(path); err != nil {
			return "", err
		}
		return "✅ File removed: " + args["path"], nil

	case "apply_patch":
		path := safeJoin(workspace, args["path"])
		patch := args["patch"]
		// Snapshot before applying patch
		createSnapshot(path)
		newContent, err := applyUnifiedPatch(path, patch)
		if err != nil {
			return "", err
		}
		err = os.WriteFile(path, []byte(newContent), 0644)
		if err != nil {
			return "", err
		}
		return "✅ Patch applied successfully", nil

	case "search_code":
		query := args["query"]
		scope := workspace
		if s, ok := args["path"]; ok && s != "" {
			scope = safeJoin(workspace, s)
		}
		var results []string
		_ = filepath.Walk(scope, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || info.Size() > 1_000_000 || strings.HasPrefix(info.Name(), ".") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err == nil && strings.Contains(string(data), query) {
				rel, _ := filepath.Rel(workspace, p)
				results = append(results, rel)
			}
			return nil
		})
		if len(results) == 0 {
			return "No matches found.", nil
		}
		return strings.Join(results, "\n"), nil

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
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf

		if err := cmd.Start(); err != nil {
			return "", err
		}

		done := make(chan error)
		go func() { done <- cmd.Wait() }()

		select {
		case err := <-done:
			out := outBuf.String()
			if errBuf.Len() > 0 {
				out += "\n[stderr]\n" + errBuf.String()
			}
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
	return "", fmt.Errorf("unknown tool: %s", name)
}

// ---------- Unified Patch Parser (basic) ----------
func applyUnifiedPatch(filePath, patch string) (string, error) {
	// Read original file
	orig, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(orig), "\n")

	// Parse patch
	// Expect standard unified diff, but we'll simplify: if patch starts with "---" and "+++" we ignore,
	// then look for hunks starting with @@.
	patchLines := strings.Split(patch, "\n")
	var newLines []string
	lineIdx := 0 // index into original lines
	hunkStart := -1
	hunkOldCount, hunkNewCount := 0, 0
	inHunk := false

	for _, pl := range patchLines {
		pl = strings.TrimRight(pl, "\r")
		if strings.HasPrefix(pl, "@@") {
			// parse hunk header: @@ -l,c +l,c @@
			re := regexp.MustCompile(`@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@`)
			matches := re.FindStringSubmatch(pl)
			if len(matches) < 5 {
				return "", fmt.Errorf("invalid hunk header: %s", pl)
			}
			hunkStart, _ = strconv.Atoi(matches[1])
			hunkOldCount = 1
			if matches[2] != "" {
				hunkOldCount, _ = strconv.Atoi(matches[2])
			}
			hunkNewCount = 1
			if matches[4] != "" {
				hunkNewCount, _ = strconv.Atoi(matches[4])
			}
			// Adjust lineIdx to hunkStart-1 (0-indexed)
			lineIdx = hunkStart - 1
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		if strings.HasPrefix(pl, "---") || strings.HasPrefix(pl, "+++") {
			continue
		}
		if strings.HasPrefix(pl, "-") {
			// skip line (deletion)
			lineIdx++ // move past original line
		} else if strings.HasPrefix(pl, "+") {
			// add line
			newLines = append(newLines, pl[1:])
		} else if pl == " " || pl == "" {
			// context line
			if lineIdx < len(lines) {
				newLines = append(newLines, lines[lineIdx])
			}
			lineIdx++
		} else {
			return "", fmt.Errorf("unexpected patch line: %s", pl)
		}
	}
	// Append remaining original lines after last hunk
	if lineIdx < len(lines) {
		newLines = append(newLines, lines[lineIdx:]...)
	}

	return strings.Join(newLines, "\n"), nil
}

func dirTree(root, indent string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err.Error()
	}
	var out string
	for i, e := range entries {
		prefix := indent + "├── "
		childIndent := indent + "│   "
		if i == len(entries)-1 {
			prefix = indent + "└── "
			childIndent = indent + "    "
		}
		out += prefix + e.Name() + "\n"
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
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

// ---------- Project Init & Rules ----------
func initializeProject() {
	fmt.Println(c(bold) + "🔧 Initializing .nooty project workspace..." + c(reset))
	nooDir := filepath.Join(workspace, ".nooty")
	if err := os.MkdirAll(nooDir, 0755); err != nil {
		fmt.Printf("Error creating .nooty: %v\n", err)
		return
	}
	// Create rules.md if not exist
	rulesFile := filepath.Join(nooDir, "rules.md")
	if _, err := os.Stat(rulesFile); os.IsNotExist(err) {
		defaultRules := `# Project Rules
# Add your custom instructions here, one per line.
# Examples:
# - Never modify migrations automatically.
# - Use Python 3.11.
# - Follow PEP 8.
# - Run tests after every change.
`
		if err := os.WriteFile(rulesFile, []byte(defaultRules), 0644); err != nil {
			fmt.Printf("Error creating rules.md: %v\n", err)
		} else {
			fmt.Println("Created .nooty/rules.md")
		}
	}
	// Create snapshots dir
	snapDir := filepath.Join(nooDir, "snapshots")
	if err := os.MkdirAll(snapDir, 0755); err != nil {
		fmt.Printf("Error creating snapshots dir: %v\n", err)
	} else {
		fmt.Println("Created .nooty/snapshots/")
	}
	// Create project memory file if not exists
	pmFile := filepath.Join(nooDir, "project_memory.json")
	if _, err := os.Stat(pmFile); os.IsNotExist(err) {
		if err := os.WriteFile(pmFile, []byte("[]"), 0644); err != nil {
			fmt.Printf("Error creating project_memory.json: %v\n", err)
		} else {
			fmt.Println("Created .nooty/project_memory.json")
		}
	}
	fmt.Println(c(green) + "✅ Project workspace initialized." + c(reset))
}

func loadProjectRules() string {
	rulesFile := filepath.Join(workspace, ".nooty", "rules.md")
	data, err := os.ReadFile(rulesFile)
	if err != nil {
		return ""
	}
	return string(data)
}

// ---------- Memory Management (Global + Project) ----------
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

func loadProjectMemories() {
	data, err := os.ReadFile(projMemFile)
	if err != nil {
		projectMemories = []Memory{}
		return
	}
	_ = json.Unmarshal(data, &projectMemories)
}

func saveProjectMemories() {
	if projMemFile == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(projMemFile), 0755)
	data, _ := json.MarshalIndent(projectMemories, "", "  ")
	_ = os.WriteFile(projMemFile, data, 0644)
}

func handleMemory(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: /memory list | add <text> | forget <id> | project add <text> | project list")
		return
	}
	// Handle project memory subcommand
	if args[0] == "project" {
		if len(args) < 2 {
			fmt.Println("Usage: /memory project list | add <text> | forget <id>")
			return
		}
		switch args[1] {
		case "list":
			if len(projectMemories) == 0 {
				fmt.Println("🧠 No project memories found.")
				return
			}
			fmt.Println(c(bold) + "\n📁 Project Memories:" + c(reset))
			for _, m := range projectMemories {
				fmt.Printf("  [%d] (%s) %s\n", m.ID, m.Tag, m.Content)
			}
			fmt.Println()
		case "add":
			if len(args) < 3 {
				fmt.Println("Usage: /memory project add <text>")
				return
			}
			text := strings.Join(args[2:], " ")
			m := Memory{
				ID:      len(projectMemories) + 1,
				Tag:     "project",
				Content: text,
				Added:   time.Now().Format(time.RFC3339),
			}
			projectMemories = append(projectMemories, m)
			saveProjectMemories()
			fmt.Printf("✅ Saved project memory ID [%d]\n", m.ID)
		case "forget":
			if len(args) < 3 {
				fmt.Println("Usage: /memory project forget <id>")
				return
			}
			id, _ := strconv.Atoi(args[2])
			var newMem []Memory
			for _, m := range projectMemories {
				if m.ID != id {
					newMem = append(newMem, m)
				}
			}
			projectMemories = newMem
			saveProjectMemories()
			fmt.Printf("✅ Project memory [%d] removed.\n", id)
		default:
			fmt.Println("Usage: /memory project list | add <text> | forget <id>")
		}
		return
	}

	// Global memory commands
	switch args[0] {
	case "list":
		if len(memories) == 0 {
			fmt.Println("🧠 No global memories found.")
			return
		}
		fmt.Println(c(bold) + "\n🧠 Global Memories:" + c(reset))
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
	default:
		fmt.Println("Usage: /memory list | add <text> | forget <id> | project ...")
	}
}

func handleSafety(args []string) {
	if len(args) == 0 {
		fmt.Printf("🛡️ Safety Mode: %s\n", config.Safety)
		return
	}
	switch args[0] {
	case "strict", "balanced", "auto":
		config.Safety = args[0]
		saveConfig()
		fmt.Printf("✅ Safety updated to %s.\n", config.Safety)
	default:
		fmt.Println("Usage: /safety strict|balanced|auto")
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

// ---------- Session Management ----------
func handleSession(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: /session new | save <name> | load <name> | list")
		return
	}
	switch args[0] {
	case "new":
		sessionMessages = nil
		fmt.Println("🆕 New session started.")
	case "save":
		if len(args) < 2 {
			fmt.Println("Usage: /session save <name>")
			return
		}
		name := args[1]
		file := filepath.Join(nootyDir, "chats", name+".json")
		data, _ := json.MarshalIndent(sessionMessages, "", "  ")
		if err := os.WriteFile(file, data, 0644); err != nil {
			fmt.Printf("Error saving session: %v\n", err)
			return
		}
		fmt.Printf("✅ Session saved to %s\n", file)
	case "load":
		if len(args) < 2 {
			fmt.Println("Usage: /session load <name>")
			return
		}
		name := args[1]
		file := filepath.Join(nootyDir, "chats", name+".json")
		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("Error loading session: %v\n", err)
			return
		}
		_ = json.Unmarshal(data, &sessionMessages)
		fmt.Printf("✅ Session '%s' loaded (%d messages).\n", name, len(sessionMessages))
	case "list":
		files, err := filepath.Glob(filepath.Join(nootyDir, "chats", "*.json"))
		if err != nil || len(files) == 0 {
			fmt.Println("No saved sessions.")
			return
		}
		fmt.Println("📁 Saved Sessions:")
		for _, f := range files {
			fmt.Printf("  - %s\n", filepath.Base(f))
		}
	default:
		fmt.Println("Usage: /session new | save <name> | load <name> | list")
	}
}

// ---------- Test Runner ----------
func runTest() {
	fmt.Println(c(bold) + "🧪 Running project tests..." + c(reset))
	cmdStr := detectTestCommand()
	if cmdStr == "" {
		fmt.Println("⚠️ Could not detect test command for this project.")
		return
	}
	fmt.Printf("Executing: %s\n", cmdStr)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", cmdStr)
	} else {
		cmd = exec.Command("sh", "-c", cmdStr)
	}
	cmd.Dir = workspace
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("%s❌ Tests failed: %v%s\n", c(red), err, c(reset))
	} else {
		fmt.Println(c(green) + "✅ Tests passed." + c(reset))
	}
}

func detectTestCommand() string {
	projInfo := detectProjectInfo(workspace)
	switch projInfo {
	case "Go":
		return "go test ./..."
	case "Node.js":
		return "npm test"
	case "Python":
		// Check if pytest is used
		if _, err := os.Stat(filepath.Join(workspace, "pytest.ini")); err == nil {
			return "pytest"
		}
		if _, err := os.Stat(filepath.Join(workspace, "pyproject.toml")); err == nil {
			// Could be poetry or pytest; assume pytest
			return "pytest"
		}
		return "python -m unittest discover"
	case "Rust":
		return "cargo test"
	case "PHP":
		return "composer test"
	default:
		return ""
	}
}

// ---------- Code Review ----------
func runReview() {
	fmt.Println(c(bold) + "🔍 AI Code Review of git diff..." + c(reset))
	// Get git diff
	cmd := exec.Command("git", "diff")
	cmd.Dir = workspace
	diffOut, err := cmd.Output()
	if err != nil {
		fmt.Printf("Error getting git diff: %v\n", err)
		return
	}
	diff := string(diffOut)
	if diff == "" {
		fmt.Println("No uncommitted changes to review.")
		return
	}

	// Build prompt for review
	prompt := "Review the following git diff. Provide a structured code review with critical issues, warnings, and suggestions. Format your response as:\n\n🔴 Critical\n...\n\n🟡 Warning\n...\n\n🟢 Good\n...\n\nDiff:\n" + diff
	messages := []Message{
		{Role: "system", Content: "You are a senior code reviewer. Be concise and focus on important issues."},
		{Role: "user", Content: prompt},
	}
	review, err := getModelResponseText(messages)
	if err != nil {
		fmt.Printf("Review failed: %v\n", err)
		return
	}
	fmt.Println(c(green) + "\n📝 Code Review Report:" + c(reset))
	fmt.Println(review)
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
	if config.Safety == "" {
		config.Safety = "strict"
	}
}

func saveConfig() {
	data, _ := json.MarshalIndent(config, "", "  ")
	_ = os.WriteFile(configFile, data, 0600)
}

// ---------- Utilities ----------
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
		snapshotDir = filepath.Join(workspace, ".nooty", "snapshots")
		projMemFile = filepath.Join(workspace, ".nooty", "project_memory.json")
		loadProjectMemories()
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
	fmt.Printf("• Project Type      : %s\n", detectProjectInfo(workspace))
	fmt.Print("• Provider Status   : ")

	models, err := fetchAvailableModels()
	if err != nil {
		fmt.Printf("%sFAILED (%v)%s\n\n", c(red), err, c(reset))
	} else {
		fmt.Printf("%sOK (%d models accessible via %s)%s\n\n", c(green), len(models), activeDNSName, c(reset))
	}
}
