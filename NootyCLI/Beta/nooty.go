// nooty.go — NootyCLI v0.3.0 "Radin Pro" – Agentic Terminal Intelligence
// Single‑file, zero external dependencies, cross‑platform (macOS / Linux / Windows / WSL).
//
// 🚀 Compile & Build:
//   go build -ldflags="-s -w" -o nooty nooty.go
//
// 🛠 Commands:
//   /config       → Interactive configuration wizard
//   /model list   → Browse & select available models
//   /mode cli     → Switch to Autonomous Agent Mode (uses native function calling)
//   /dns          → View Anti‑Sanction Smart DNS Shield chain
//   /doctor       → Run full network connection & provider diagnostic
//   /version      → Show current version

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

// ---------- Version ----------
const VERSION = "0.3.0"

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
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
	Tools    []Tool    `json:"tools,omitempty"`
}

type Tool struct {
	Type     string   `json:"type"` // "function"
	Function Function `json:"function"`
}

type Function struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta,omitempty"`
	} `json:"choices"`
}

type DNSResolver struct {
	Name    string
	Address string
}

// ---------- Global State ----------
var (
	config          Config
	memories        []Memory
	sessionMessages []Message
	currentMode     = "chat"
	workspace       string
	homeDir         string
	nootyDir        string
	memFile         string
	configFile      string

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
		fmt.Fprintf(os.Stderr, "NootyCLI v%s — Agentic Terminal Intelligence\n\nUsage:\n  nooty [options]\n\nOptions:\n", VERSION)
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

	drawHeader()
	if config.APIKey == "" {
		fmt.Printf("%s⚠️  API key is not configured. Run %s/config%s to set it up.%s\n\n", yellow, c(bold)+c(green), c(yellow), c(reset))
	}
	repl()
}

// ---------- Minimal Sleek Header ----------
func drawHeader() {
	width := 64
	line := strings.Repeat("─", width-2)

	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), centerText(" NOOTY CLI ", width-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), centerText("v"+VERSION+" Radin Pro — Agentic Terminal Intelligence", width-2), c(cyan), c(reset))
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
	case "/version":
		fmt.Printf("NootyCLI version %s\n", VERSION)
	case "/mode":
		if len(parts) > 1 && parts[1] == "cli" {
			currentMode = "cli"
			fmt.Println(c(green) + "🛠 Switched to Agent Mode (Native Function Calling Enabled)." + c(reset))
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
  /version                     Display version number
  /mode [chat|cli]             Toggle Chat or Agentic CLI Execution Mode
  /config                      Interactive wizard to setup API key, endpoint & model
  /workspace show|set <path>   Manage current working directory
  /model show|set <name>|list  View, switch, or browse models interactively
  /dns                         Display Anti-Sanction Smart DNS Shield status
  /doctor                      Run full connection and API health check
  /memory list|add|forget      Manage long-term persistent agent context
  /safety strict|balanced      Set command safety confirmation policies
  /history                     Display conversation session log
  /clear                       Reset current screen & session memory
  /exit                        Terminate NootyCLI session

  💡 In Agent CLI Mode: Prefix commands with ! for direct shell execution.
  🛡️ Anti-sanction DNS only protects Nooty's own API calls, not external shell commands.`)
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
	fmt.Println("⚠️  This only protects Nooty’s internal HTTP requests. Shell commands (via ! or run_command) still use your system DNS.")
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

// ---------- Tool Definitions for Function Calling ----------
func getToolDefinitions() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: Function{
				Name:        "list_files",
				Description: "List files and directories in a given path relative to workspace",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Directory path relative to workspace, defaults to '.'",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "tree",
				Description: "Recursively list directory tree",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Starting directory path relative to workspace",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "read_file",
				Description: "Read file contents from a path relative to workspace",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "File path relative to workspace",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "write_file",
				Description: "Write content to a file (overwrites if exists). Use with caution.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "File path relative to workspace",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Full content to write",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "delete_file",
				Description: "Delete a file. Dangerous! User confirmation required.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "File path relative to workspace",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "search_code",
				Description: "Search for text content in workspace files",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Search query string",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Scope directory (default workspace root)",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "file_info",
				Description: "Get metadata of a file",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "File path relative to workspace",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "git_status",
				Description: "Show git working directory status",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "git_diff",
				Description: "Show git diff of unstaged changes",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "run_command",
				Description: "Execute a shell command (dangerous, user confirmation required)",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "The shell command to execute",
						},
						"timeout": map[string]interface{}{
							"type":        "integer",
							"description": "Timeout in seconds (default 60)",
						},
					},
					"required": []string{"command"},
				},
			},
		},
	}
}

// ---------- Chat Execution ----------
func handleChat(input string) {
	if config.APIKey == "" {
		fmt.Printf("%s❌ API key missing. Please run /config first.%s\n", c(red), c(reset))
		return
	}
	messages := buildMessages(input, false)
	if currentMode == "cli" {
		runAgentLoop(messages)
		return
	}
	streamResponse(messages, false)
}

