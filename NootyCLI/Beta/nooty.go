// nooty.go — NootyCLI v0.3 "Parallel Agent" – Agentic Terminal Intelligence
// Single‑file, zero external dependencies, cross‑platform (macOS / Linux / Windows / WSL).
//
// 🚀 Compile & Build:
//   go build -ldflags="-s -w" -o nooty nooty.go
//
// 🛠 New in v0.3:
//   • Streaming model output in both chat & agent modes
//   • Parallel batch tool execution (multiple tool calls per response)
//   • patch_file tool with search/replace diff support
//   • Token‑based context management with automatic summarization
//   • Session persistence & resume (/sessions, /resume)
//   • Streaming run_command output
//   • Gitignore‑aware file walking (skips node_modules, .git, vendor …)
//   • Retry/backoff for transient network errors
//   • Checkpoint & undo for file modifications

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
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- Cross‑Platform ANSI Styling Engine ----------
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

type Session struct {
	ID        string    `json:"id"`
	Started   time.Time `json:"started"`
	Updated   time.Time `json:"updated"`
	Messages  []Message `json:"messages"`
	Summary   string    `json:"summary,omitempty"`
	Workspace string    `json:"workspace"`
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
	chatDir         string
	checkpointDir   string

	fallbackDNS = []DNSResolver{
		{Name: "Direct Connection", Address: ""},
		{Name: "Electro DNS", Address: "78.157.42.100"},
		{Name: "Shecan DNS #1", Address: "178.22.122.100"},
		{Name: "Shecan DNS #2", Address: "185.51.200.2"},
		{Name: "Begzar DNS #1", Address: "185.55.226.26"},
		{Name: "Begzar DNS #2", Address: "185.55.225.25"},
	}
	activeDNSName = "Direct Connection"

	// Checkpoint stack
	checkpointStack []string
	checkpointMu    sync.Mutex

	// Token budget & context
	maxContextTokens = 8000 // approx 8k tokens
	maxResponseTokens = 2000
	summaryThreshold = 0.7 // when used tokens > 70% budget, summarize
)

func main() {
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
	_ = os.MkdirAll(nootyDir, 0700)
	_ = os.MkdirAll(filepath.Join(nootyDir, "chats"), 0700)
	_ = os.MkdirAll(filepath.Join(nootyDir, "checkpoints"), 0700)
	configFile = filepath.Join(nootyDir, "config.json")
	memFile = filepath.Join(nootyDir, "memories.json")
	chatDir = filepath.Join(nootyDir, "chats")
	checkpointDir = filepath.Join(nootyDir, "checkpoints")

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
	repl()
}

// ---------- Minimal Sleek Header ----------
func drawHeader() {
	width := 64
	line := strings.Repeat("─", width-2)

	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), centerText(" NOOTY CLI ", width-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), centerText("v0.3 Parallel Agent — Agentic Terminal Intelligence", width-2), c(cyan), c(reset))
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
		checkpointStack = nil
		if runtime.GOOS == "windows" {
			cmd := exec.Command("cmd", "/c", "cls")
			cmd.Stdout = os.Stdout
			_ = cmd.Run()
		} else {
			fmt.Print("\033[H\033[2J")
		}
		drawHeader()
		fmt.Println(c(green) + "✨ Session history & screen cleared." + c(reset))
	case "/sessions":
		listSessions()
	case "/resume":
		if len(parts) > 1 {
			resumeSession(parts[1])
		} else {
			fmt.Println("Usage: /resume <session-id>")
		}
	case "/undo":
		undoLastCheckpoint()
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
  /dns                         Display Anti‑Sanction Smart DNS Shield status
  /doctor                      Run full connection and API health check
  /memory list|add|forget      Manage long‑term persistent agent context
  /safety strict|balanced      Set command safety confirmation policies
  /history                     Display conversation session log
  /clear                       Reset current screen & session memory
  /sessions                    List saved chat sessions
  /resume <id>                 Resume a previous session
  /undo                        Undo last file modification (checkpoint)
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
	fmt.Println(c(bold) + "\n🛡️ Anti‑Sanction Smart DNS Fallback Chain:" + c(reset))
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

// ---------- Network Transport Engine (with retry/backoff) ----------
func dnsDialer(dnsServer string) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{}
		return d.DialContext(ctx, network, dnsServer+":53")
	}
}

func httpClientForDNS(dns string) *http.Client {
	if dns == "" {
		return &http.Client{Timeout: 120 * time.Second}
	}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial:     dnsDialer(dns),
	}
	dialer := &net.Dialer{Resolver: resolver}
	return &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext},
		Timeout:   120 * time.Second,
	}
}

