// nooty.go — NootyCLI v0.3.0 "Radin Agent Core" – Autonomous Code Agent & Terminal Intelligence
// Single‑file, zero external dependencies, cross-platform (macOS / Linux / Windows / WSL).
//
// 🚀 Compile & Build:
//   go build -ldflags="-s -w" -o nooty nooty.go
//
// 🛠 Usage:
//   nooty                      → Interactive REPL Mode
//   nooty "task description"   → Execute single agent task directly
//   nooty init                 → Initialize .nooty workspace & project rules
//   nooty review               → Review uncommitted git changes
//   nooty test                 → Auto-detect and run project unit tests
//   nooty explain <file>       → Explain architecture & file design

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
	"sort"
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
	Safety           string `json:"safety"` // strict, balanced, auto
	Workspace        string `json:"workspace"`
}

type Memory struct {
	ID      int    `json:"id"`
	Tag     string `json:"tag"`
	Content string `json:"content"`
	Added   string `json:"added"`
}

type ProjectInfo struct {
	Type        string `json:"type"`
	TestCommand string `json:"test_command"`
	FilesCount  int    `json:"files_count"`
	Rules       string `json:"rules"`
	IsGit       bool   `json:"is_git"`
}

type Snapshot struct {
	ID         string `json:"id"`
	Timestamp  string `json:"timestamp"`
	FilePath   string `json:"file_path"`
	BackupPath string `json:"backup_path"`
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
	snapshots       []Snapshot
	sessionMessages []Message
	currentMode     = "cli" // Default to CLI mode in v0.3
	workspace       string
	homeDir         string
	nootyDir        string
	projectNootyDir string
	memFile         string
	configFile      string
	snapshotDir     string
	snapshotLog     string
	projInfo        ProjectInfo

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
		fmt.Fprintf(os.Stderr, "NootyCLI v0.3.0 — Autonomous Code Agent Core\n\nUsage:\n  nooty [options] [task|subcommand]\n\nCommands:\n  nooty \"fix auth timeout\"  Direct Agent Task\n  nooty init               Initialize project rules & .nooty workspace\n  nooty review             Code review uncommitted changes\n  nooty test               Run auto-detected unit tests\n  nooty explain <file>     Explain target file or architecture\n  nooty doctor             System & API health check\n\nOptions:\n")
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

	// Setup local project workspace directory
	projectNootyDir = filepath.Join(workspace, ".nooty")
	snapshotDir = filepath.Join(projectNootyDir, "snapshots")
	snapshotLog = filepath.Join(projectNootyDir, "snapshots.json")
	_ = os.MkdirAll(snapshotDir, 0700)
	loadSnapshots()

	// Detect project attributes
	projInfo = detectProject(workspace)

	// Direct CLI Arguments Dispatcher
	args := flag.Args()
	if len(args) > 0 {
		subcmd := args[0]
		switch subcmd {
		case "init":
			runInit()
			return
		case "doctor":
			runDoctor()
			return
		case "review":
			runReview()
			return
		case "test":
			runAutoTests()
			return
		case "explain":
			target := "."
			if len(args) > 1 {
				target = args[1]
			}
			runExplain(target)
			return
		case "ask":
			if len(args) > 1 {
				handleChat(strings.Join(args[1:], " "))
			} else {
				fmt.Println("Usage: nooty ask <question>")
			}
			return
		default:
			// Treat full argument list as direct Agent Task
			task := strings.Join(args, " ")
			currentMode = "cli"
			drawHeader()
			fmt.Printf("%s🎯 Direct Task Received:%s %s\n\n", c(bold)+c(yellow), c(reset), task)
			handleChat(task)
			return
		}
	}

	drawHeader()
	repl()
}

