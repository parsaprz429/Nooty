// nooty.go — NootyCLI v0.3 "Radin Pro Max" – Agentic Terminal Intelligence
// Single‑file, zero external dependencies, cross-platform (macOS / Linux / Windows / WSL).
//
// 🚀 Compile & Build:
//   go build -ldflags="-s -w" -o nooty nooty.go
//
// 🛠 New in v0.3:
//   - Parallel tool execution & real-time streaming
//   - patch_file / replace_in_file (search & replace)
//   - Token-aware context compaction & summarization
//   - Session persistence & resume (/sessions, /resume)
//   - DNS racing, retry/backoff, connection pooling
//   - Live subprocess streaming, .gitignore-aware search
//   - Undo/checkpoint, /compact, /export, signal handling
//   - Reflection & self-correction in agent loop
//   - Built-in developer tools: find_files, lint_or_check, git_quick_commit

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

type Session struct {
	Name     string    `json:"name"`
	Messages []Message `json:"messages"`
	Mode     string    `json:"mode"`
	Created  time.Time `json:"created"`
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
	checkpointDir   string
	currentSession  *Session
	sessionMu       sync.Mutex

	fallbackDNS = []DNSResolver{
		{Name: "Direct Connection", Address: ""},
		{Name: "Electro DNS", Address: "78.157.42.100"},
		{Name: "Shecan DNS #1", Address: "178.22.122.100"},
		{Name: "Shecan DNS #2", Address: "185.51.200.2"},
		{Name: "Begzar DNS #1", Address: "185.55.226.26"},
		{Name: "Begzar DNS #2", Address: "185.55.225.25"},
	}
	activeDNSName     = "Direct Connection"
	activeDNSResolver *DNSResolver
	dnsTestOnce       sync.Once

	// HTTP client pool
	httpClientCache sync.Map // key: dns address, value: *http.Client

	// Token budget
	maxContextTokens = 8000 // estimated budget
)

func main() {
	// Signal handling
	setupSignalHandler()

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
	checkpointDir = filepath.Join(nootyDir, "checkpoints")
	_ = os.MkdirAll(nootyDir, 0700)
	_ = os.MkdirAll(chatsDir, 0700)
	_ = os.MkdirAll(checkpointDir, 0700)
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

	// DNS racing: find fastest active DNS
	dnsTestOnce.Do(raceDNS)

	drawHeader()
	repl()
}