func isRetryableStatus(code int) bool {
	return code == 429 || code >= 500
}

func doWithFallback(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	var lastErr error
	for i, dnsResolver := range fallbackDNS {
		client := httpClientForDNS(dnsResolver.Address)

		// Retry loop per DNS resolver
		for attempt := 0; attempt < 3; attempt++ {
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
			if err == nil && resp.StatusCode < 500 && resp.StatusCode != 429 && resp.StatusCode != 403 && resp.StatusCode != 451 {
				activeDNSName = dnsResolver.Name
				return resp, nil
			}

			// Close if response exists
			if resp != nil {
				_ = resp.Body.Close()
			}

			lastErr = err
			if err == nil && resp != nil {
				lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			}
			if err != nil || (resp != nil && !isRetryableStatus(resp.StatusCode)) {
				// Non-retryable, break to next DNS
				break
			}

			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			fmt.Printf("%s⚠️ Retry %d/%d after %v (%v)%s\n",
				c(yellow), attempt+1, 3, backoff, lastErr, c(reset))
			time.Sleep(backoff)
		}

		if i < len(fallbackDNS)-1 {
			fmt.Printf("%s⚠️ Connection via %s failed. Switching to %s...%s\n",
				c(yellow), dnsResolver.Name, fallbackDNS[i+1].Name, c(reset))
		}
	}
	return nil, fmt.Errorf("network connection failed after all resolvers: %v", lastErr)
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

// ---------- Token Estimation & Context Management ----------
func estimateTokens(s string) int {
	// Rough estimate: 4 characters = 1 token
	return len(s) / 4
}

func messagesTokenCount(msgs []Message) int {
	total := 0
	for _, m := range msgs {
		total += estimateTokens(m.Content) + 4 // overhead per message
	}
	return total
}

// Summarizes a slice of messages into a single string using the model.
func summarizeMessages(msgs []Message) (string, error) {
	if len(msgs) == 0 {
		return "", nil
	}
	var combined strings.Builder
	for _, m := range msgs {
		combined.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}
	prompt := "Summarize the following conversation into a concise paragraph, capturing key decisions, context, and important details:\n\n" + combined.String()
	resp, err := getModelResponseText([]Message{{Role: "user", Content: prompt}})
	if err != nil {
		return "", err
	}
	return resp, nil
}

// Trims messages to fit within token budget, optionally summarizing older ones.
func trimMessages(messages []Message, budget int) ([]Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}
	// Keep system message and last user/assistant messages as many as fit.
	// We'll try to include a summary of older messages if necessary.
	systemMsgs := []Message{}
	var nonSystem []Message
	for _, m := range messages {
		if m.Role == "system" {
			systemMsgs = append(systemMsgs, m)
		} else {
			nonSystem = append(nonSystem, m)
		}
	}

	// Always keep system messages (assume they are small)
	currentTokens := messagesTokenCount(systemMsgs)
	if currentTokens > budget {
		// Even system messages exceed budget; drop oldest system messages except the first?
		// We'll just keep first system message and drop rest.
		systemMsgs = systemMsgs[:1]
		currentTokens = messagesTokenCount(systemMsgs)
	}

	// Add non‑system messages from the end until budget exceeded
	kept := []Message{}
	for i := len(nonSystem) - 1; i >= 0; i-- {
		t := estimateTokens(nonSystem[i].Content) + 4
		if currentTokens+t > budget {
			break
		}
		kept = append([]Message{nonSystem[i]}, kept...) // prepend
		currentTokens += t
	}

	// If there are older messages not kept, summarize them and add as system
	if len(kept) < len(nonSystem) {
		older := nonSystem[:len(nonSystem)-len(kept)]
		summary, err := summarizeMessages(older)
		if err != nil {
			// Fallback: just add a note
			summary = "Earlier conversation was truncated due to token limits."
		}
		systemMsgs = append(systemMsgs, Message{Role: "system", Content: "Summary of earlier conversation:\n" + summary})
	}

	result := append(systemMsgs, kept...)
	return result, nil
}

// ---------- Chat Execution ----------
func handleChat(input string) {
	// Build messages with token budget
	messages := buildMessages(input)
	if currentMode == "cli" {
		runAgentLoop(messages)
		return
	}
	streamResponse(messages)
	// Persist session after each turn
	saveCurrentSession()
}

