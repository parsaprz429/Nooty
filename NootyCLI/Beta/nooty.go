// nooty.go — NootyCLI v0.3.0 "Radin Pro V2" – Agentic Terminal Intelligence
// Single‑file, zero external dependencies, cross-platform (macOS / Linux / Windows / WSL).
//
// 🚀 Compile & Build:
//   go build -ldflags="-s -w" -o nooty nooty.go

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
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
	OriginalPath string `json:"original_path"`
	BackupPath   string `json:"backup_path"`
	WasDeleted   bool   `json:"was_deleted"`
	Timestamp    string `json:"timestamp"`
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
	chatsDir        string
	checkpointsDir  string
	memFile         string
	configFile      string
	sessionID       string
	lastCheckpoint  *Checkpoint

	fallbackDNS = []DNSResolver{
		{Name: "Direct Connection", Address: ""},
		{Name: "Electro DNS", Address: "78.157.42.100"},
		{Name: "Shecan DNS #1", Address: "178.22.122.100"},
		{Name: "Shecan DNS #2", Address: "185.51.200.2"},
		{Name: "Begzar DNS #1", Address: "185.55.226.26"},
		{Name: "Begzar DNS #2", Address: "185.55.225.25"},
	}
	activeDNSName = "Direct Connection"
	
	ignoreDirs = map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "venv": true, "__pycache__": true, "dist": true, "build": true,
	}
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
	sessionID = time.Now().Format("20060102_150405")

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