func buildMessages(userInput string, includeTools bool) []Message {
	var msgs []Message
	sysPrompt := `You are NootyCLI, an autonomous agentic terminal AI assistant.

When in CHAT mode: Provide concise, expert terminal and software engineering responses.

When in CLI mode: You act as an autonomous workspace agent. You have access to tools. Use them to fulfill the user request.`
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

func streamResponse(messages []Message, withTools bool) {
	reqPayload := ChatCompletionRequest{
		Model:    config.Model,
		Messages: messages,
		Stream:   true,
	}
	if withTools {
		reqPayload.Tools = getToolDefinitions()
	}
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

		var chunk ChatCompletionResponse
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

// ---------- Agentic Plan & Execute Loop (Native Function Calling) ----------
func runAgentLoop(messages []Message) {
	// Provide a small planning prompt
	planMsg := append(messages, Message{Role: "user", Content: "Before using tools, outline a brief numbered plan."})
	planText, err := getCompletionText(planMsg, true) // enable tools for planning? maybe not, but we can.
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

	// Now start the tool-use loop
	msgs := append(messages,
		Message{Role: "assistant", Content: planText},
		Message{Role: "user", Content: "Plan approved. Proceed step by step using the available tools."},
	)

	for i := 0; i < 15; i++ {
		response, err := getChatCompletionWithTools(msgs)
		if err != nil {
			fmt.Printf("%s❌ Agent error: %v%s\n", c(red), err, c(reset))
			return
		}

		if response.Choices[0].Message.Content != "" {
			text := response.Choices[0].Message.Content
			fmt.Println(c(green) + text + c(reset) + "\n")
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: text})
		}

		if len(response.Choices[0].Message.ToolCalls) == 0 {
			return // no more tools, finished
		}

		// Process tool calls
		assistantMsg := Message{Role: "assistant", ToolCalls: response.Choices[0].Message.ToolCalls}
		msgs = append(msgs, assistantMsg)

		for _, tc := range response.Choices[0].Message.ToolCalls {
			name := tc.Function.Name
			var args map[string]string
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = map[string]string{} // fallback
				}
			} else {
				args = map[string]string{}
			}

			fmt.Printf("\n%s🔧 Agent Action [%d]: %s%s\n", c(bold)+c(yellow), i+1, name, c(reset))
			for k, v := range args {
				fmt.Printf("   %s%s%s: %s\n", c(dim), k, c(reset), v)
			}

			result, approved := executeAgentTool(name, args)
			if !approved {
				result = "Operation denied by user safety policy."
			}

			fmt.Printf("%s📄 Tool Output:%s\n%s\n", c(dim), c(reset), result)

			toolMsg := Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			}
			msgs = append(msgs, toolMsg)
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: fmt.Sprintf("[Tool %s]\n%s", name, result)})
		}
	}
	fmt.Printf("%s⚠️ Agent loop step limit reached (15 steps).%s\n", c(yellow), c(reset))
}

func getChatCompletionWithTools(messages []Message) (*ChatCompletionResponse, error) {
	reqPayload := ChatCompletionRequest{
		Model:    config.Model,
		Messages: messages,
		Tools:    getToolDefinitions(),
	}
	jsonData, _ := json.Marshal(reqPayload)
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
	headers := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}

	resp, err := doWithFallback("POST", endpoint, jsonData, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result ChatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result, nil
}

func getCompletionText(messages []Message, withTools bool) (string, error) {
	reqPayload := ChatCompletionRequest{
		Model:    config.Model,
		Messages: messages,
	}
	if withTools {
		reqPayload.Tools = getToolDefinitions()
	}
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

	var result ChatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty choices")
	}
	return result.Choices[0].Message.Content, nil
}

func executeAgentTool(name string, args map[string]string) (string, bool) {
	needsApproval := false
	switch name {
	case "write_file", "delete_file", "run_command":
		needsApproval = true
	}
	if needsApproval {
		if name == "delete_file" {
			fmt.Printf("%s⚠️ SAFETY WARNING: %s will permanently delete target file!%s\n", c(red), name, c(reset))
			fmt.Print("Type DELETE to confirm action: ")
		} else {
			fmt.Print("Confirm execution? [Y/n]: ")
		}
		reader := bufio.NewReader(os.Stdin)
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))
		if name == "delete_file" {
			if confirm != "DELETE" {
				return "Operation aborted by safety check.", false
			}
		} else if confirm == "n" || confirm == "no" {
			return "Operation cancelled by user.", false
		}
	}

	result, err := runTool(name, args)
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

	case "write_file":
		path := safeJoin(workspace, args["path"])
		content := args["content"]
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ File saved (%d bytes): %s", len(content), args["path"]), nil

	case "delete_file":
		path := safeJoin(workspace, args["path"])
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
		if t, ok := args["timeout"]; ok && t != "" {
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
	fmt.Println("⚠️  This command uses your system DNS, not Nooty’s bypass.")
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
	fmt.Printf("• Version           : %s\n", VERSION)
	fmt.Printf("• Provider Endpoint : %s\n", config.ProviderEndpoint)
	fmt.Printf("• Active Model      : %s\n", config.Model)
	fmt.Printf("• API Key           : %s\n", maskAPIKey(config.APIKey))
	fmt.Printf("• Active Workspace  : %s\n", formatPath(workspace))
	fmt.Print("• Provider Status   : ")

	models, err := fetchAvailableModels()
	if err != nil {
		fmt.Printf("%sFAILED (%v)%s\n\n", c(red), err, c(reset))
	} else {
		fmt.Printf("%sOK (%d models accessible via %s)%s\n\n", c(green), len(models), activeDNSName, c(reset))
	}
	fmt.Println("• DNS Shield only protects LLM API calls. External commands use your system resolver.")
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
		} else if msg.Role == "tool" {
			role = "🛠️ Tool"
		}
		fmt.Printf("%s%s:%s %s\n", c(bold), role, c(reset), msg.Content)
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