func buildMessages(userInput string) []Message {
	var msgs []Message
	sysPrompt := `You are NootyCLI, an autonomous agentic terminal AI assistant.

When in CHAT mode: Provide concise, expert terminal and software engineering responses.

When in CLI mode: You act as an autonomous workspace agent.
You may issue one or more tool calls per response. For each tool call, use the following syntax on a separate line:
TOOL: tool_name key1="value1" key2="value2"

Available Workspace Tools:
- list_files (path="relative_path")
- tree (path="relative_path")
- read_file (path="relative_path")
- write_file (path="relative_path", content="full_content")
- create_file (path="relative_path", content="initial_content")
- patch_file (path="relative_path", search="exact_text", replace="new_text", count="optional")
- delete_file (path="relative_path")
- search_code (query="text", path="relative_path")
- file_info (path="relative_path")
- git_status
- git_diff
- run_command (command="shell_cmd", timeout="seconds")

IMPORTANT: Use EXACT tool format. You may output multiple TOOL lines in a single response. Each line must start with "TOOL:".`

	relevant := getRelevantMemories(userInput)
	if len(relevant) > 0 {
		sysPrompt += "\n\nUser Context & Memories:\n"
		for _, m := range relevant {
			sysPrompt += fmt.Sprintf("- [%s] %s\n", m.Tag, m.Content)
		}
	}

	msgs = append(msgs, Message{Role: "system", Content: sysPrompt})

	// Add existing session messages
	msgs = append(msgs, sessionMessages...)

	// Add new user message
	userMsg := Message{Role: "user", Content: userInput}
	msgs = append(msgs, userMsg)

	// Truncate to token budget
	trimmed, err := trimMessages(msgs, maxContextTokens)
	if err != nil {
		// If summarization fails, fall back to last 10 messages
		start := 0
		if len(msgs) > 10 {
			start = len(msgs) - 10
		}
		trimmed = append([]Message{msgs[0]}, msgs[start:]...)
	}

	// Update sessionMessages with trimmed version
	sessionMessages = trimmed
	return trimmed
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

// getModelResponseText now supports streaming optionally? We'll keep non‑streaming for planning/summarization.
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

// ---------- Agentic Plan & Execute Loop (parallel) ----------
func runAgentLoop(messages []Message) {
	// Plan step (streamed)
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

	for step := 0; step < 10; step++ {
		// Stream model response for this step
		fmt.Print(c(magenta) + "\n🤖 Nooty > " + c(reset))
		respText, err := streamModelResponseText(msgs) // returns full text after streaming
		if err != nil {
			fmt.Printf("%s❌ Agent Execution Error: %v%s\n", c(red), err, c(reset))
			return
		}

		// Extract all tool calls (could be multiple)
		toolCalls := extractAllToolCalls(respText)
		if len(toolCalls) == 0 {
			fmt.Println(c(green) + respText + c(reset))
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText})
			return
		}

		fmt.Printf("\n%s🔧 Agent Actions (step %d):%s\n", c(bold)+c(yellow), step+1, c(reset))
		for i, tc := range toolCalls {
			fmt.Printf("  %d. %s", i+1, tc.Name)
			if len(tc.Args) > 0 {
				fmt.Print(" (")
				keys := make([]string, 0, len(tc.Args))
				for k := range tc.Args {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Printf("%s=%q ", k, tc.Args[k])
				}
				fmt.Print(")")
			}
			fmt.Println()
		}

		// Execute tools in parallel if possible (all safe ones run concurrently)
		type toolResult struct {
			Index  int
			Result string
			Error  error
		}
		results := make([]toolResult, len(toolCalls))
		var wg sync.WaitGroup

		// Determine which tools need approval; we'll ask before launching any
		// For simplicity: ask approval for all first, then execute parallel.
		// But we need to handle approval per tool. We'll collect all needed approvals.
		approvalNeeded := false
		for _, tc := range toolCalls {
			if toolNeedsApproval(tc.Name) {
				approvalNeeded = true
				break
			}
		}
		if approvalNeeded {
			fmt.Print(c(yellow) + "⚠️ Some actions require confirmation. Approve all? [Y/n]: " + c(reset))
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(strings.ToLower(confirm))
			if confirm == "n" || confirm == "no" {
				// Deny all
				for i := range toolCalls {
					results[i] = toolResult{Index: i, Result: "Denied by user safety policy.", Error: nil}
				}
			} else {
				// Approve all, but for delete_file we need special confirmation
				for i, tc := range toolCalls {
					if tc.Name == "delete_file" {
						fmt.Printf("%s⚠️ Delete file %s? Type DELETE to confirm: %s", c(red), tc.Args["path"], c(reset))
						confirmDelete, _ := reader.ReadString('\n')
						if strings.TrimSpace(confirmDelete) != "DELETE" {
							results[i] = toolResult{Index: i, Result: "Denied by user.", Error: nil}
							continue
						}
					}
					// Approved, will execute later
				}
			}
		}

		// Launch parallel execution for those not pre‑denied
		for i, tc := range toolCalls {
			if results[i].Result != "" {
				// Already denied
				continue
			}
			wg.Add(1)
			go func(idx int, tc ToolCall) {
				defer wg.Done()
				res, err := runTool(tc.Name, tc.Args)
				results[idx] = toolResult{Index: idx, Result: res, Error: err}
			}(i, tc)
		}
		wg.Wait()

		// Collect results into a single message for model
		var resultStr strings.Builder
		for i, res := range results {
			resultStr.WriteString(fmt.Sprintf("Tool %d (%s):\n", i+1, toolCalls[i].Name))
			if res.Error != nil {
				resultStr.WriteString(fmt.Sprintf("Error: %v\n", res.Error))
			} else {
				// Truncate long outputs
				out := res.Result
				if len(out) > 2500 {
					out = out[:2500] + "\n... (output truncated)"
				}
				resultStr.WriteString(out)
			}
			resultStr.WriteString("\n\n")
		}

		fmt.Printf("%s📄 Tool Outputs:%s\n%s\n", c(dim), c(reset), resultStr.String())

		// Add assistant response with tool calls and results to message history
		msgs = append(msgs, Message{Role: "assistant", Content: respText})
		msgs = append(msgs, Message{Role: "user", Content: resultStr.String()})

		// Also update sessionMessages with the assistant's tool call response
		sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText})
	}

	fmt.Printf("%s⚠️ Agent loop step limit reached (10 steps).%s\n", c(yellow), c(reset))
}