// ---------- Sleek Header ----------
func drawHeader() {
	width := 66
	line := strings.Repeat("─", width-2)

	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), centerText(" NOOTY CLI ", width-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), centerText("v0.3.0 Radin Agent Core — Code Agent Intelligence", width-2), c(cyan), c(reset))
	fmt.Println(c(cyan) + "├" + line + "┤" + c(reset))

	prettyWorkspace := formatPath(workspace)
	gitStatusStr := "Not a Git Repo"
	if projInfo.IsGit {
		gitStatusStr = "Git Enabled"
	}

	entries := [][]string{
		{"Provider", truncateString(config.ProviderEndpoint, 38)},
		{"Model", config.Model},
		{"Project", fmt.Sprintf("%s (%s)", projInfo.Type, gitStatusStr)},
		{"Workspace", truncateString(prettyWorkspace, 38)},
		{"DNS Shield", activeDNSName},
		{"Mode / Safety", fmt.Sprintf("%s (%s)", strings.ToUpper(currentMode), config.Safety)},
	}

	for _, e := range entries {
		val := fmt.Sprintf("%-38s", e[1])
		fmt.Printf("%s│%s %-12s: %s%s %s│%s\n",
			c(cyan), c(bold)+c(white), e[0], c(green), val, c(cyan), c(reset))
	}
	fmt.Println(c(cyan) + "└" + line + "┘" + c(reset))
	if projInfo.Rules != "" {
		fmt.Printf("%s📌 Loaded Project Rules from .nooty/rules.md%s\n", c(yellow), c(reset))
	}
	fmt.Printf("%s💡 Type %s/help%s for commands, %s/undo%s to revert changes.%s\n\n", c(dim), c(bold)+c(green), c(dim), c(bold)+c(cyan), c(dim), c(reset))
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

// ---------- Project Detection & Rules ----------
func detectProject(dir string) ProjectInfo {
	info := ProjectInfo{Type: "Generic Codebase", TestCommand: "", IsGit: false}

	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		info.IsGit = true
	}

	files, _ := os.ReadDir(dir)
	info.FilesCount = len(files)

	if exists(filepath.Join(dir, "go.mod")) {
		info.Type = "Go Modules"
		info.TestCommand = "go test ./..."
	} else if exists(filepath.Join(dir, "package.json")) {
		info.Type = "Node.js"
		info.TestCommand = "npm test"
	} else if exists(filepath.Join(dir, "pyproject.toml")) || exists(filepath.Join(dir, "requirements.txt")) {
		info.Type = "Python"
		info.TestCommand = "pytest"
	} else if exists(filepath.Join(dir, "Cargo.toml")) {
		info.Type = "Rust Cargo"
		info.TestCommand = "cargo test"
	} else if exists(filepath.Join(dir, "composer.json")) {
		info.Type = "PHP Composer"
		info.TestCommand = "vendor/bin/phpunit"
	} else if exists(filepath.Join(dir, "pom.xml")) || exists(filepath.Join(dir, "build.gradle")) {
		info.Type = "Java Project"
		info.TestCommand = "mvn test"
	}

	rulesPath := filepath.Join(dir, ".nooty", "rules.md")
	if data, err := os.ReadFile(rulesPath); err == nil {
		info.Rules = strings.TrimSpace(string(data))
	}

	return info
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runInit() {
	_ = os.MkdirAll(projectNootyDir, 0700)
	rulesPath := filepath.Join(projectNootyDir, "rules.md")
	if !exists(rulesPath) {
		sampleRules := `# NootyCLI Project Rules
- Maintain code quality and clean architecture.
- Write unit tests for new functions.
- Do not modify database migrations automatically.
- Prefer error returning over panicking.
`
		_ = os.WriteFile(rulesPath, []byte(sampleRules), 0644)
		fmt.Printf("%s✅ Created %s%s\n", c(green), rulesPath, c(reset))
	} else {
		fmt.Printf("%sℹ️ .nooty/rules.md already exists.%s\n", c(yellow), c(reset))
	}

	contextPath := filepath.Join(projectNootyDir, "context.json")
	if !exists(contextPath) {
		_ = os.WriteFile(contextPath, []byte("{\n  \"project_notes\": \"\"\n}\n"), 0644)
	}

	fmt.Printf("%s🚀 Nooty Workspace initialized successfully for: %s%s\n", c(bold)+c(cyan), workspace, c(reset))
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
			fmt.Println(c(green) + "🛠 Switched to Agent Mode (Autonomous Execution)." + c(reset))
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
	case "/undo", "/revert":
		undoLastSnapshot()
	case "/changes":
		listSnapshots()
	case "/init":
		runInit()
	case "/review":
		runReview()
	case "/test":
		runAutoTests()
	case "/context":
		showContextBudget()
	case "/memory":
		handleMemory(parts[1:])
	case "/safety":
		handleSafety(parts[1:])
	case "/history":
		showHistory()
	case "/git":
		handleGitCommand(parts[1:])
	case "/commit":
		runAutoCommit()
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
  /mode [chat|cli]             Toggle Chat or Autonomous Agent Mode
  /undo                        Revert last file modification via snapshot
  /changes                     List recent file snapshot changes
  /init                        Initialize .nooty project rules and setup
  /review                      Perform AI Code Review on git diff
  /test                        Run auto-detected unit test suite
  /context                     Display current context window token usage
  /commit                      Generate intelligent AI git commit message
  /git <status|diff|log>       Execute git helper commands
  /config                      Configuration wizard for API key & endpoint
  /workspace show|set <path>   Manage current working directory
  /model show|set|list         View, switch, or browse AI models
  /dns                         Display Anti-Sanction Smart DNS status
  /doctor                      Run diagnostic check on connection & API
  /memory list|add|forget      Manage long-term persistent context
  /safety strict|balanced|auto Set safety confirmation level
  /clear                       Reset screen & session memory
  /exit                        Terminate session`)
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
		config.APIKey Agent Mode
  /undo                        Revert last file modification via snapshot
  /changes                     List recent file snapshot changes
  /init                        Initialize .nooty project rules and setup
  /review                      Perform AI Code Review on git diff
  /test                        Run auto-detected unit test suite
  /context                     Display current context window token usage
  /commit                      Generate intelligent AI git commit message
  /git <status|diff|log>       Execute git helper commands
  /config                      Configuration wizard for API key & endpoint
  /workspace show|set <path>   Manage current working directory
  /model show|set|list         View, switch, or browse AI models
  /dns                         Display Anti-Sanction Smart DNS status
  /doctor                      Run diagnostic check on connection & API
  /memory list|add|forget      Manage long-term persistent context
  /safety strict|balanced|auto Set safety confirmation level
  /clear                       Reset screen & session memory
  /exit                        Terminate session`)
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
	if modlist":
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
		return &http.Client{Timeout: 40 * time.Second}
	}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial:     dnsDialer(dns),
	}
	dialer := &net.Dialer{Resolver: resolver}
	return &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext},
		Timeout:   40 * time.Second,
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