// ---------- Visuals ----------
func drawHeader() {
	width := 64
	line := strings.Repeat("─", width-2)

	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), centerText(" NOOTY CLI 0.3 ", width-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), centerText("Radin Pro V2 — Agentic Terminal Intelligence", width-2), c(cyan), c(reset))
	fmt.Println(c(cyan) + "├" + line + "┤" + c(reset))

	prettyWorkspace := formatPath(workspace)

	entries := [][]string{
		{"Provider", truncateString(config.ProviderEndpoint, 38)},
		{"Model", config.Model},
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
	if len(parts) == 0 { return }

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
	case "/workspace": handleWorkspace(parts[1:])
	case "/model": handleModelCommand(parts[1:])
	case "/config": handleConfig()
	case "/dns": showDNSStatus()
	case "/doctor": runDoctor()
	case "/memory": handleMemory(parts[1:])
	case "/safety": handleSafety(parts[1:])
	case "/history": showHistory()
	case "/sessions": listSessions()
	case "/resume":
		if len(parts) > 1 {
			resumeSession(parts[1])
		} else {
			fmt.Println("❌ Usage: /resume <session_id>")
		}
	case "/undo": undoLastAction()
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
  /mode [chat|cli]             Toggle Chat or Agentic CLI Execution Mode
  /config                      Interactive wizard to setup API key, endpoint & model
  /workspace show|set <path>   Manage current working directory
  /model show|set <name>|list  View, switch, or browse models interactively
  /dns                         Display Anti-Sanction Smart DNS Shield status
  /doctor                      Run full connection and API health check
  /memory list|add|forget      Manage long-term persistent agent context
  /safety strict|balanced      Set command safety confirmation policies
  /history                     Display conversation session log
  /sessions                    List saved chat sessions
  /resume <id>                 Resume a previous chat session
  /undo                        Undo the last file modification (write/patch/delete)
  /clear                       Reset current screen & session memory
  /exit                        Terminate NootyCLI session

  💡 In Agent CLI Mode: Prefix commands with ! for direct shell execution.`)
}

// ... [Omitted boilerplate simple handlers: handleConfig, showDNSStatus, handleModelCommand, selectModelInteractive for brevity, same as before] ...
func handleConfig() { /* Same as v0.2.2 */ }
func showDNSStatus() { /* Same as v0.2.2 */ }
func handleModelCommand(args []string) { /* Same as v0.2.2 */ }
func selectModelInteractive() { /* Same as v0.2.2 */ }


// ---------- Network Transport Engine (with Retry/Backoff) ----------
func dnsDialer(dnsServer string) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{}
		return d.DialContext(ctx, network, dnsServer+":53")
	}
}

func httpClientForDNS(dns string) *http.Client {
	if dns == "" {
		return &http.Client{Timeout: 45 * time.Second}
	}
	resolver := &net.Resolver{PreferGo: true, Dial: dnsDialer(dns)}
	dialer := &net.Dialer{Resolver: resolver}
	return &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext},
		Timeout:   45 * time.Second,
	}
}

func doWithFallback(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	maxRetries := 3
	for i, dnsResolver := range fallbackDNS {
		client := httpClientForDNS(dnsResolver.Address)
		
		for attempt := 0; attempt < maxRetries; attempt++ {
			var req *http.Request
			var err error

			if body != nil {
				req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
			} else {
				req, err = http.NewRequest(method, url, nil)
			}
			if err != nil { return nil, err }

			for k, v := range headers { req.Header.Set(k, v) }

			resp, err := client.Do(req)
			
			// Success criteria
			if err == nil && resp.StatusCode != 403 && resp.StatusCode != 451 && resp.StatusCode < 500 {
				activeDNSName = dnsResolver.Name
				return resp, nil
			}

			if resp != nil { _ = resp.Body.Close() }

			// Exponential Backoff on 5xx or timeouts
			if attempt < maxRetries-1 {
				backoff := time.Duration(1<<attempt) * time.Second
				time.Sleep(backoff)
			}
		}

		if i < len(fallbackDNS)-1 {
			fmt.Printf("%s⚠️ Connection issue (%s). Bypassing via %s...%s\n",
				c(yellow), dnsResolver.Name, fallbackDNS[i+1].Name, c(reset))
		}
	}
	return nil, fmt.Errorf("network connection failed: all anti-sanction resolvers exhausted")
}

func fetchAvailableModels() ([]string, error) {
	// ... Same as v0.2.2 ...
	return []string{"qwen-coder", "gpt-4o"}, nil // mock for brevity in this block, use original in full
}

// ---------- Smart Context Management ----------
func estimateTokens(text string) int {
	return len(text) / 4 // Simple fast estimator
}

func manageContextBudget(messages []Message) []Message {
	const MaxTokens = 4000
	var finalMsgs []Message
	totalTokens := 0
	
	// Always keep the system prompt (assumed to be at index 0)
	if len(messages) > 0 && messages[0].Role == "system" {
		totalTokens += estimateTokens(messages[0].Content)
		finalMsgs = append(finalMsgs, messages[0])
	}
	
	var historyMsgs []Message
	// Walk backwards through history to fit token budget
	for i := len(messages) - 1; i >= 1; i-- {
		msgTokens := estimateTokens(messages[i].Content)
		if totalTokens+msgTokens > MaxTokens {
			// Summarize dropped older messages
			summary := Message{
				Role: "system",
				Content: "[System Note: Older conversation history was truncated due to context limits.]",
			}
			historyMsgs = append([]Message{summary}, historyMsgs...)
			break
		}
		totalTokens += msgTokens
		historyMsgs = append([]Message{messages[i]}, historyMsgs...)
	}
	
	finalMsgs = append(finalMsgs, historyMsgs...)
	return finalMsgs
}

func buildMessages(userInput string) []Message {
	var msgs []Message
	sysPrompt := `You are NootyCLI, an autonomous agentic terminal AI assistant.

When in CHAT mode: Provide concise, expert terminal and software engineering responses.

When in CLI mode: You act as an autonomous workspace agent.
You can output MULTIPLE tools in a single response to run them in parallel if they don't depend on each other.
To execute tools, reply STRICTLY using this exact syntax (one per line):
TOOL: tool_name key1="value1" key2="value2"

Available Workspace Tools:
- list_files (path="relative_path")
- tree (path="relative_path")
- read_file (path="relative_path")
- write_file (path="relative_path", content="full_content")
- patch_file (path="relative_path", search="exact_old_text", replace="new_text")
- delete_file (path="relative_path")
- search_code (query="text", path="relative_path")
- file_info (path="relative_path")
- run_command (command="shell_cmd", timeout="seconds")

IMPORTANT: Use EXACT tool format.`

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
	saveSession()

	return manageContextBudget(msgs)
}

func getRelevantMemories(query string) []Memory {
	q := strings.ToLower(query)
	var res []Memory
	for _, m := range memories {
		if strings.Contains(strings.ToLower(m.Content), q) || strings.Contains(strings.ToLower(m.Tag), q) {
			res = append(res, m)
		}
	}
	if len(res) > 5 { res = res[:5] }
	return res
}

// ---------- Core Execution Engine (Streaming + Parallel) ----------
func handleChat(input string) {
	messages := buildMessages(input)
	if currentMode == "cli" {
		runAgentLoop(messages)
		return
	}
	streamResponse(messages)
}

func streamResponse(messages []Message) string {
	reqPayload := ChatRequest{Model: config.Model, Messages: messages, Stream: true}
	jsonData, _ := json.Marshal(reqPayload)
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
	headers := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" { headers["Authorization"] = "Bearer " + config.APIKey }

	resp, err := doWithFallback("POST", endpoint, jsonData, headers)
	if err != nil {
		fmt.Printf("%s❌ Request error: %v%s\n", c(red), err, c(reset))
		return ""
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	var fullContent strings.Builder
	fmt.Print(c(cyan))

	for {
		line, err := reader.ReadString('\n')
		if err != nil { break }
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") { continue }
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" { break }

		var chunk ChatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err == nil {
			for _, choice := range chunk.Choices {
				fmt.Print(choice.Delta.Content)
				fullContent.WriteString(choice.Delta.Content)
			}
		}
	}

	fmt.Print(c(reset) + "\n\n")
	contentStr := fullContent.String()
	if currentMode == "chat" {
		sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: contentStr})
		saveSession()
	}
	return contentStr
}

func runAgentLoop(messages []Message) {
	planPrompt := append(messages, Message{Role: "user", Content: "Plan your actions and use TOOL calls to execute them."})
	
	for step := 0; step < 10; step++ {
		fmt.Print(c(yellow) + "🤔 Agent is thinking...\n" + c(reset))
		respText := streamResponse(planPrompt)
		if respText == "" { return }

		sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText})
		saveSession()

		toolCalls := extractToolCalls(respText)
		if len(toolCalls) == 0 {
			// No more tools, agent is done.
			return
		}

		fmt.Printf("\n%s🔧 Found %d Tool Call(s) to execute...%s\n", c(bold)+c(yellow), len(toolCalls), c(reset))
		
		var wg sync.WaitGroup
		var mu sync.Mutex
		results := make([]string, len(toolCalls))
		allApproved := true

		// Execute tools in parallel
		for i, tc := range toolCalls {
			wg.Add(1)
			go func(idx int, tool *ToolCall) {
				defer wg.Done()
				
				mu.Lock()
				fmt.Printf("  ▶️ Preparing: %s\n", tool.Name)
				mu.Unlock()

				toolResult, approved := executeAgentTool(tool)
				
				mu.Lock()
				if !approved { allApproved = false }
				
				if len(toolResult) > 2500 {
					toolResult = toolResult[:2500] + "\n... (truncated)"
				}
				results[idx] = fmt.Sprintf("Output of %s:\n%s", tool.Name, toolResult)
				mu.Unlock()
			}(i, tc)
		}
		
		wg.Wait()
		
		if !allApproved {
			planPrompt = append(planPrompt, Message{Role: "assistant", Content: respText}, Message{Role: "user", Content: "Some actions were denied by user safety policy."})
			continue
		}

		combinedResult := strings.Join(results, "\n\n---\n\n")
		planPrompt = append(planPrompt, Message{Role: "assistant", Content: respText}, Message{Role: "user", Content: combinedResult})
	}
	fmt.Printf("%s⚠️ Agent loop step limit reached.%s\n", c(yellow), c(reset))
}

func extractToolCalls(text string) []*ToolCall {
	var calls []*ToolCall
	lines := strings.Split(text, "\n")
	re := regexp.MustCompile(`(?i)TOOL:\s*(\w+)\s+(.*)`)
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 3 {
			call := parseToolArgs(matches[1], matches[2])
			if call != nil {
				calls = append(calls, call)
			}
		}
	}
	return calls
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
	if len(args) == 0 && strings.TrimSpace(argsStr) != "" { args["path"] = strings.TrimSpace(argsStr) }
	return &ToolCall{Name: name, Args: args}
}

// ---------- Tool & Filesystem Engine ----------
func createCheckpoint(targetPath string, isDelete bool) {
	absPath := safeJoin(workspace, targetPath)
	if _, err := os.Stat(absPath); os.IsNotExist(err) && !isDelete {
		return // File doesn't exist yet, nothing to backup
	}

	backupName := fmt.Sprintf("%d_%s.bak", time.Now().UnixNano(), filepath.Base(absPath))
	backupPath := filepath.Join(checkpointsDir, backupName)
	
	data, _ := os.ReadFile(absPath)
	_ = os.WriteFile(backupPath, data, 0644)
	
	lastCheckpoint = &Checkpoint{
		OriginalPath: absPath,
		BackupPath:   backupPath,
		WasDeleted:   isDelete,
		Timestamp:    time.Now().Format(time.RFC3339),
	}
}

func undoLastAction() {
	if lastCheckpoint == nil {
		fmt.Println("❌ No recent file modifications to undo.")
		return
	}
	
	if lastCheckpoint.WasDeleted {
		data, _ := os.ReadFile(lastCheckpoint.BackupPath)
		_ = os.WriteFile(lastCheckpoint.OriginalPath, data, 0644)
		fmt.Printf("✅ Restored deleted file: %s\n", lastCheckpoint.OriginalPath)
	} else {
		if _, err := os.Stat(lastCheckpoint.BackupPath); err == nil {
			data, _ := os.ReadFile(lastCheckpoint.BackupPath)
			_ = os.WriteFile(lastCheckpoint.OriginalPath, data, 0644)
			fmt.Printf("✅ Reverted modifications to: %s\n", lastCheckpoint.OriginalPath)
		} else {
			_ = os.Remove(lastCheckpoint.OriginalPath)
			fmt.Printf("✅ Undid file creation (removed): %s\n", lastCheckpoint.OriginalPath)
		}
	}
	lastCheckpoint = nil // clear after use
}

func executeAgentTool(tc *ToolCall) (string, bool) {
	needsApproval := true
	switch tc.Name {
	case "list_files", "tree", "read_file", "search_code", "file_info":
		needsApproval = false
	}

	if needsApproval && config.Safety == "strict" {
		fmt.Printf("\n%s⚠️ Tool %s needs approval.%s\n", c(yellow), tc.Name, c(reset))
		fmt.Print("Confirm execution? [Y/n]: ")
		reader := bufio.NewReader(os.Stdin)
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))
		if confirm == "n" || confirm == "no" {
			return "Operation cancelled by user.", false
		}
	}

	result, err := runTool(tc.Name, tc.Args)
	if err != nil { return fmt.Sprintf("Tool Error: %v", err), true }
	return result, true
}

func runTool(name string, args map[string]string) (string, error) {
	switch name {
	case "list_files":
		path := workspace
		if p, ok := args["path"]; ok && p != "" { path = safeJoin(workspace, p) }
		entries, err := os.ReadDir(path)
		if err != nil { return "", err }
		var names []string
		for _, e := range entries {
			if ignoreDirs[e.Name()] { continue }
			if e.IsDir() { names = append(names, e.Name()+"/") } else { names = append(names, e.Name()) }
		}
		if len(names) == 0 { return "(directory empty)", nil }
		return strings.Join(names, "\n"), nil

	case "tree":
		path := workspace
		if p, ok := args["path"]; ok && p != "" { path = safeJoin(workspace, p) }
		return dirTree(path, ""), nil

	case "read_file":
		path := safeJoin(workspace, args["path"])
		data, err := os.ReadFile(path)
		if err != nil { return "", err }
		return string(data), nil

	case "write_file":
		path := safeJoin(workspace, args["path"])
		createCheckpoint(args["path"], false)
		content := args["content"]
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil { return "", err }
		return fmt.Sprintf("✅ File saved (%d bytes): %s", len(content), args["path"]), nil

	case "patch_file":
		path := safeJoin(workspace, args["path"])
		createCheckpoint(args["path"], false)
		
		data, err := os.ReadFile(path)
		if err != nil { return "", err }
		
		strData := string(data)
		search := args["search"]
		replace := args["replace"]
		
		if !strings.Contains(strData, search) {
			return "", fmt.Errorf("search string not found in file")
		}
		
		strData = strings.Replace(strData, search, replace, 1)
		err = os.WriteFile(path, []byte(strData), 0644)
		if err != nil { return "", err }
		return "✅ File successfully patched: " + args["path"], nil

	case "delete_file":
		path := safeJoin(workspace, args["path"])
		createCheckpoint(args["path"], true)
		if err := os.Remove(path); err != nil { return "", err }
		return "✅ File removed: " + args["path"], nil

	case "search_code":
		query := args["query"]
		scope := workspace
		if s, ok := args["path"]; ok && s != "" { scope = safeJoin(workspace, s) }
		
		var results []string
		_ = filepath.Walk(scope, func(p string, info os.FileInfo, err error) error {
			if err != nil { return nil }
			if info.IsDir() {
				if ignoreDirs[info.Name()] || strings.HasPrefix(info.Name(), ".") { return filepath.SkipDir }
				return nil
			}
			
			// Line-by-line memory efficient search
			file, err := os.Open(p)
			if err != nil { return nil }
			defer file.Close()
			
			scanner := bufio.NewScanner(file)
			lineNum := 1
			for scanner.Scan() {
				if strings.Contains(scanner.Text(), query) {
					rel, _ := filepath.Rel(workspace, p)
					results = append(results, fmt.Sprintf("%s:%d: %s", rel, lineNum, strings.TrimSpace(scanner.Text())))
				}
				lineNum++
			}
			return nil
		})
		if len(results) == 0 { return "No matches found.", nil }
		return strings.Join(results, "\n"), nil

	case "run_command":
		cmdStr := args["command"]
		timeout := 60
		if t, ok := args["timeout"]; ok { _, _ = fmt.Sscanf(t, "%d", &timeout) }

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", cmdStr)
		} else {
			cmd = exec.Command("sh", "-c", cmdStr)
		}
		cmd.Dir = workspace

		// Live Streaming output for commands
		var outBuf bytes.Buffer
		multiWriter := io.MultiWriter(os.Stdout, &outBuf)
		
		cmd.Stdout = multiWriter
		cmd.Stderr = multiWriter

		fmt.Printf("\n%s▶️ Executing: %s%s\n", c(cyan), cmdStr, c(reset))
		if err := cmd.Start(); err != nil { return "", err }

		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		select {
		case err := <-done:
			out := outBuf.String()
			if err != nil { return out + fmt.Sprintf("\nExit status: %v", err), nil }
			if out == "" { out = "(command executed successfully with no output)" }
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
	if err != nil { return err.Error() }
	var out string
	var validEntries []os.DirEntry
	
	for _, e := range entries {
		if !ignoreDirs[e.Name()] && !strings.HasPrefix(e.Name(), ".") {
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
	if !strings.HasPrefix(abs, base) { return base }
	return abs
}

// ---------- Direct Shell Engine ----------
func handleShellBang(cmd string) {
	cmd = strings.TrimSpace(cmd)
	fmt.Printf("\n%s⚡ Direct Shell Command:%s %s\n", c(yellow), c(reset), cmd)
	executeShell(cmd)
}

func executeShell(command string) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" { cmd = exec.Command("cmd", "/C", command)
	} else { cmd = exec.Command("sh", "-c", command) }
	cmd.Dir = workspace
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("%s❌ Command failed: %v%s\n", c(red), err, c(reset))
	}
}

// ---------- Persistence Methods (Sessions & Config) ----------
func saveSession() {
	if len(sessionMessages) == 0 { return }
	path := filepath.Join(chatsDir, sessionID+".json")
	data, _ := json.MarshalIndent(sessionMessages, "", "  ")
	_ = os.WriteFile(path, data, 0644)
}

func listSessions() {
	files, _ := os.ReadDir(chatsDir)
	if len(files) == 0 {
		fmt.Println("📂 No saved sessions found.")
		return
	}
	fmt.Println(c(bold) + "\n📂 Saved Sessions:" + c(reset))
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			id := strings.TrimSuffix(f.Name(), ".json")
			fmt.Printf("  • %s\n", id)
		}
	}
	fmt.Println()
}

func resumeSession(id string) {
	path := filepath.Join(chatsDir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("❌ Session '%s' not found.\n", id)
		return
	}
	var msgs []Message
	if err := json.Unmarshal(data, &msgs); err == nil {
		sessionMessages = msgs
		sessionID = id // Keep using same ID to append
		fmt.Printf("✅ Session '%s' resumed successfully! (%d messages)\n", id, len(sessionMessages))
	} else {
		fmt.Println("❌ Failed to parse session file.")
	}
}

// ... [loadConfig, saveConfig, loadMemories, saveMemories - Same as v0.2.2] ...
func loadConfig() { /* Same as v0.2.2 */ }
func saveConfig() { /* Same as v0.2.2 */ }
func loadMemories() { /* Same as v0.2.2 */ }
func saveMemories() { /* Same as v0.2.2 */ }
func handleWorkspace(args []string) { /* Same as v0.2.2 */ }
func runDoctor() { /* Same as v0.2.2 */ }
func handleMemory(args []string) { /* Same as v0.2.2 */ }
func handleSafety(args []string) { /* Same as v0.2.2 */ }
func showHistory() { /* Same as v0.2.2 */ }