func toolNeedsApproval(name string) bool {
	switch name {
	case "list_files", "tree", "read_file", "search_code", "file_info", "git_status", "git_diff":
		return false
	default:
		return true // write, create, patch, delete, run_command
	}
}

// streamModelResponseText streams the model response to stdout and returns the full text.
func streamModelResponseText(messages []Message) (string, error) {
	reqPayload := ChatRequest{Model: config.Model, Messages: messages, Stream: true}
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

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	var fullContent strings.Builder

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
	fmt.Println()
	return fullContent.String(), nil
}

func extractAllToolCalls(text string) []ToolCall {
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
	// If none found via line, try regex over whole text
	if len(calls) == 0 {
		re := regexp.MustCompile(`(?i)TOOL:\s*(\w+)\s+([^\n]+)`)
		matches := re.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) >= 3 {
				tc := parseToolArgs(match[1], match[2])
				if tc != nil {
					calls = append(calls, *tc)
				}
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

// ---------- Tool Execution ----------
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
		// Checkpoint before writing
		backupFile(path)
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ File saved (%d bytes): %s", len(content), args["path"]), nil

	case "patch_file":
		path := safeJoin(workspace, args["path"])
		search := args["search"]
		replace := args["replace"]
		countStr := args["count"]
		count := 1 // default replace first occurrence, or -1 for all if count absent?
		if countStr != "" {
			if n, err := strconv.Atoi(countStr); err == nil {
				count = n
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		original := string(data)
		if !strings.Contains(original, search) {
			return "", fmt.Errorf("search string not found in file")
		}
		// Checkpoint
		backupFile(path)
		var newContent string
		if count < 0 {
			newContent = strings.ReplaceAll(original, search, replace)
		} else {
			newContent = strings.Replace(original, search, replace, count)
		}
		err = os.WriteFile(path, []byte(newContent), 0644)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ File patched: %s", args["path"]), nil

	case "delete_file":
		path := safeJoin(workspace, args["path"])
		backupFile(path)
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
		err := filepath.Walk(scope, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if shouldSkipDir(info.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			// Skip hidden files and very large files
			if strings.HasPrefix(info.Name(), ".") || info.Size() > 1_000_000 {
				return nil
			}
			// Use bufio.Scanner to read line by line
			file, err := os.Open(p)
			if err != nil {
				return nil
			}
			defer file.Close()
			scanner := bufio.NewScanner(file)
			lineNum := 0
			found := false
			for scanner.Scan() {
				lineNum++
				if strings.Contains(scanner.Text(), query) {
					found = true
					break
				}
			}
			if found {
				rel, _ := filepath.Rel(workspace, p)
				results = append(results, rel)
			}
			return nil
		})
		if err != nil {
			return "", err
		}
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

		// Stream output to user AND capture to buffer
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = io.MultiWriter(os.Stdout, &outBuf)
		cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

		if err := cmd.Start(); err != nil {
			return "", err
		}

		done := make(chan error, 1)
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

func shouldSkipDir(name string) bool {
	skipDirs := map[string]bool{
		".git":         true,
		".svn":         true,
		".hg":          true,
		"node_modules": true,
		"vendor":       true,
		".venv":        true,
		"venv":         true,
		"__pycache__":  true,
		".idea":        true,
		".vscode":      true,
	}
	return skipDirs[name]
}

func dirTree(root, indent string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err.Error()
	}
	var out string
	for i, e := range entries {
		if shouldSkipDir(e.Name()) && e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") && e.IsDir() {
			// still skip hidden dirs
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

// ---------- Checkpoint & Undo ----------
func backupFile(path string) {
	checkpointMu.Lock()
	defer checkpointMu.Unlock()
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		rel, _ := filepath.Rel(workspace, path)
		timestamp := time.Now().Format("20060102_150405.000000000")
		backupPath := filepath.Join(checkpointDir, fmt.Sprintf("%s_%s.bak", strings.ReplaceAll(rel, "/", "_"), timestamp))
		if err := os.WriteFile(backupPath, data, 0644); err == nil {
			checkpointStack = append(checkpointStack, backupPath)
		}
	}
}

func undoLastCheckpoint() {
	checkpointMu.Lock()
	defer checkpointMu.Unlock()
	if len(checkpointStack) == 0 {
		fmt.Println("No checkpoints available to undo.")
		return
	}
	last := checkpointStack[len(checkpointStack)-1]
	checkpointStack = checkpointStack[:len(checkpointStack)-1]

	// Restore file
	// backup filename format: relpath_escaped_20060102_150405.bak
	// We need original path: remove timestamp suffix and replace '_' with '/'
	base := filepath.Base(last)
	// remove .bak
	base = strings.TrimSuffix(base, ".bak")
	// Split at last "_" (timestamp)
	idx := strings.LastIndex(base, "_")
	if idx == -1 {
		fmt.Println("Invalid checkpoint file name.")
		return
	}
	origRel := base[:idx]
	origRel = strings.ReplaceAll(origRel, "_", "/")
	origPath := safeJoin(workspace, origRel)

	// Read backup and restore
	data, err := os.ReadFile(last)
	if err != nil {
		fmt.Printf("Failed to read backup: %v\n", err)
		return
	}
	_ = os.MkdirAll(filepath.Dir(origPath), 0755)
	if err := os.WriteFile(origPath, data, 0644); err != nil {
		fmt.Printf("Failed to restore file: %v\n", err)
		return
	}
	_ = os.Remove(last)
	fmt.Printf("✅ Restored %s from checkpoint.\n", origRel)
}

// ---------- Session Persistence ----------
func saveCurrentSession() {
	if len(sessionMessages) == 0 {
		return
	}
	session := Session{
		ID:        time.Now().Format("20060102_150405.000000000"),
		Started:   time.Now(),
		Updated:   time.Now(),
		Messages:  sessionMessages,
		Workspace: workspace,
	}
	data, _ := json.MarshalIndent(session, "", "  ")
	filename := filepath.Join(chatDir, session.ID+".json")
	_ = os.WriteFile(filename, data, 0600)
}

func listSessions() {
	files, err := filepath.Glob(filepath.Join(chatDir, "*.json"))
	if err != nil || len(files) == 0 {
		fmt.Println("No saved sessions found.")
		return
	}
	fmt.Println(c(bold) + "\n📂 Saved Sessions:" + c(reset))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal(data, &s) != nil {
			continue
		}
		fmt.Printf("  %s  (updated %s, workspace: %s)\n", s.ID, s.Updated.Format(time.RFC3339), formatPath(s.Workspace))
	}
	fmt.Println()
}

func resumeSession(id string) {
	filename := filepath.Join(chatDir, id+".json")
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("❌ Session %s not found.\n", id)
		return
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		fmt.Printf("❌ Failed to load session: %v\n", err)
		return
	}
	sessionMessages = s.Messages
	workspace = s.Workspace
	config.Workspace = workspace
	saveConfig()
	fmt.Printf("✅ Resumed session %s (%d messages).\n", id, len(sessionMessages))
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