// ---------- Chat Execution & Context Budget ----------
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
	sysPrompt := `You are NootyCLI v0.3, an autonomous agentic code AI workspace assistant.

When in CHAT mode: Provide concise, expert engineering guidance.

When in CLI mode: You act as an autonomous workspace code agent.
To execute tools, reply STRICTLY using this exact syntax:
TOOL: tool_name key1="value1" key2="value2"

Available Workspace Tools:
- read_file (path="relative_path")
- write_file (path="relative_path", content="full_content")
- apply_patch (path="relative_path", patch="search_replace_block")
  Notice for apply_patch: Format patch argument strictly as:
  <<<<<<< SEARCH
  original lines to find
  =======
  replacement lines
  >>>>>>> REPLACE
- delete_file (path="relative_path")
- list_files (path="relative_path")
- search_files (pattern="filename_or_ext")
- search_code (query="text", path="relative_path")
- find_symbol (name="func_or_var_name")
- file_info (path="relative_path")
- tree (path="relative_path")
- git_status
- git_diff
- git_log (count="5")
- git_branch
- run_command (command="shell_cmd", timeout="60")
- run_test (scope="optional_pkg")
- run_linter
- project_info
- env_info

IMPORTANT: Use EXACT tool format. Issue only ONE tool call per interaction step.`

	if projInfo.Rules != "" {
		sysPrompt += "\n\nProject Specific Rules (.nooty/rules.md):\n" + projInfo.Rules
	}

	relevant := getRelevantMemories(userInput)
	if len(relevant) > 0 {
		sysPrompt += "\n\nUser Persistent Context:\n"
		for _, m := range relevant {
			sysPrompt += fmt.Sprintf("- [%s] %s\n", m.Tag, m.Content)
		}
	}

	msgs = append(msgs, Message{Role: "system", Content: sysPrompt})

	histLimit := 12
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

