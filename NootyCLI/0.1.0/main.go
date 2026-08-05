package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ANSI Color Codes
const (
	Reset       = "\033[0m"
	Bold        = "\033[1m"
	Dim         = "\033[2m"
	Red         = "\033[31m"
	Green       = "\033[32m"
	Yellow      = "\033[33m"
	Blue        = "\033[34m"
	Magenta     = "\033[35m"
	Cyan        = "\033[36m"
	White       = "\033[37m"
	BgDarkGray  = "\033[48;5;235m"
)

// Config Structure
type Config struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	Safety    string `json:"safety"`
	CustomDNS string `json:"custom_dns"`
}

// Memory Structure
type Memory struct {
	ID        int       `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// OpenAI API Standard Structures
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Function struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolDefInReq `json:"function"`
}

type ToolDefInReq struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type ChatCompletionReq struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}

type ChatCompletionResp struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Application Global State
type AppState struct {
	Mode           string // "chat" or "cli"
	Workspace      string
	Config         Config
	Memories       []Memory
	SessionHistory []Message
	HomeDir        string
	NootyDir       string
}

var state AppState

// Popular Preset Models
var presetModels = []struct {
	Name string
	ID   string
}{
	{"Qwen 2.5 Coder 32B (Recommended)", "qwen/qwen-2.5-coder-32b-instruct"},
	{"DeepSeek V3 (Fast & Smart)", "deepseek/deepseek-chat"},
	{"DeepSeek R1 (Reasoning Master)", "deepseek/deepseek-r1"},
	{"Claude 3.5 Sonnet (Anthropic)", "anthropic/claude-3.5-sonnet"},
	{"GPT-4o Mini (OpenAI)", "openai/gpt-4o-mini"},
}

func main() {
	initAppState()
	renderHeader()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		promptPrefix := fmt.Sprintf("%s%snooty[%s]%s ❯ ", Bold, Cyan, state.Mode, Reset)
		if state.Mode == "chat" {
			promptPrefix = fmt.Sprintf("%s%snooty%s ❯ ", Bold, Green, Reset)
		}

		fmt.Print(promptPrefix)
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle System Slash Commands
		if strings.HasPrefix(input, "/") {
			handleCommand(input, scanner)
			continue
		}

		// Direct Raw Shell Execution with ! prefix
		if strings.HasPrefix(input, "!") {
			if state.Mode != "cli" {
				fmt.Printf("%s[!] Raw shell execution is only allowed in 'cli' mode.%s\n", Yellow, Reset)
				continue
			}
			cmdStr := strings.TrimPrefix(input, "!")
			executeShellCommand(cmdStr)
			continue
		}

		// AI Query Processing
		processAIQuery(input)
	}
}

// Initialize Application Settings
func initAppState() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	state.HomeDir = home
	state.NootyDir = filepath.Join(home, ".nooty")
	_ = os.MkdirAll(state.NootyDir, 0755)

	pwd, err := os.Getwd()
	if err != nil {
		pwd = "."
	}
	state.Workspace = pwd
	state.Mode = "chat"

	configPath := filepath.Join(state.NootyDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		state.Config = Config{
			BaseURL:   "https://openrouter.ai/api/v1",
			APIKey:    os.Getenv("NOOTY_API_KEY"),
			Model:     "qwen/qwen-2.5-coder-32b-instruct",
			Safety:    "strict",
			CustomDNS: "10.202.10.202",
		}
		saveConfig()
	} else {
		data, _ := os.ReadFile(configPath)
		_ = json.Unmarshal(data, &state.Config)
	}

	// Safety fallback check for URLs
	if state.Config.BaseURL == "" {
		state.Config.BaseURL = "https://openrouter.ai/api/v1"
	}
	if state.Config.Model == "" {
		state.Config.Model = "qwen/qwen-2.5-coder-32b-instruct"
	}
	if envKey := os.Getenv("NOOTY_API_KEY"); envKey != "" {
		state.Config.APIKey = envKey
	}

	loadMemories()
}

// Mask Sensitive API Key (Shows first 8 and last 4 chars)
func maskAPIKey(key string) string {
	if key == "" {
		return fmt.Sprintf("%s[NOT SET - Edit ~/.nooty/config.json]%s", Red, Reset)
	}
	if len(key) <= 12 {
		return "••••••••••••"
	}
	return key[:8] + "••••••••" + key[len(key)-4:]
}

// Claude Code Style Beautiful ASCII Banner & Info Box
func renderHeader() {
	fmt.Print("\033[H\033[2J") // Clear screen
	asciiLogo := `
  _  _  ___   ___  _____ __   __  ___  _     ___ 
 | \| |/ _ \ / _ \|_   _|\ \ / / / __|| |   |_ _|
 | .` + "`" + ` | (_) | (_) | | |   \ V / | (__ | |__  | | 
 |_|\_|\___/ \___/  |_|    |_|   \___||____||___|`

	width := 64
	line := strings.Repeat("─", width-2)

	fmt.Printf("%s┌%s┐%s\n", Cyan, line, Reset)
	for _, l := range strings.Split(asciiLogo, "\n") {
		if strings.TrimSpace(l) != "" {
			fmt.Printf("%s│%s%-*s%s│%s\n", Cyan, Bold+Magenta, width-2, l, Cyan, Reset)
		}
	}
	fmt.Printf("%s├%s┤%s\n", Cyan, line, Reset)

	// Meta Information Section
	provider := state.Config.BaseURL
	if strings.Contains(provider, "openrouter") {
		provider = "OpenRouter AI (" + provider + ")"
	}

	fmt.Printf("%s│%s %sProvider:%s  %-*s %s│%s\n", Cyan, Reset, Bold, Reset, width-14, provider, Cyan, Reset)
	fmt.Printf("%s│%s %sModel:%s     %-*s %s│%s\n", Cyan, Reset, Bold, Reset, width-14, state.Config.Model, Cyan, Reset)
	fmt.Printf("%s│%s %sAPI Key:%s   %-*s %s│%s\n", Cyan, Reset, Bold, Reset, width-14, maskAPIKey(state.Config.APIKey), Cyan, Reset)
	fmt.Printf("%s│%s %sWorkspace:%s %-*s %s│%s\n", Cyan, Reset, Bold, Reset, width-14, truncatePath(state.Workspace, width-15), Cyan, Reset)
	fmt.Printf("%s│%s %sMode:%s %s%-6s%s │ %sSafety:%s %s%-8s%s │ %sDNS:%s %-*s %s│%s\n",
		Cyan, Reset,
		Bold, Reset, Green, state.Mode, Reset,
		Bold, Reset, Yellow, state.Config.Safety, Reset,
		Bold, Reset, width-44, state.Config.CustomDNS,
		Cyan, Reset,
	)
	fmt.Printf("%s└%s┘%s\n", Cyan, line, Reset)
	fmt.Printf("%sType %s/help%s for system commands | %s/model%s to switch AI models.\n\n", Dim+Yellow, Bold, Reset+Dim, Bold, Reset)
}

func truncatePath(p string, max int) string {
	if len(p) <= max {
		return p
	}
	return "..." + p[len(p)-max+3:]
}

// Slash Command Router
func handleCommand(input string, scanner *bufio.Scanner) {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/help":
		fmt.Println(Bold + "\n─── Nooty CLI Commands ───" + Reset)
		fmt.Println("  /model             - Interactively select or search AI models")
		fmt.Println("  /mode chat|cli     - Switch between Conversation & Agent CLI Mode")
		fmt.Println("  /workspace set|show- Manage workspace target directory")
		fmt.Println("  /memory list|add   - Manage local smart memories")
		fmt.Println("  /doctor            - Run Network & Anti-Sanction Smart DNS test")
		fmt.Println("  /clear             - Refresh terminal screen & status banner")
		fmt.Println("  /exit              - Exit NootyCLI\n")

	case "/model":
		selectModelInteractive(scanner)

	case "/mode":
		if len(parts) > 1 {
			m := strings.ToLower(parts[1])
			if m == "chat" || m == "cli" {
				state.Mode = m
				saveConfig()
				renderHeader()
			} else {
				fmt.Println(Yellow + "Usage: /mode chat  OR  /mode cli" + Reset)
			}
		} else {
			fmt.Printf("Current mode: %s%s%s\n", Green, state.Mode, Reset)
		}

	case "/clear":
		renderHeader()

	case "/workspace":
		if len(parts) > 1 && parts[1] == "set" && len(parts) > 2 {
			absPath, err := filepath.Abs(parts[2])
			if err == nil {
				state.Workspace = absPath
				renderHeader()
			}
		} else {
			fmt.Printf("Workspace: %s\n", state.Workspace)
		}

	case "/memory":
		handleMemoryCommands(parts)

	case "/doctor":
		runConnectionDoctor()

	case "/exit":
		fmt.Println("Goodbye from Nooty! 🚀")
		os.Exit(0)

	default:
		fmt.Printf("%s[!] Unknown command: %s. Type /help%s\n", Red, cmd, Reset)
	}
}

// Interactive Model Picker
func selectModelInteractive(scanner *bufio.Scanner) {
	fmt.Println(Bold + "\n─── Select AI Model ───" + Reset)
	for idx, m := range presetModels {
		selectedMarker := " "
		if m.ID == state.Config.Model {
			selectedMarker = "✓"
		}
		fmt.Printf("  [%d] %s %-35s %s(%s)%s\n", idx+1, selectedMarker, m.Name, Dim, m.ID, Reset)
	}
	fmt.Printf("  [%d] Custom Model ID (Type manually)\n", len(presetModels)+1)

	fmt.Printf("\n%sSelect option (1-%d): %s", Yellow, len(presetModels)+1, Reset)
	if scanner.Scan() {
		choiceStr := strings.TrimSpace(scanner.Text())
		choice, err := strconv.Atoi(choiceStr)
		if err == nil && choice >= 1 && choice <= len(presetModels) {
			state.Config.Model = presetModels[choice-1].ID
			saveConfig()
			renderHeader()
			fmt.Printf("%s[✓] Model switched to: %s%s\n\n", Green, state.Config.Model, Reset)
			return
		} else if choice == len(presetModels)+1 {
			fmt.Print("Enter custom model ID (e.g. google/gemini-2.5-flash): ")
			if scanner.Scan() {
				customID := strings.TrimSpace(scanner.Text())
				if customID != "" {
					state.Config.Model = customID
					saveConfig()
					renderHeader()
					fmt.Printf("%s[✓] Custom model set to: %s%s\n\n", Green, state.Config.Model, Reset)
					return
				}
			}
		}
	}
	fmt.Println("No changes made.")
}

// Local Memory Subsystem
func handleMemoryCommands(parts []string) {
	if len(parts) < 2 {
		fmt.Println(Yellow + "Usage: /memory list | /memory add <content>" + Reset)
		return
	}

	switch parts[1] {
	case "list":
		fmt.Println(Bold + "\n─── Stored Local Smart Memories ───" + Reset)
		if len(state.Memories) == 0 {
			fmt.Println("No memories saved yet.")
		}
		for _, m := range state.Memories {
			fmt.Printf(" [%d] %s (%s)\n", m.ID, m.Content, m.CreatedAt.Format("2006-01-02 15:04"))
		}
		fmt.Println()

	case "add":
		if len(parts) < 3 {
			fmt.Println(Yellow + "Specify content to remember." + Reset)
			return
		}
		content := strings.Join(parts[2:], " ")
		newMem := Memory{
			ID:        len(state.Memories) + 1,
			Content:   content,
			CreatedAt: time.Now(),
		}
		state.Memories = append(state.Memories, newMem)
		saveMemories()
		fmt.Printf("%s[✓] Memory saved: \"%s\"%s\n", Green, content, Reset)
	}
}

// AI Engine Query Processor
func processAIQuery(input string) {
	if state.Config.APIKey == "" {
		fmt.Printf("%s[!] API Key missing. Set NOOTY_API_KEY environment variable or edit ~/.nooty/config.json%s\n", Red, Reset)
		return
	}

	systemPrompt := "You are Nooty, a senior developer AI assistant."
	if state.Mode == "cli" {
		systemPrompt = fmt.Sprintf("You are NootyCLI Agent operating in workspace: %s. Use tools to read/write files or execute commands as requested.", state.Workspace)
	}

	if len(state.Memories) > 0 {
		systemPrompt += "\n\nUser Preferences & Context Memories:"
		for _, m := range state.Memories {
			systemPrompt += fmt.Sprintf("\n- %s", m.Content)
		}
	}

	messages := []Message{
		{Role: "system", Content: systemPrompt},
	}
	messages = append(messages, state.SessionHistory...)
	messages = append(messages, Message{Role: "user", Content: input})

	reqBody := ChatCompletionReq{
		Model:    state.Config.Model,
		Messages: messages,
	}

	if state.Mode == "cli" {
		reqBody.Tools = getAvailableTools()
	}

	fmt.Printf("%s⚡ Nooty thinking...%s\r", Dim+Cyan, Reset)
	resp, err := callNootyAPI(reqBody)
	if err != nil {
		fmt.Printf("%s[!] API Error: %v%s\n", Red, err, Reset)
		return
	}

	if resp.Error != nil {
		fmt.Printf("%s[!] Provider Error: %s%s\n", Red, resp.Error.Message, Reset)
		return
	}

	if len(resp.Choices) == 0 {
		fmt.Printf("%s[!] Received empty response.%s\n", Red, Reset)
		return
	}

	aiMsg := resp.Choices[0].Message

	// Handle Agent Tools Call
	if len(aiMsg.ToolCalls) > 0 {
		state.SessionHistory = append(state.SessionHistory, Message{Role: "user", Content: input})
		state.SessionHistory = append(state.SessionHistory, aiMsg)

		for _, tool := range aiMsg.ToolCalls {
			executeToolCall(tool)
		}
	} else {
		fmt.Printf("\n%sNooty:%s %s\n\n", Bold+Green, Reset, aiMsg.Content)
		state.SessionHistory = append(state.SessionHistory, Message{Role: "user", Content: input})
		state.SessionHistory = append(state.SessionHistory, aiMsg)
	}
}

// Tool Definitions
func getAvailableTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: ToolDefInReq{
				Name:        "list_files",
				Description: "List files inside workspace",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ToolDefInReq{
				Name:        "read_file",
				Description: "Read file content",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolDefInReq{
				Name:        "write_file",
				Description: "Create or edit a file",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":    map[string]interface{}{"type": "string"},
						"content": map[string]interface{}{"type": "string"},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolDefInReq{
				Name:        "delete_file",
				Description: "Delete a file",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolDefInReq{
				Name:        "run_command",
				Description: "Execute shell command",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{"type": "string"},
					},
					"required": []string{"command"},
				},
			},
		},
	}
}

// Agent Tool Execution Dispatcher
func executeToolCall(tool ToolCall) {
	fnName := tool.Function.Name
	var args map[string]string
	_ = json.Unmarshal([]byte(tool.Function.Arguments), &args)

	fmt.Printf("\n%s➔ Agent Tool Call:%s %s%s%s\n", Magenta, Reset, Bold, fnName, Reset)

	switch fnName {
	case "list_files":
		relPath := args["path"]
		fullPath := filepath.Join(state.Workspace, relPath)
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			sendToolResult(tool.ID, fmt.Sprintf("Error: %v", err))
			return
		}
		var list []string
		for _, e := range entries {
			t := "[FILE]"
			if e.IsDir() {
				t = "[DIR ]"
			}
			list = append(list, fmt.Sprintf("%s %s", t, e.Name()))
		}
		sendToolResult(tool.ID, strings.Join(list, "\n"))

	case "read_file":
		relPath := args["path"]
		fullPath := filepath.Join(state.Workspace, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			sendToolResult(tool.ID, fmt.Sprintf("Error: %v", err))
			return
		}
		fmt.Printf("   Reading: %s%s%s (%d bytes)\n", Cyan, relPath, Reset, len(content))
		sendToolResult(tool.ID, string(content))

	case "write_file":
		relPath := args["path"]
		content := args["content"]
		fullPath := filepath.Join(state.Workspace, relPath)

		fmt.Printf("   Target: %s%s%s\n", Yellow, relPath, Reset)
		if askApproval("Allow writing to this file?") {
			_ = os.MkdirAll(filepath.Dir(fullPath), 0755)
			err := os.WriteFile(fullPath, []byte(content), 0644)
			if err != nil {
				sendToolResult(tool.ID, fmt.Sprintf("Error: %v", err))
			} else {
				fmt.Printf("%s[✓] File written successfully.%s\n", Green, Reset)
				sendToolResult(tool.ID, "File written successfully.")
			}
		} else {
			sendToolResult(tool.ID, "User denied write approval.")
		}

	case "delete_file":
		relPath := args["path"]
		fullPath := filepath.Join(state.Workspace, relPath)
		fmt.Printf("%s⚠️ Warning: Delete %s%s\n", Red, relPath, Reset)
		if askApproval("Confirm DELETE file?") {
			err := os.Remove(fullPath)
			if err != nil {
				sendToolResult(tool.ID, fmt.Sprintf("Error: %v", err))
			} else {
				fmt.Printf("%s[✓] File deleted.%s\n", Green, Reset)
				sendToolResult(tool.ID, "File deleted successfully.")
			}
		} else {
			sendToolResult(tool.ID, "User denied delete.")
		}

	case "run_command":
		cmdStr := args["command"]
		executeShellCommand(cmdStr)
	}
}

func sendToolResult(toolID string, result string) {
	toolMsg := Message{
		Role:       "tool",
		ToolCallID: toolID,
		Content:    result,
	}
	state.SessionHistory = append(state.SessionHistory, toolMsg)
}

func askApproval(prompt string) bool {
	fmt.Printf("%s[?] %s [y/N]: %s", Yellow, prompt, Reset)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return ans == "y" || ans == "yes"
	}
	return false
}

func executeShellCommand(cmdStr string) {
	fmt.Printf("%s➔ Execute Command:%s %s%s%s\n", Yellow, Reset, Bold, cmdStr, Reset)
	if !askApproval("Allow running command?") {
		fmt.Println("Command cancelled.")
		return
	}

	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = state.Workspace
	out, err := cmd.CombinedOutput()

	fmt.Printf("\n%s--- Output ---%s\n%s\n", Dim, Reset, string(out))
	if err != nil {
		fmt.Printf("%s[!] Command failed: %v%s\n", Red, err, Reset)
	}
}

// Connection Doctor & Smart DNS Diagnostic
func runConnectionDoctor() {
	fmt.Println(Bold + "\n─── Network & Anti-Sanction Diagnostic ───" + Reset)
	fmt.Printf("1. Target Provider Endpoint: %s\n", state.Config.BaseURL)

	client := http.Client{Timeout: 5 * time.Second}
	_, err := client.Get(state.Config.BaseURL)
	if err == nil {
		fmt.Printf("%s[✓] Direct Connection Healthy! No blocks detected.%s\n\n", Green, Reset)
		return
	}

	fmt.Printf("%s[!] Direct test failed: %v%s\n", Yellow, err, Reset)
	fmt.Println("2. Testing Smart DNS Anti-Sanction Route...")

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					return dialer.DialContext(ctx, "udp", state.Config.CustomDNS+":53")
				},
			}
			host, port, _ := net.SplitHostPort(addr)
			ips, err := resolver.LookupHost(ctx, host)
			if err != nil || len(ips) == 0 {
				return dialer.DialContext(ctx, network, addr)
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
		},
	}

	smartClient := http.Client{Transport: transport, Timeout: 5 * time.Second}
	_, err = smartClient.Get(state.Config.BaseURL)
	if err == nil {
		fmt.Printf("%s[✓] Smart DNS (%s) route working! Anti-sanction ready.%s\n\n", Green, state.Config.CustomDNS, Reset)
	} else {
		fmt.Printf("%s[!] Anti-sanction route failed. Verify Internet/DNS config.%s\n\n", Red, Reset)
	}
}

// HTTP API Requester
func callNootyAPI(reqBody ChatCompletionReq) (*ChatCompletionResp, error) {
	jsonBytes, _ := json.Marshal(reqBody)
	baseURL := strings.TrimSuffix(state.Config.BaseURL, "/")
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	url := fmt.Sprintf("%s/chat/completions", baseURL)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", state.Config.APIKey))

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var compResp ChatCompletionResp
	err = json.Unmarshal(body, &compResp)
	return &compResp, err
}

// File Storage Helpers
func saveConfig() {
	data, _ := json.MarshalIndent(state.Config, "", "  ")
	_ = os.WriteFile(filepath.Join(state.NootyDir, "config.json"), data, 0644)
}

func loadMemories() {
	path := filepath.Join(state.NootyDir, "memories.json")
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &state.Memories)
	}
}

func saveMemories() {
	data, _ := json.MarshalIndent(state.Memories, "", "  ")
	_ = os.WriteFile(filepath.Join(state.NootyDir, "memories.json"), data, 0644)
}