// ---------- Minimal Sleek Header ----------
func drawHeader() {
	width := 64
	line := strings.Repeat("─", width-2)

	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), centerText(" NOOTY CLI ", width-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), centerText("v0.3 Radin Pro Max — Agentic Terminal Intelligence", width-2), c(cyan), c(reset))
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
	// Increase buffer for long inputs
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for {
		fmt.Print(prompt())
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Multiline input: triple quotes or <<< syntax
		if strings.HasPrefix(line, `"""`) || strings.HasPrefix(line, "<<<") {
			var delimiter string
			if strings.HasPrefix(line, `"""`) {
				delimiter = `"""`
			} else {
				delimiter = "<<<"
			}
			var multiline strings.Builder
			multiline.WriteString(strings.TrimPrefix(line, delimiter))
			for {
				fmt.Print("... ")
				if !scanner.Scan() {
					break
				}
				nextLine := scanner.Text()
				if strings.TrimSpace(nextLine) == delimiter {
					break
				}
				multiline.WriteString("\n")
				multiline.WriteString(nextLine)
			}
			line = strings.TrimSpace(multiline.String())
			if line == "" {
				continue
			}
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
	case "/compact":
		compactHistory()
	case "/sessions":
		listSessions()
	case "/resume":
		resumeSession(parts[1:])
	case "/session":
		handleSessionCommand(parts[1:])
	case "/undo":
		undoLastChange()
	case "/export":
		exportChat()
	case "/exit":
		saveCurrentSession()
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
  /clear                       Reset current screen & session memory
  /compact                     Summarize old history to save tokens
  /sessions                    List saved chat sessions
  /resume <name>               Resume a previous session
  /session save|load|list      Manage sessions manually
  /undo                        Revert last file modification
  /export                      Export current chat to Markdown
  /exit                        Save session and terminate NootyCLI

  💡 In Agent CLI Mode: Prefix commands with ! for direct shell execution.
  💡 Multiline input: Start with """ or <<< and end with same delimiter.`)
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

// ---------- DNS Racing (Concurrent Speed Test) ----------
func raceDNS() {
	fmt.Print(c(dim) + "🔍 Testing DNS resolvers..." + c(reset))
	results := make(chan struct {
		name string
		ms   time.Duration
	}, len(fallbackDNS))

	var wg sync.WaitGroup
	for _, dns := range fallbackDNS {
		wg.Add(1)
		go func(d DNSResolver) {
			defer wg.Done()
			start := time.Now()
			client := getHTTPClient(d.Address)
			req, _ := http.NewRequest("GET", "https://www.google.com/generate_204", nil)
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode < 400 {
				elapsed := time.Since(start)
				results <- struct {
					name string
					ms   time.Duration
				}{d.Name, elapsed}
				resp.Body.Close()
			}
		}(dns)
	}
	wg.Wait()
	close(results)

	best := ""
	bestMs := time.Duration(1<<63 - 1)
	for r := range results {
		if r.ms < bestMs {
			bestMs = r.ms
			best = r.name
		}
	}

	if best != "" {
		activeDNSName = best
		fmt.Printf("%s fastest: %s (%v)%s\n", c(green), best, bestMs, c(reset))
	} else {
		fmt.Println(c(yellow) + " all failed, using direct connection." + c(reset))
	}
}

// ---------- Network Transport Engine ----------
func dnsDialer(dnsServer string) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{}
		return d.DialContext(ctx, network, dnsServer+":53")
	}
}

func getHTTPClient(dns string) *http.Client {
	if dns == "" {
		// Direct connection
		if c, ok := httpClientCache.Load(""); ok {
			return c.(*http.Client)
		}
		client := &http.Client{
			Timeout: 35 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		}
		httpClientCache.Store("", client)
		return client
	}

	if c, ok := httpClientCache.Load(dns); ok {
		return c.(*http.Client)
	}

	resolver := &net.Resolver{
		PreferGo: true,
		Dial:     dnsDialer(dns),
	}
	dialer := &net.Dialer{Resolver: resolver}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: 35 * time.Second,
	}
	httpClientCache.Store(dns, client)
	return client
}

// doWithFallback with retry/backoff
func doWithFallback(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	maxRetries := 3
	baseDelay := 1 * time.Second

	// Use the active DNS first if set
	order := []DNSResolver{}
	if activeDNSResolver != nil {
		order = append(order, *activeDNSResolver)
	}
	for _, d := range fallbackDNS {
		if d.Name != activeDNSName {
			order = append(order, d)
		}
	}

	var lastErr error
	for _, dns := range order {
		client := getHTTPClient(dns.Address)
		for attempt := 0; attempt < maxRetries; attempt++ {
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
			if err == nil {
				// Check for retryable status
				if resp.StatusCode < 500 && resp.StatusCode != 408 && resp.StatusCode != 429 {
					activeDNSName = dns.Name
					activeDNSResolver = &dns
					return resp, nil
				}
				// Non-retryable status
				if resp.StatusCode == 403 || resp.StatusCode == 451 {
					resp.Body.Close()
					break // try next DNS
				}
				// 5xx or timeout: retry with backoff
				resp.Body.Close()
				if attempt < maxRetries-1 {
					time.Sleep(baseDelay * time.Duration(1<<attempt))
				}
			} else {
				lastErr = err
				if attempt < maxRetries-1 {
					time.Sleep(baseDelay * time.Duration(1<<attempt))
				}
			}
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("network connection failed after all attempts: %v", lastErr)
	}
	return nil, fmt.Errorf("all anti-sanction resolvers exhausted")
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
	// Auto-compact if needed
	if len(sessionMessages) > 15 {
		compactHistory()
	}

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
- patch_file (path="relative_path", old_str="exact_block", new_str="replacement")
- replace_in_file (path="relative_path", old="exact_string", new="replacement")
- append_file (path="relative_path", content="text_to_append")
- search_code (query="text", path="relative_path")
- find_files (pattern="glob", path="relative_path")   // e.g., "*.go", "src/**"
- file_info (path="relative_path")
- git_status
- git_diff
- git_quick_commit (message="commit message")   // auto stage all & commit
- lint_or_check (command="go vet ./..." or "go test ./...")
- run_command (command="shell_cmd", timeout="seconds")
- count_tokens (path="relative_path")   // estimate tokens in file

IMPORTANT: Use EXACT tool format. You may issue multiple TOOL calls in one response, one per line. Independent tools will be run in parallel.`

	relevant := getRelevantMemories(userInput)
	if len(relevant) > 0 {
		sysPrompt += "\n\nUser Context & Memories:\n"
		for _, m := range relevant {
			sysPrompt += fmt.Sprintf("- [%s] %s\n", m.Tag, m.Content)
		}
	}

	msgs = append(msgs, Message{Role: "system", Content: sysPrompt})

	// Token-aware history selection
	histLimit := 10
	tokenBudget := maxContextTokens - estimateTokens([]Message{{Role: "system", Content: sysPrompt}}) - estimateTokens([]Message{{Role: "user", Content: userInput}}) - 500 // reserve for response
	selectedHistory := selectHistoryByTokenBudget(sessionMessages, tokenBudget, histLimit)
	msgs = append(msgs, selectedHistory...)

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

// Token estimation: ~4 chars per token
func estimateTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)/4 + 4
	}
	return total
}

func selectHistoryByTokenBudget(history []Message, tokenBudget, minRecent int) []Message {
	if len(history) == 0 {
		return nil
	}
	// Always keep last minRecent messages
	if len(history) <= minRecent {
		return history
	}
	// Start from the most recent and go backwards until budget is exceeded
	selected := make([]Message, 0)
	totalTokens := 0
	for i := len(history) - 1; i >= 0; i-- {
		msgTokens := estimateTokens([]Message{history[i]})
		if totalTokens+msgTokens > tokenBudget {
			break
		}
		selected = append([]Message{history[i]}, selected...)
		totalTokens += msgTokens
	}
	// If we couldn't include minRecent, force include them
	if len(selected) < minRecent {
		selected = history[len(history)-minRecent:]
	}
	return selected
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

	tokenCount := 0
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
				tokenCount += len(content) / 4
			}
		}
	}

	elapsed := time.Since(start)
	speed := float64(tokenCount) / elapsed.Seconds()
	fmt.Print(c(reset) + "\n\n")
	fmt.Printf("%s⚡ ~%d tokens | %.1f tok/s | %.1fs | Model: %s%s\n", c(dim), tokenCount, speed, elapsed.Seconds(), config.Model, c(reset))

	sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: fullContent.String()})
	saveCurrentSession()
}

// ---------- Agentic Plan & Execute Loop ----------
func runAgentLoop(messages []Message) {
	// Panic recovery
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s⚠️ Agent crashed: %v. Session preserved.%s\n", c(red), r, c(reset))
		}
	}()

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

	stepLimit := 15
	for step := 0; step < stepLimit; step++ {
		// Get model response (could contain multiple tool calls)
		respText, err := getModelResponseTextStreaming(msgs) // stream while receiving
		if err != nil {
			fmt.Printf("%s❌ Agent Execution Error: %v%s\n", c(red), err, c(reset))
			return
		}

		toolCalls := extractToolCalls(respText)
		if len(toolCalls) == 0 {
			// No tools: just output the response and end
			fmt.Println("\n" + c(green) + respText + c(reset) + "\n")
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText})
			saveCurrentSession()
			return
		}

		// Show tool calls
		for i, tc := range toolCalls {
			fmt.Printf("\n%s🔧 Agent Action [%d.%d]: %s%s\n", c(bold)+c(yellow), step+1, i+1, tc.Name, c(reset))
			for k, v := range tc.Args {
				fmt.Printf("   %s%s%s: %s\n", c(dim), k, c(reset), v)
			}
		}

		// Execute tools: parallel if independent (no dependencies among them)
		results := make([]string, len(toolCalls))
		approved := make([]bool, len(toolCalls))
		var wg sync.WaitGroup
		for i, tc := range toolCalls {
			wg.Add(1)
			go func(idx int, tool ToolCall) {
				defer wg.Done()
				// Check approval and execute
				res, appr := executeAgentTool(&tool)
				results[idx] = res
				approved[idx] = appr
				if !appr {
					fmt.Printf("%s (denied)%s\n", res, c(yellow))
				}
			}(i, tc)
		}
		wg.Wait()

		// Prepare tool results for next iteration
		feedback := "Tool results:\n"
		for i, tc := range toolCalls {
			if approved[i] {
				// Truncate long outputs
				out := results[i]
				if len(out) > 2000 {
					out = out[:2000] + "\n... (truncated)"
				}
				feedback += fmt.Sprintf("[%d] %s: %s\n", i+1, tc.Name, out)
				fmt.Printf("%s📄 %s Output:%s\n%s\n", c(dim), tc.Name, c(reset), out)
			} else {
				feedback += fmt.Sprintf("[%d] %s: DENIED\n", i+1, tc.Name)
			}
		}

		// Reflection & self-correction step
		msgs = append(msgs,
			Message{Role: "assistant", Content: respText},
			Message{Role: "user", Content: feedback},
		)
	}

	fmt.Printf("%s⚠️ Agent loop step limit reached (%d steps).%s\n", c(yellow), stepLimit, c(reset))
}

// getModelResponseTextStreaming streams response but returns full text (used in agent loop)
func getModelResponseTextStreaming(messages []Message) (string, error) {
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
	var full strings.Builder
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
				full.WriteString(content)
			}
		}
	}
	fmt.Print(c(reset) + "\n")
	return full.String(), nil
}

// Non-streaming version for planning
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

// Extract multiple tool calls from response text (one per line)
func extractToolCalls(text string) []ToolCall {
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
	// If no line-based, try regex over whole text
	if len(calls) == 0 {
		re := regexp.MustCompile(`(?i)TOOL:\s*(\w+)\s+(.*)`)
		matches := re.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if len(m) >= 3 {
				tc := parseToolArgs(m[1], m[2])
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
func executeAgentTool(tc *ToolCall) (string, bool) {
	needsApproval := true
	switch tc.Name {
	case "list_files", "tree", "read_file", "search_code", "file_info", "git_status", "git_diff",
		"find_files", "count_tokens":
		needsApproval = false
	case "run_command":
		if config.Safety == "balanced" {
			needsApproval = false
		}
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

// runTool executes a named tool with args and returns output string
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
		// Checkpoint
		backupFile(path)
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ File saved (%d bytes): %s", len(content), args["path"]), nil

	case "patch_file":
		path := safeJoin(workspace, args["path"])
		oldStr := args["old_str"]
		newStr := args["new_str"]
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		content := string(data)
		if !strings.Contains(content, oldStr) {
			return "❌ old_str block not found in file. Ensure exact match.", nil
		}
		backupFile(path)
		newContent := strings.Replace(content, oldStr, newStr, 1)
		err = os.WriteFile(path, []byte(newContent), 0644)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Patched: %s", args["path"]), nil

	case "replace_in_file":
		path := safeJoin(workspace, args["path"])
		oldStr := args["old"]
		newStr := args["new"]
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		content := string(data)
		if !strings.Contains(content, oldStr) {
			return "❌ old string not found in file.", nil
		}
		backupFile(path)
		newContent := strings.Replace(content, oldStr, newStr, 1)
		err = os.WriteFile(path, []byte(newContent), 0644)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Replaced: %s", args["path"]), nil

	case "append_file":
		path := safeJoin(workspace, args["path"])
		content := args["content"]
		backupFile(path)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return "", err
		}
		defer f.Close()
		if _, err := f.WriteString(content); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Appended to: %s", args["path"]), nil

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
		return searchCodeInDir(scope, query), nil

	case "find_files":
		pattern := args["pattern"]
		if pattern == "" {
			return "❌ pattern required (e.g., *.go)", nil
		}
		scope := workspace
		if s, ok := args["path"]; ok && s != "" {
			scope = safeJoin(workspace, s)
		}
		return findFilesByGlob(scope, pattern), nil

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

	case "git_quick_commit":
		msg := args["message"]
		if msg == "" {
			msg = "auto commit"
		}
		// Stage all
		cmd := exec.Command("git", "add", ".")
		cmd.Dir = workspace
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git add failed: %v %s", err, out)
		}
		// Commit
		cmd = exec.Command("git", "commit", "-m", msg)
		cmd.Dir = workspace
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git commit failed: %v %s", err, out)
		}
		return "✅ Committed with message: " + msg, nil

	case "lint_or_check":
		command := args["command"]
		if command == "" {
			return "❌ command required (e.g., go vet ./...)", nil
		}
		// Run command, stream output
		return runStreamingCommand(command, workspace)

	case "run_command":
		cmdStr := args["command"]
		if cmdStr == "" {
			return "❌ command required", nil
		}
		timeout := 60
		if t, ok := args["timeout"]; ok {
			_, _ = fmt.Sscanf(t, "%d", &timeout)
		}
		return runStreamingCommandWithTimeout(cmdStr, workspace, timeout)

	case "count_tokens":
		path := safeJoin(workspace, args["path"])
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		tokens := len(data) / 4
		return fmt.Sprintf("Estimated tokens in %s: %d", args["path"], tokens), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// Helper: .gitignore-aware directory walk for search_code
func searchCodeInDir(root, query string) string {
	ignored := readGitignorePatterns(root)
	var results []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip ignored directories
			rel, _ := filepath.Rel(root, path)
			if rel == "." {
				return nil
			}
			if isIgnoredPath(rel, ignored) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if isIgnoredPath(rel, ignored) {
			return nil
		}
		if info.Size() > 1_000_000 {
			return nil
		}
		// Open and scan line by line
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			if strings.Contains(scanner.Text(), query) {
				results = append(results, fmt.Sprintf("%s:%d", rel, lineNum))
			}
		}
		return nil
	})
	if len(results) == 0 {
		return "No matches found."
	}
	return strings.Join(results, "\n")
}

func findFilesByGlob(root, pattern string) string {
	ignored := readGitignorePatterns(root)
	var matches []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			if isIgnoredPath(rel, ignored) {
				return filepath.SkipDir
			}
			return nil
		}
		if isIgnoredPath(rel, ignored) {
			return nil
		}
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			matches = append(matches, rel)
		}
		return nil
	})
	if len(matches) == 0 {
		return "No files matched."
	}
	return strings.Join(matches, "\n")
}

// Simple .gitignore parser
func readGitignorePatterns(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var patterns []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Remove trailing slash
		line = strings.TrimSuffix(line, "/")
		patterns = append(patterns, line)
	}
	return patterns
}

func isIgnoredPath(rel string, patterns []string) bool {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		// Simple match: if pattern contains *, use filepath.Match
		if strings.Contains(p, "*") || strings.Contains(p, "?") {
			matched, err := filepath.Match(p, rel)
			if err == nil && matched {
				return true
			}
			// Also match directory prefix
			if strings.HasPrefix(rel, strings.TrimSuffix(p, "/*")) {
				return true
			}
		} else {
			// Exact or prefix match
			if rel == p || strings.HasPrefix(rel, p+"/") || strings.HasPrefix(rel, p) {
				return true
			}
		}
	}
	return false
}

// Run command with live streaming and optional timeout
func runStreamingCommandWithTimeout(command, dir string, timeout int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Dir = dir

	// Use a MultiWriter to both capture and stream to console
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(&outBuf, &dimWriter{os.Stdout})
	cmd.Stderr = io.MultiWriter(&errBuf, &dimWriter{os.Stderr})

	err := cmd.Run()
	out := outBuf.String()
	if errBuf.Len() > 0 {
		out += "\n[stderr]\n" + errBuf.String()
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("execution timed out (%d sec)", timeout)
		}
		return out, fmt.Errorf("command failed: %v", err)
	}
	if out == "" {
		out = "(command executed successfully with no output)"
	}
	return out, nil
}

// For run_command and lint_or_check (no timeout)
func runStreamingCommand(command, dir string) (string, error) {
	return runStreamingCommandWithTimeout(command, dir, 0) // 0 means no timeout
}

type dimWriter struct {
	w io.Writer
}

func (d *dimWriter) Write(p []byte) (int, error) {
	// Print with dim color
	fmt.Fprint(d.w, c(dim)+string(p)+c(reset))
	return len(p), nil
}

func dirTree(root, indent string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err.Error()
	}
	// Respect .gitignore
	ignored := readGitignorePatterns(root)
	var out string
	for i, e := range entries {
		rel, _ := filepath.Rel(root, filepath.Join(root, e.Name()))
		if isIgnoredPath(rel, ignored) {
			continue
		}
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

// ---------- Checkpoint & Undo ----------
func backupFile(path string) {
	// Save copy to checkpoint dir with timestamp
	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	backupPath := filepath.Join(checkpointDir, rel+"."+time.Now().Format("20060102_150405"))
	_ = os.MkdirAll(filepath.Dir(backupPath), 0755)
	data, err := os.ReadFile(path)
	if err == nil {
		_ = os.WriteFile(backupPath, data, 0644)
	}
}

func undoLastChange() {
	// Find latest backup and restore
	backups, _ := filepath.Glob(filepath.Join(checkpointDir, "*"))
	if len(backups) == 0 {
		fmt.Println("No checkpoints found.")
		return
	}
	// Sort by mod time
	sort.Slice(backups, func(i, j int) bool {
		infoI, _ := os.Stat(backups[i])
		infoJ, _ := os.Stat(backups[j])
		return infoI.ModTime().After(infoJ.ModTime())
	})
	latest := backups[0]
	// Extract original relative path
	rel := strings.TrimSuffix(filepath.Base(latest), filepath.Ext(latest))
	originalPath := safeJoin(workspace, rel)
	data, err := os.ReadFile(latest)
	if err != nil {
		fmt.Printf("Failed to read backup: %v\n", err)
		return
	}
	err = os.WriteFile(originalPath, data, 0644)
	if err != nil {
		fmt.Printf("Failed to restore: %v\n", err)
		return
	}
	// Remove the used backup
	_ = os.Remove(latest)
	fmt.Printf("✅ Restored %s from checkpoint.\n", originalPath)
}

// ---------- Session Persistence ----------
func saveCurrentSession() {
	if len(sessionMessages) == 0 {
		return
	}
	if currentSession == nil {
		currentSession = &Session{
			Name:     time.Now().Format("session_20060102_150405"),
			Messages: sessionMessages,
			Mode:     currentMode,
			Created:  time.Now(),
		}
	} else {
		currentSession.Messages = sessionMessages
		currentSession.Mode = currentMode
	}
	data, _ := json.MarshalIndent(currentSession, "", "  ")
	filename := filepath.Join(chatsDir, currentSession.Name+".json")
	_ = os.WriteFile(filename, data, 0600)
}

func listSessions() {
	files, err := filepath.Glob(filepath.Join(chatsDir, "*.json"))
	if err != nil || len(files) == 0 {
		fmt.Println("No saved sessions.")
		return
	}
	fmt.Println(c(bold) + "\n📚 Saved Sessions:" + c(reset))
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		info, _ := os.Stat(f)
		fmt.Printf("  %-40s %s\n", name, info.ModTime().Format(time.RFC3339))
	}
	fmt.Println()
}

func resumeSession(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: /resume <session_name>")
		return
	}
	name := args[0]
	filename := filepath.Join(chatsDir, name+".json")
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("Session not found: %s\n", name)
		return
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		fmt.Printf("Failed to load session: %v\n", err)
		return
	}
	currentSession = &sess
	sessionMessages = sess.Messages
	currentMode = sess.Mode
	fmt.Printf("✅ Loaded session '%s' (%d messages).\n", name, len(sessionMessages))
}

func handleSessionCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: /session save|load|list [name]")
		return
	}
	switch args[0] {
	case "save":
		if len(args) > 1 {
			currentSession = &Session{Name: args[1], Messages: sessionMessages, Mode: currentMode, Created: time.Now()}
		}
		saveCurrentSession()
		fmt.Println("Session saved.")
	case "load":
		if len(args) < 2 {
			fmt.Println("Usage: /session load <name>")
			return
		}
		resumeSession(args[1:])
	case "list":
		listSessions()
	default:
		fmt.Println("Unknown subcommand.")
	}
}

// ---------- Context Compaction ----------
func compactHistory() {
	if len(sessionMessages) < 6 {
		fmt.Println("Not enough messages to compact.")
		return
	}
	// Keep last 3 messages, summarize older
	older := sessionMessages[:len(sessionMessages)-3]
	recent := sessionMessages[len(sessionMessages)-3:]

	summaryPrompt := []Message{
		{Role: "system", Content: "Summarize the following conversation into a compact context block (max 200 words). Include important facts, decisions, and code references."},
		{Role: "user", Content: formatMessagesForSummary(older)},
	}
	summary, err := getModelResponseText(summaryPrompt)
	if err != nil {
		fmt.Printf("%s⚠️ Compaction failed: %v%s\n", c(yellow), err, c(reset))
		return
	}
	sessionMessages = append([]Message{{Role: "system", Content: "## Conversation Summary\n" + summary}}, recent...)
	fmt.Println(c(green) + "✅ History compacted." + c(reset))
	saveCurrentSession()
}

func formatMessagesForSummary(msgs []Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(m.Role[:1])+m.Role[1:], m.Content))
	}
	return sb.String()
}

// ---------- Export Chat ----------
func exportChat() {
	if len(sessionMessages) == 0 {
		fmt.Println("No chat to export.")
		return
	}
	filename := filepath.Join(chatsDir, "export_"+time.Now().Format("20060102_150405")+".md")
	var sb strings.Builder
	for _, m := range sessionMessages {
		if m.Role == "user" {
			sb.WriteString("**You:** " + m.Content + "\n\n")
		} else if m.Role == "assistant" {
			sb.WriteString("**Nooty:** " + m.Content + "\n\n---\n\n")
		} else if m.Role == "system" {
			sb.WriteString("*System:* " + m.Content + "\n\n")
		}
	}
	_ = os.WriteFile(filename, []byte(sb.String()), 0644)
	fmt.Printf("✅ Exported to %s\n", filename)
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

// ---------- Signal Handling ----------
func setupSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			if currentMode == "cli" {
				fmt.Println("\n⏸ Agent interrupted. Type /resume or continue.")
				// Could set a flag to stop agent loop
			} else {
				fmt.Println("\n👋 Goodbye!")
				saveCurrentSession()
				os.Exit(0)
			}
		}
	}()
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

// ---------- Model Management ----------
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