func showContextBudget() {
	sysLen := 1800
	projLen := len(projInfo.Rules)
	histLen := 0
	for _, m := range sessionMessages {
		histLen += len(m.Content)
	}

	estTokens := (sysLen + projLen + histLen) / 4

	fmt.Println(c(bold) + "\n📊 Context Window Token Usage Estimate:" + c(reset))
	fmt.Printf("  • System Prompt & Tools : ~%d tokens\n", sysLen/4)
	fmt.Printf("  • Project Rules         : ~%d tokens\n", projLen/4)
	fmt.Printf("  • Conversation History  : ~%d tokens (%d msgs)\n", histLen/4, len(sessionMessages))
	fmt.Printf("  ---------------------------------------\n")
	fmt.Printf("  %s• Total Estimated Usage : ~%d tokens%s\n\n", c(bold)+c(cyan), estTokens, c(reset))
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

// ---------- Agentic Plan & Execute & Verify Loop ----------
func runAgentLoop(messages []Message) {
	planPrompt := append(messages, Message{Role: "user", Content: "Provide a concise execution plan with step-by-step tool actions to fulfill this task."})
	fmt.Print(c(yellow) + "🤔 Analyzing workspace & planning action graph... " + c(reset))

	planText, err := getModelResponseText(planPrompt)
	if err != nil {
		fmt.Printf("%s❌ Planning failed: %v%s\n", c(red), err, c(reset))
		return
	}

	fmt.Println("\n" + c(cyan) + c(bold) + "📋 Proposed Action Graph:" + c(reset))
	fmt.Println(c(cyan) + planText + c(reset) + "\n")

	if config.Safety != "auto" {
		fmt.Print(c(bold) + "Approve execution plan? [Y/n]: " + c(reset))
		reader := bufio.NewReader(os.Stdin)
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))
		if confirm == "n" || confirm == "no" {
			fmt.Println("🛑 Execution cancelled by user.")
			return
		}
	}

	msgs := append(messages,
		Message{Role: "assistant", Content: planText},
		Message{Role: "user", Content: "Plan approved. Proceed step by step using TOOL commands."},
	)

	maxSteps := 12
	executedSteps := 0

	for i := 0; i < maxSteps; i++ {
		executedSteps++
		respText, err := getModelResponseText(msgs)
		if err != nil {
			fmt.Printf("%s❌ Agent Execution Error: %v%s\n", c(red), err, c(reset))
			return
		}

		toolCall := extractToolCall(respText)
		if toolCall == nil {
			fmt.Println("\n" + c(green) + respText + c(reset) + "\n")
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText})

			// Agent Verification Step
			runVerificationStep(msgs)
			return
		}

		fmt.Printf("\n%s🔧 Agent Action [%d]: %s%s\n", c(bold)+c(yellow), i+1, toolCall.Name, c(reset))
		for k, v := range toolCall.Args {
			fmt.Printf("   %s%s%s: %s\n", c(dim), k, c(reset), truncateString(v, 60))
		}

		toolResult, approved := executeAgentTool(toolCall)
		if !approved {
			msgs = append(msgs, Message{Role: "assistant", Content: respText}, Message{Role: "user", Content: "Action denied by safety policy."})
			continue
		}

		// Smart Tool Output Summarization
		if len(toolResult) > 2500 {
			toolResult = summarizeToolOutput(toolResult)
		}

		fmt.Printf("%s📄 Tool Output:%s\n%s\n", c(dim), c(reset), toolResult)
		msgs = append(msgs, Message{Role: "assistant", Content: respText}, Message{Role: "user", Content: fmt.Sprintf("Tool '%s' output:\n%s", toolCall.Name, toolResult)})
	}

	fmt.Printf("%s⚠️ Agent loop max step limit reached (%d steps).%s\n", c(yellow), maxSteps, c(reset))
}

func runVerificationStep(history []Message) {
	if projInfo.TestCommand != "" {
		fmt.Printf("%s🧪 Running automatic project verification check (%s)...%s\n", c(bold)+c(cyan), projInfo.TestCommand, c(reset))
		out, err := executeShellCommand(projInfo.TestCommand, 45)
		if err == nil && !strings.Contains(out, "FAIL") {
			fmt.Printf("%s✅ Verification Passed: Project tests running cleanly.%s\n", c(green), c(reset))
		} else {
			fmt.Printf("%s⚠️ Verification Note: Test suite execution reported issues.\n%s%s\n", c(yellow), truncateString(out, 300), c(reset))
		}
	}
}

func summarizeToolOutput(output string) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= 40 {
		return output[:2400] + "\n... (output truncated)"
	}
	head := strings.Join(lines[:25], "\n
		}

		// Smart Tool Output Summarization
		if len(toolResult) > 2500 {
			toolResult = summarizeToolOutput(toolResult)
		}

		fmt.Printf("%s📄 Tool Output:%s\n%s\n", c(dim), c(reset), toolResult)
		msgs = append(msgs, Message{Role: "assistant", Content: respText}, Message{Role: "user", Content: fmt.Sprintf("Tool '%s' output:\n%s", toolCall.Name, toolResult)})
	}

	fmt.Printf("%s⚠️ Agent loop max step limit reached (%d steps).%s\n", c(yellow), maxSteps, c(reset))
}

func runVerificationStep(history []Message) {
	if projInfo.TestCommand != "" {
		fmt.Printf("%s🧪 Running automatic project verification check (%s)...%s\n", c(bold)+c(cyan), projInfo.TestCommand, c(reset))
		out, err := executeShellCommand(projInfo.TestCommand, 45)
		if err == nil && !strings.Contains(out, "FAIL") {
			fmt.Printf("%s✅ Verification Passed: Project tests running cleanly.%s\n", c(green), c(reset))
		} else {
			fmt.Printf("%s⚠️ Verification Note: Test suite execution reported issues.\n%s%s\n", c(yellow), truncateString(out, 300), c(reset))
		}
	}
}

