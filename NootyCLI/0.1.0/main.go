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
)

// Config Structure
type Config struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	Safety    string `json:"safety"`
	ActiveDNS string `json:"active_dns"`
}

// Memory Structure
type Memory struct {
	ID        int       `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Flexible API Error Struct (Handles string OR struct JSON formats)
type APIError struct {
	Message string
}

func (e *APIError) UnmarshalJSON(data []byte) error {
	var strVal string
	if err := json.Unmarshal(data, &strVal); err == nil {
		e.Message = strVal
		return nil
	}
	var structVal struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &structVal); err == nil {
		e.Message = structVal.Message
		return nil
	}
	e.Message = string(data)
	return nil
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
	Error *APIError `json:"error,omitempty"`
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

// Anti-Sanction DNS Pool (Electro, Shecan, Begzar)
var dnsPool = []struct {
	Name string
	IP   string
}{
	{"Electro", "10.202.10.202"},
	{"Shecan", "178.22.122.100"},
	{"Begzar", "185.55.226.26"},
}

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

		// System Slash Commands
		if strings.HasPrefix(input, "/") {
			handleCommand(input, scanner)
			continue
		}

		// Direct Raw Shell Execution (! prefix)
		if strings.HasPrefix(input, "!") {
			if state.Mode != "cli" {
				fmt.Printf("%s[!] Raw shell execution is only allowed in 'cli' mode.%s\n", Yellow, Reset)
				continue
			}
			cmdStr := strings.TrimPrefix(input, "!")
			executeShellCommand(cmdStr)
			continue
		}

		// Autonomous AI Agent Loop Processing
		processAIQueryAutonomous(input, scanner)
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
			ActiveDNS: "Electro (10.202.10.202)",
		}
		saveConfig()
	} else {
		data, _ := os.ReadFile(configPath)
		_ = json.Unmarshal(data, &state.Config)
	}

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

// Mask Sensitive API Key
func maskAPIKey(key string) string {
	if key == "" {
		return fmt.Sprintf("%s[NOT SET - Use /config]%s", Red, Reset)
	}
	if len(key) <= 12 {
		return "••••••••••••"
	}
	return key[:8] + "••••••••" + key[len(key)-4:]
}

// Claude Code Style Banner & System Dashboard
func renderHeader() {
	fmt.Print("\033[H\033[2J") // Clear terminal screen
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
		Bold, Reset, width-44, state.Config.ActiveDNS,
		Cyan, Reset,
	)
	fmt.Printf("%s└%s┘%s\n", Cyan, line, Reset)
	fmt.Printf("%sType %s/help%s for commands | %s/config%s for settings | %s/model%s to switch AI.\n\n", Dim+Yellow, Bold, Reset+Dim, Bold, Reset+Dim, Bold, Reset)
}

func truncatePath(p string, max int) string {
	if len(p) <= max {
		return p
	}
	return "..." + p[len(p)-max+3:]
}

// Command Handler
func handleCommand(input string, scanner *bufio.Scanner) {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/help":
		fmt.Println(Bold + "\n─── Nooty CLI System Commands ───" + Reset)
		fmt.Println("  /config            - Interactively configure API Key & Base URL")
		fmt.Println("  /model             - Select or change AI models")
		fmt.Println("  /mode chat|cli     - Switch between Chat Mode & Autonomous CLI Agent Mode")
		fmt.Println("  /workspace set|show- Set target project directory")
		fmt.Println("  /memory list|add   - Manage local smart context memories")
		fmt.Println("  /doctor            - Run Network & Anti-Sanction Smart DNS test")
		fmt.Println("  /clear             - Refresh terminal screen & status header")
		fmt.Println("  /exit              - Exit NootyCLI\n")

	case "/config":
		configureSystemInteractive(scanner)

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

// Interactive Configuration Manager (/config)
func configureSystemInteractive(scanner *bufio.Scanner) {
	fmt.Println(Bold + "\n─── System Interactive Settings ───" + Reset)

	// 1. API Key Input
	fmt.Printf("Current API Key: %s\n", maskAPIKey(state.Config.APIKey))
	fmt.Print(Yellow + "Enter new API Key (press Enter to keep current): " + Reset)
	if scanner.Scan() {
		newKey := strings.TrimSpace(scanner.Text())
		if newKey != "" {
			state.Config.APIKey = newKey
		}
	}

	// 2. Base URL Input
	fmt.Printf("\nCurrent Base URL: %s\n", state.Config.BaseURL)
	fmt.Println("Presets:")
	fmt.Println("  [1] OpenRouter AI (https://openrouter.ai/api/v1)")
	fmt.Println("  [2] DeepSeek Direct (https://api.deepseek.com/v1)")
	fmt.Println("  [3] Local Ollama (http://localhost:11434/v1)")
	fmt.Println("  [4] Custom URL")
	fmt.Print(Yellow + "Select option or press Enter to keep current: " + Reset)

	if scanner.Scan() {
		choice := strings.TrimSpace(scanner.Text())
		switch choice {
		case "1":
			state.Config.BaseURL = "https://openrouter.ai/api/v1"
		case "2":
			state.Config.BaseURL = "https://api.deepseek.com/v1"
		case "3":
			state.Config.BaseURL = "http://localhost:11434/v1"
		case "4":
			fmt.Print("Enter custom Base URL: ")
			if scanner.Scan() {
				customURL := strings.TrimSpace(scanner.Text())
				if customURL != "" {
					state.Config.BaseURL = customURL
				}
			}
		}
	}

	saveConfig()
	renderHeader()
	fmt.Printf("%s[✓] Configuration updated successfully!%s\n\n", Green, Reset)
}

// Model Selector
func selectModelInteractive(scanner *bufio.Scanner) {
	fmt.Println(Bold + "\n─── Select AI Model ───" + Reset)
	for idx, m := range presetModels {
		selectedMarker := " "
		if m.ID == state.Config.Model {
			selectedMarker = "✓"
		}
		fmt.Printf("  [%d] %s %-35s %s(%s)%s\n", idx+1, selectedMarker, m.Name, Dim, m.ID, Reset)
	}
	fmt.Printf("  [%d] Custom Model ID\n", len(presetModels)+1)

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
					fmt.Printf("%s[✓] Model set to: %s%s\n\n", Green, state.Config.Model, Reset)
					return
				}
			}
		}
	}
}

// Memory System
func handleMemoryCommands(parts []string) {
	if len(parts) < 2 {
		fmt.Println(Yellow + "Usage: /memory list | /memory add <content>" + Reset)
		return
	}

	switch parts[1] {
	case "list":
		fmt.Println(Bold + "\n─── Stored Smart Memories ───" + Reset)
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

// Autonomous Agent AI Processing Loop
func processAIQueryAutonomous(input string, scanner *bufio.Scanner) {
	if state.Config.APIKey == "" {
		fmt.Printf("%s[!] API Key missing. Run /config to set your key.%s\n", Red, Reset)
		return
	}

	systemPrompt := "You are Nooty, an expert senior developer AI assistant."
	if state.Mode == "cli" {
		systemPrompt = fmt.Sprintf("You are NootyCLI, an autonomous developer agent operating in workspace: %s. Use tools to inspect files, execute terminal commands, and solve problems iteratively until the task is complete.", state.Workspace)
	}

	if len(state.Memories) > 0 {
		systemPrompt += "\n\nUser Context Memories:"
		for _, m := range state.Memories {
			systemPrompt += fmt.Sprintf("\n- %s", m.Content)
		}
	}

	messages := []Message{
		{Role: "system", Content: systemPrompt},
	}
	messages = append(messages, state.SessionHistory...)
	messages = append(messages, Message{Role: "user", Content: input})

	// Autonomous Loop
	for loopCount := 0; loopCount < 10; loopCount++ {
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
			fmt.Printf("%s[!] API Request Error: %v%s\n", Red, err, Reset)
			return
		}

		if resp.Error != nil {
			fmt.Printf("%s[!] Provider API Error: %s%s\n", Red, resp.Error.Message, Reset)
			return
		}

		if len(resp.Choices) == 0 {
			fmt.Printf("%s[!] Received empty response from model.%s\n", Red, Reset)
			return
		}

		aiMsg := resp.Choices[0].Message
		messages = append(messages, aiMsg)

		// Check if AI requested tool calls
		if len(aiMsg.ToolCalls) > 0 {
			for _, tool := range aiMsg.ToolCalls {
				toolResult := executeToolCallAutonomous(tool, scanner)
				toolMsg := Message{
					Role:       "tool",
					ToolCallID: tool.ID,
					Content:    toolResult,
				}
				messages = append(messages, toolMsg)
			}
			// Loop continues: feed tool results back to AI!
			continue
		}

		// AI completed its task and gave final answer
		if aiMsg.Content != "" {
			fmt.Printf("\n%sNooty:%s %s\n\n", Bold+Green, Reset, aiMsg.Content)
		}

		state.SessionHistory = append(state.SessionHistory, Message{Role: "user", Content: input})
		state.SessionHistory = append(state.SessionHistory, aiMsg)
		break
	}
}

func getAvailableTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: ToolDefInReq{
				Name:        "list_files",
				Description: "List files inside the current workspace path",
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
				Description: "Read file contents",
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
				Name:        "run_command",
				Description: "Execute a terminal shell command",
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

// Execute Tool Calls & Return Output to AI Loop
func executeToolCallAutonomous(tool ToolCall, scanner *bufio.Scanner) string {
	fnName := tool.Function.Name
	var args map[string]string
	_ = json.Unmarshal([]byte(tool.Function.Arguments), &args)

	fmt.Printf("\n%s➔ Agent Executing Tool:%s %s%s%s\n", Magenta, Reset, Bold, fnName, Reset)

	switch fnName {
	case "list_files":
		relPath := args["path"]
		fullPath := filepath.Join(state.Workspace, relPath)
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return fmt.Sprintf("Error listing files: %v", err)
		}
		var list []string
		for _, e := range entries {
			t := "[FILE]"
			if e.IsDir() {
				t = "[DIR ]"
			}
			list = append(list, fmt.Sprintf("%s %s", t, e.Name()))
		}
		out := strings.Join(list, "\n")
		fmt.Printf("   Found %d entries.\n", len(entries))
		return out

	case "read_file":
		relPath := args["path"]
		fullPath := filepath.Join(state.Workspace, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Sprintf("Error reading file: %v", err)
		}
		fmt.Printf("   Read %s (%d bytes)\n", relPath, len(content))
		return string(content)

	case "write_file":
		relPath := args["path"]
		content := args["content"]
		fullPath := filepath.Join(state.Workspace, relPath)

		fmt.Printf("   Target File: %s%s%s\n", Yellow, relPath, Reset)
		if askApprovalInteractive("Allow write to file?", scanner) {
			_ = os.MkdirAll(filepath.Dir(fullPath), 0755)
			err := os.WriteFile(fullPath, []byte(content), 0644)
			if err != nil {
				return fmt.Sprintf("Error writing file: %v", err)
			}
			fmt.Printf("%s[✓] File written successfully.%s\n", Green, Reset)
			return "File written successfully."
		}
		return "User denied write approval."

	case "run_command":
		cmdStr := args["command"]
		fmt.Printf("   Command: %s%s%s\n", Yellow, cmdStr, Reset)
		if askApprovalInteractive("Allow executing command?", scanner) {
			cmd := exec.Command("bash", "-c", cmdStr)
			cmd.Dir = state.Workspace
			out, err := cmd.CombinedOutput()
			fmt.Printf("\n%s--- Execution Output ---%s\n%s\n", Dim, Reset, string(out))
			if err != nil {
				return fmt.Sprintf("Command failed with error: %v. Output: %s", err, string(out))
			}
			return string(out)
		}
		return "User denied command execution."
	}

	return "Unknown tool name"
}

func askApprovalInteractive(prompt string, scanner *bufio.Scanner) bool {
	fmt.Printf("%s[?] %s [y/N]: %s", Yellow, prompt, Reset)
	if scanner.Scan() {
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return ans == "y" || ans == "yes"
	}
	return false
}

func executeShellCommand(cmdStr string) {
	fmt.Printf("%s➔ Executing Shell Command:%s %s%s%s\n", Yellow, Reset, Bold, cmdStr, Reset)
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = state.Workspace
	out, err := cmd.CombinedOutput()
	fmt.Printf("\n%s--- Output ---%s\n%s\n", Dim, Reset, string(out))
	if err != nil {
		fmt.Printf("%s[!] Command failed: %v%s\n", Red, err, Reset)
	}
}

// Connection Doctor & Triple Anti-Sanction DNS Pool Tester
func runConnectionDoctor() {
	fmt.Println(Bold + "\n─── Network & Anti-Sanction Diagnostic ───" + Reset)
	fmt.Printf("1. Testing Provider Endpoint: %s\n", state.Config.BaseURL)

	client := http.Client{Timeout: 4 * time.Second}
	_, err := client.Get(state.Config.BaseURL)
	if err == nil {
		fmt.Printf("%s[✓] Direct Connection Healthy! No sanctions detected.%s\n\n", Green, Reset)
		return
	}

	fmt.Printf("%s[!] Direct connection blocked or slow. Testing Anti-Sanction DNS Pool...%s\n\n", Yellow, Reset)

	for _, dns := range dnsPool {
		fmt.Printf(" Testing %s DNS (%s)... ", dns.Name, dns.IP)
		dialer := &net.Dialer{Timeout: 3 * time.Second}
		transport := &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				resolver := &net.Resolver{
					PreferGo: true,
					Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
						return dialer.DialContext(ctx, "udp", dns.IP+":53")
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

		smartClient := http.Client{Transport: transport, Timeout: 4 * time.Second}
		_, err := smartClient.Get(state.Config.BaseURL)
		if err == nil {
			fmt.Printf("%s[✓] ACTIVE & WORKING!%s\n", Green, Reset)
			state.Config.ActiveDNS = fmt.Sprintf("%s (%s)", dns.Name, dns.IP)
			saveConfig()
			renderHeader()
			return
		}
		fmt.Printf("%s[FAILED]%s\n", Red, Reset)
	}
	fmt.Printf("\n%s[!] All Anti-Sanction DNS routes failed. Check network connection.%s\n\n", Red, Reset)
}

// HTTP API Request Engine
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

	client := &http.Client{Timeout: 90 * time.Second}
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

// Storage Helpers
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