func summarizeToolOutput(output string) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= 40 {
		return output[:2400] + "\n... (output truncated)"
	}
	head := strings.Join(lines[:25], "\n")
	tail := strings.Join(lines[len(lines)-15:], "\n")
	return fmt.Sprintf("%s\n\n... [%d lines truncated by Smart Summarizer] ...\n\n%s", head, len(lines)-40, tail)
}

func getModelResponseText(messages []Message) (string, error) {") {
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

// ---------- Tool Execution & Security Engine ----------
func executeAgentTool(tc *ToolCall) (string, bool) {
	needsApproval := false

	if config.Safety == "strict" {
		switch tc.Name {
		case "write_file", "apply_patch", "delete_file", "run_command":
			needsApproval = true
		}
	} else if config.Safety == "balanced" {
		switch tc.Name {
		case "delete_file", "run_command":
			needsApproval = true
		}
	}

	if tc.Name == "run_command" && isDangerousCommand(tc.Args["command"]) {
		fmt.Printf("%s⚠️ SAFETY ALERT: Dangerous shell command detected!%s\n", c(red), c(reset))
		needsApproval = true
	}

	if needsApproval {
		if tc.Name == "delete_file" {
			fmt.Printf("%s⚠️ PERMISSION WARNING: %s will permanently delete target file!%s\n", c(red), tc.Name, c(reset))
			fmt.Print("Type DELETE to confirm: ")
			reader := bufio.NewReader(os.Stdin)
			confirm, _ := reader.ReadString('\n')
			if strings.TrimSpace(confirm) != "DELETE" {
				return "Operation aborted by safety check.", false
			}
		} else {
			fmt.Print(c(bold) + "Confirm tool execution? [Y/n]: " + c(reset))
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

func isDangerousCommand(cmd string) bool {
	dangerPatterns := []string{
		`rm\s+-rf\s+/`, `mkfs`, `dd\s+if=`, `>\s*/dev/sd`, `chmod\s+-R\s+777`, `:(){ :|:& };:`,
		`rm\s+-rf\s+\*`, `drop\s+database`, `shutdown`, `reboot`,
	}
	for _, pat := range dangerPatterns {
		matched, _ := regexp.MatchString(`(?i)`+pat, cmd)
		if matched {
			return true
		}
	}
	return false
}

func runTool(name string, args map[string]string) (string, error) {
	switch name {
	case "read_file":
		path := safeJoin(workspace, args["path"])
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil

	case "write_file":
		relPath := args["path"]
		path := safeJoin(workspace, relPath)
		content := args["content"]

		_ = createSnapshot(relPath)
		_ = os.MkdirAll(filepath.Dir(path), 0755)

		oldText := ""
		if data, err := os.ReadFile(path); err == nil {
			oldText = string(data)
		}

		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			return "", err
		}

		diff := produceDiff(relPath, oldText, content)
		return fmt.Sprintf("✅ File written (%d bytes): %s\n\nProposed Changes Diff:\n%s", len(content), relPath, diff), nil

	case "apply_patch":
		relPath := args["path"]
		patch := args["patch"]
		path := safeJoin(workspace, relPath)

		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("cannot read file to patch: %v", err)
		}
		oldText := string(data)

		newText, err := applyPatchContent(oldText, patch)
		if err != nil {
			return "", fmt.Errorf("patch application failed: %v", err)
		}

		_ = createSnapshot(relPath)
		_ = os.WriteFile(path, []byte(newText), 0644)

		diff := produceDiff(relPath, oldText, newText)
		return fmt.Sprintf("✅ Patch applied successfully to %s\n\nDiff Preview:\n%s", relPath, diff), nil

	case "delete_file":
		relPath := args["path"]
		path := safeJoin(workspace, relPath)
		_ = createSnapshot(relPath)

		if err := os.Remove(path); err != nil {
			return "", err
		}
		return "✅ File removed (snapshot saved): " + relPath, nil

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

	case "search_files":
		pattern := args["pattern"]
		var matches []string
		_ = filepath.Walk(workspace, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || strings.HasPrefix(info.Name(), ".") {
				return nil
			}
			if strings.Contains(strings.ToLower(info.Name()), strings.ToLower(pattern)) {
				rel, _ := filepath.Rel(workspace, p)
				matches = append(matches, rel)
			}
			return nil
		})
		if len(matches) == 0 {
			return "No matching files found.", nil
		}
		return strings.Join(matches, "\n"), nil

	case "search_code":
		query := args["query"]
		scope := workspace
		if s, ok := args["path"]; ok && s != "" {
			scope = safeJoin(workspace, s)
		}
		var results []string
		_ = filepath.Walk(scope, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || info.Size() > 1_000_000 || strings.Contains(p, ".git") || strings.Contains(p, "node_modules") {
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
			return "No code matches found.", nil
		}
		return strings.Join(results, "\n"), nil

	case "find_symbol":
		sym := args["name"]
		var results []string
		_ = filepath.Walk(workspace, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || info.Size() > 500_000 || strings.Contains(p, ".git") || strings.Contains(p, "node_modules") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err == nil {
				lines := strings.Split(string(data), "\n")
				for idx, line := range lines {
					if strings.Contains(line, sym) && (strings.Contains(line, "func ") || strings.Contains(line, "class ") || strings.Contains(line, "function ") || strings.Contains(line, "def ")) {
						rel, _ := filepath.Rel(workspace, p)
						results = append(results, fmt.Sprintf("%s:%d -> %s", rel, idx+1, strings.TrimSpace(line)))
					}
				}
			}
			return nil
		})
		if len(results) == 0 {
			return "No symbol definitions found.", nil
		}
		return strings.Join(results, "\n"), nil

	case "file_info":
		path := safeJoin(workspace, args["path"])
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Path: %s\nSize: %d bytes\nMode: %s\nModTime: %s", path, info.Size(), info.Mode(), info.ModTime().Format(time.RFC3339)), nil

	case "tree":
		path := workspace
		if p, ok := args["path"]; ok && p != "" && p != "." {
			path = safeJoin(workspace, p)
		}
		return dirTree(path, ""), nil

	case "git_status":
		return runGit("status", "--short")

	case "git_diff":
		return runGit("diff")

	case "git_log":
		count := "5"
		if c, ok := args["count"]; ok {
			count = c
		}
		return runGit("log", "-n", count, "--oneline")

	case "git_branch":
		return runGit("branch", "-a")

	case "run_command":
		cmdStr := args["command"]
		timeout := 60
		if t, ok := args["timeout"]; ok {
			_, _ = fmt.Sscanf(t, "%d", &timeout)
		}
		return executeShellCommand(cmdStr, timeout)

	case "run_test":
		if projInfo.TestCommand == "" {
			return "No automatic test command detected for this workspace.", nil
		}
		return executeShellCommand(projInfo.TestCommand, 90)

	case "run_linter":
		if exists(filepath.Join(workspace, "golangci.yml")) || projInfo.Type == "Go Modules" {
			return executeShellCommand("go vet ./...", 60)
		}
		return "No default linter configured for this project type.", nil

	case "project_info":
		return fmt.Sprintf("Project Type: %s\nTest Command: %s\nFiles Count: %d\nIs Git: %v\nRules Present: %v",
			projInfo.Type, projInfo.TestCommand, projInfo.FilesCount, projInfo.IsGit, projInfo.Rules != ""), nil

	case "env_info":
		return fmt.Sprintf("OS: %s\nArch: %s\nGo Version: %s\nWorkspace: %s", runtime.GOOS, runtime.GOARCH, runtime.Version(), workspace), nil
	}

	return "", fmt.Errorf("unknown tool: %s", name)
}

func applyPatchContent(original, patch string) (string, error) {
	if strings.Contains(patch, "<<<<<<< SEARCH") && strings.Contains(patch, ">>>>>>> REPLACE") {
		re := regexp.MustCompile(`(?s)<<<<<<< SEARCH\r?\n(.*?)\r?\n=======\r?\n(.*?)\r?\n>>>>>>> REPLACE`)
		matches := re.FindAllStringSubmatch(patch, -1)
		if len(matches) == 0 {
			return "", fmt.Errorf("invalid patch block format")
		}
		result := original
		for _, match := range matches {
			searchBlock := match[1]
			replaceBlock := match[2]
			if !strings.Contains(result, searchBlock) {
				return "", fmt.Errorf("search block not found in file:\n%s", searchBlock)
			}
			result = strings.Replace(result, searchBlock, replaceBlock, 1)
		}
		return result, nil
	}
	return "", fmt.Errorf("patch must strictly contain <<<<<<< SEARCH ... ======= ... >>>>>>> REPLACE blocks")
}

func produceDiff(filename, oldText, newText string) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- %s (original)\n+++ %s (modified)\n", filename, filename))

	max := len(oldLines)
	if len(newLines) > max {
		max = len(newLines)
	}
	changes := 0
	for i := 0; i < max; i++ {
		var oldL, newL string
		if i < len(oldLines) {
			oldL = oldLines[i]
		}
		if i < len(newLines) {
			newL = newLines[i]
		}
		if oldL != newL {
			if i < len(oldLines) {
				sb.WriteString(c(red) + "- " + oldL + c(reset) + "\n")
			}
			if i < len(newLines) {
				sb.WriteString(c(green) + "+ " + newL + c(reset) + "\n")
			}
			changes++
		}
	}
	if changes == 0 {
		return "(no line changes)"
	}
	return sb.String()
}

func executeShellCommand(cmdStr string, timeoutSec int) (string, error) {
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
			out = "(executed successfully with no output)"
		}
		return out, nil
	case <-time.After(time.Duration(timeoutSec) * time.Second):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("execution timed out (%d sec)", timeoutSec)
	}
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workspace
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s error: %v", args[0], err)
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		s = "(no output)"
	}
	return s, nil
}

func dirTree(root, indent string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err.Error()
	}
	var out string
	for i, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" || e.Name() == "vendor" {
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
	cleanRel := filepath.Clean(rel)
	if filepath.IsAbs(cleanRel) {
		relPath, err := filepath.Rel(base, cleanRel)
		if err != nil || strings.HasPrefix(relPath, "..") {
			return base
		}
		return cleanRel
	}
	joined := filepath.Clean(filepath.Join(base, cleanRel))
	relPath, err := filepath.Rel(base, joined)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return base
	}
	return joined
}

// ---------- Snapshot & Undo System ----------
func createSnapshot(relPath string) error {
	fullPath := safeJoin(workspace, relPath)
	if !exists(fullPath) {
		return nil
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return err
	}

	ts := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("%s_%s.bak", ts, filepath.Base(relPath))
	backupPath := filepath.Join(snapshotDir, backupName)

	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return err
	}

	snap := Snapshot{
		ID:         fmt.Sprintf("snap_%d", len(snapshots)+1),
		Timestamp:  time.Now().Format(time.RFC3339),
		FilePath:   relPath,
		BackupPath: backupPath,
	}

	snapshots = append(snapshots, snap)
	saveSnapshots()
	return nil
}

func undoLastSnapshot() {
	if len(snapshots) == 0 {
		fmt.Println("⚠️ No file snapshots found to restore.")
		return
	}

	last := snapshots[len(snapshots)-1]
	backupData, err := os.ReadFile(last.BackupPath)
	if err != nil {
		fmt.Printf("%s❌ Error reading snapshot backup: %v%s\n", c(red), err, c(reset))
		return
	}

	targetPath := safeJoin(workspace, last.FilePath)
	if err := os.WriteFile(targetPath, backupData, 0644); err != nil {
		fmt.Printf("%s❌ Restoration failed: %v%s\n", c(red), err, c(reset))
		return
	}

	snapshots = snapshots[:len(snapshots)-1]
	saveSnapshots()
	fmt.Printf("%s✅ Reverted [%s] back to snapshot (%s)%s\n", c(green), last.FilePath, last.Timestamp, c(reset))
}

func listSnapshots() {
	if len(snapshots) == 0 {
		fmt.Println("📜 No active workspace snapshots.")
		return
	}
	fmt.Println(c(bold) + "\n📜 Workspace Modification Snapshots:" + c(reset))
	for _, s := range snapshots {
		fmt.Printf("  • [%s] %s → %s\n", s.ID, s.FilePath, s.Timestamp)
	}
	fmt.Println()
}

func loadSnapshots() {
	data, err := os.ReadFile(snapshotLog)
	if err != nil {
		snapshots = []Snapshot{}
		return
	}
	_ = json.Unmarshal(data, &snapshots)
}

func saveSnapshots() {
	data, _ := json.MarshalIndent(snapshots, "", "  ")
	_ = os.WriteFile(snapshotLog, data, 0600)
}

// ---------- Subcommands: Review, Explain, Commit & Shell Bang ----------
func runReview() {
	diff, err := runGit("diff")
	if err != nil || diff == "(no output)" {
		fmt.Println("ℹ️ No uncommitted changes detected in git workspace.")
		return
	}

	reviewPrompt := []Message{
		{Role: "system", Content: "You are a Senior Code Reviewer AI. Review the provided git diff for security bugs, logic issues, and performance optimizations. Provide structured findings with line suggestions."},
		{Role: "user", Content: "Please review these uncommitted workspace changes:\n\n" + diff},
	}

	fmt.Println(c(bold) + c(cyan) + "\n🔍 Running Autonomous Code Review..." + c(reset))
	streamResponse(reviewPrompt)
}

func runExplain(target string) {
	path := safeJoin(workspace, target)
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("%s❌ Cannot read target: %v%s\n", c(red), err, c(reset))
		return
	}

	explainPrompt := []Message{
		{Role: "system", Content: "You are a Software Architect AI. Explain the code architecture, key data structures, and purpose clearly."},
		{Role: "user", Content: fmt.Sprintf("Explain the code architecture of %s:\n\n%s", target, string(data))},
	}

	fmt.Printf("%s📖 Explaining %s...%s\n", c(bold)+c(cyan), target, c(reset))
	streamResponse(explainPrompt)
}

func runAutoCommit() {
	diff, err := runGit("diff")
	if err != nil || diff == "(no output)" {
		fmt.Println("ℹ️ No staged/unstaged changes to commit.")
		return
	}

	commitPrompt := []Message{
		{Role: "system", Content: "Generate a concise Conventional Commit message (e.g. fix(api): ..., feat(cli): ...) summarizing the diff. Return ONLY the single line commit message."},
		{Role: "user", Content: "Git diff:\n" + diff},
	}

	fmt.Print(c(yellow) + "🤔 Generating commit message... " + c(reset))
	msg, err := getModelResponseText(commitPrompt)
	if err != nil {
		fmt.Printf("%s❌ Error: %v%s\n", c(red), err, c(reset))
		return
	}

	msg = strings.TrimSpace(msg)
	fmt.Printf("\n%sSuggested Commit:%s %s\n", c(bold)+c(green), c(reset), msg)
	fmt.Print("Execute git commit with this message? [Y/n]: ")
	reader := bufio.NewReader(os.Stdin)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))
	if confirm != "n" && confirm != "no" {
		out, err := runGit("commit", "-am", msg)
		if err != nil {
			fmt.Printf("%s❌ Commit failed: %v%s\n", c(red), err, c(reset))
		} else {
			fmt.Printf("%s✅ Committed successfully:\n%s%s\n", c(green), out, c(reset))
		}
	}
}

func runAutoTests() {
	if projInfo.TestCommand == "" {
		fmt.Println("⚠️ No automatic test suite runner recognized for this project type.")
		return
	}
	fmt.Printf("%s🧪 Executing Test Suite (%s)...%s\n", c(bold)+c(cyan), projInfo.TestCommand, c(reset))
	out, err := executeShellCommand(projInfo.TestCommand, 90)
	if err != nil {
		fmt.Printf("%s❌ Test Run Failed:\n%s%s\n", c(red), out, c(reset))
	} else {
		fmt.Printf("%s✅ Test Suite Passed:\n%s%s\n", c(green), out, c(reset))
	}
}

func handleGitCommand(args []string) {
	if len(args) == 0 {
		out, _ := runGit("status", "--short")
		fmt.Println(out)
		return
	}
	out, err := runGit(args...)
	if err != nil {
		fmt.Printf("%s❌ Git Error: %v%s\n", c(red), err, c(reset))
	} else {
		fmt.Println(out)
	}
}

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
		projInfo = detectProject(workspace)
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
	fmt.Printf("• Detected Project  : %s (Git: %v)\n", projInfo.Type, projInfo.IsGit)
	fmt.Printf("• Safety Mode       : %s\n", config.Safety)
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
	mode := strings.ToLower(args[0])
	if mode == "strict" || mode == "balanced" || mode == "auto" {
		config.Safety = mode
		saveConfig()
		fmt.Printf("✅ Safety mode updated to '%s'.\n", config.Safety)
	} else {
		fmt.Println("Usage: /safety strict | balanced | auto")
	}
}

func showHistory() {
	if len(sessionMessages) == 0 {
		fmt.Println("💬 Session log clean.")
		return
	}
	fmt.Println(c(bold) + "\n📜 Session Log:" + c(reset))
	for _, msg := range sessionMessages {
		role := "👤 User"
		if msg.Role == "assistant" {
			role = "🤖 Nooty"
		}
		fmt.Printf("%s%s:%s %s\n", c(bold), role, c(reset), truncateString(msg.Content, 100))
	}
	fmt.Println()
}

// ---------- Persistence Methods ----------
func loadConfig() {
	data, err := os.ReadFile(configFile)
	if err != nil {
		config = Config{
			ProviderEndpoint: "https://api.openai.com/v1",
			APIKey:           os.Getenv("OPENAI_API_KEY"),
			Model:            "gpt-4o-mini",
			Safety:           "balanced",
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
