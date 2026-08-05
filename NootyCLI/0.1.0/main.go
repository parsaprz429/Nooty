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
	"strings"
	"time"
)

// ANSI Color Codes for Beautiful Terminal UI
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Purple  = "\033[35m"
	Cyan    = "\033[36m"
	Gray    = "\033[37m"
	BgBlack = "\033[40m"
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

// OpenAI / Nooty API Structures
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

// App State
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

func main() {
	initAppState()
	printBanner()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		promptPrefix := fmt.Sprintf("%s%snooty[%s]%s > ", Bold, Cyan, state.Mode, Reset)
		if state.Mode == "chat" {
			promptPrefix = fmt.Sprintf("%s%snooty%s > ", Bold, Green, Reset)
		}

		fmt.Print(promptPrefix)
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle Slash Commands
		if strings.HasPrefix(input, "/") {
			handleCommand(input)
			continue
		}

		// Handle Raw Shell Commands Execution (Prefix !)
		if strings.HasPrefix(input, "!") {
			if state.Mode != "cli" {
				fmt.Printf("%s[!] Raw shell execution is only allowed in 'cli' mode.%s\n", Yellow, Reset)
				continue
			}
			cmdStr := strings.TrimPrefix(input, "!")
			executeShellCommand(cmdStr, true)
			continue
		}

		// Process Query via AI
		processAIQuery(input)
	}
}

// Initialize Directories and Configurations
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

	// Load Config
	configPath := filepath.Join(state.NootyDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		state.Config = Config{
			BaseURL:   "https://openrouter.ai/api/v1",
			APIKey:    os.Getenv("NOOTY_API_KEY"),
			Model:     "qwen/qwen-2.5-coder-32b-instruct",
			Safety:    "strict",
			CustomDNS: "10.202.10.202", // Shecan DNS Default
		}
		saveConfig()
	} else {
		data, _ := os.ReadFile(configPath)
		_ = json.Unmarshal(data, &state.Config)
		if envKey := os.Getenv("NOOTY_API_KEY"); envKey != "" {
			state.Config.APIKey = envKey
		}
	}

	loadMemories()
}

func printBanner() {
	fmt.Printf("%s╭─────────────────────────────────────────────────────────╮%s\n", Cyan, Reset)
	fmt.Printf("%s│                 Nooty CLI & Agent v0.1.0                │%s\n", Bold+Cyan, Reset)
	fmt.Printf("%s│         Local-first Anti-Sanction Terminal Intelligence  │%s\n", Gray, Reset)
	fmt.Printf("%s╰─────────────────────────────────────────────────────────╯%s\n", Cyan, Reset)
	fmt.Printf("%sWorkspace:%s %s\n", Bold, Reset, state.Workspace)
	fmt.Printf("%sModel:%s %s | %sSafety:%s %s | %sMode:%s %s%s%s\n",
		Bold, Reset, state.Config.Model,
		Bold, Reset, state.Config.Safety,
		Bold, Reset, Green, state.Mode, Reset)
	fmt.Printf("%sType %s/help%s for available commands.\n\n", Yellow, Reset)
}

// Handle System Slash Commands
func handleCommand(input string) {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/help":
		fmt.Println(Bold + "\n--- Nooty CLI Internal Commands ---" + Reset)
		fmt.Println("  /mode chat|cli     - Switch between Chat & Agent Mode")
		fmt.Println("  /workspace show|set- View or change workspace directory")
		fmt.Println("  /memory list|add   - Manage local smart memories")
		fmt.Println("  /memory clear-session - Clear current chat history")
		fmt.Println("  /doctor            - Run Anti-Sanction & DNS Network Diagnostic")
		fmt.Println("  /safety status     - Check Safety & Approval status")
		fmt.Println("  /clear             - Clear terminal screen")
		fmt.Println("  /exit              - Quit NootyCLI\n")

	case "/mode":
		if len(parts) > 1 {
			m := strings.ToLower(parts[1])
			if m == "chat" || m == "cli" {
				state.Mode = m
				fmt.Printf("%s[✓] Mode switched to: %s%s\n", Green, state.Mode, Reset)
			} else {
				fmt.Println(Yellow + "Usage: /mode chat OR /mode cli" + Reset)
			}
		} else {
			fmt.Printf("Current mode: %s%s%s\n", Green, state.Mode, Reset)
		}

	case "/clear":
		fmt.Print("\033[H\033[2J")

	case "/workspace":
		if len(parts) > 1 && parts[1] == "set" && len(parts) > 2 {
			newPath := parts[2]
			absPath, err := filepath.Abs(newPath)
			if err == nil {
				state.Workspace = absPath
				fmt.Printf("%s[✓] Workspace updated to: %s%s\n", Green, state.Workspace, Reset)
			}
		} else {
			fmt.Printf("Current Workspace: %s\n", state.Workspace)
		}

	case "/memory":
		handleMemoryCommands(parts)

	case "/doctor":
		runConnectionDoctor()

	case "/safety":
		fmt.Printf("Safety Policy: %s%s%s\n", Yellow, state.Config.Safety, Reset)
		fmt.Println("Read operations: Auto-approved | Write/Delete/Shell: User approval required.")

	case "/exit":
		fmt.Println("Goodbye from Nooty! 🚀")
		os.Exit(0)

	default:
		fmt.Printf("%s[!] Unknown command: %s. Type /help%s\n", Red, cmd, Reset)
	}
}

// Memory System Handlers
func handleMemoryCommands(parts []string) {
	if len(parts) < 2 {
		fmt.Println(Yellow + "Usage: /memory list | /memory add \"content\" | /memory clear-session" + Reset)
		return
	}

	sub := parts[1]
	switch sub {
	case "list":
		fmt.Println(Bold + "\n--- Stored Local Smart Memories ---" + Reset)
		if len(state.Memories) == 0 {
			fmt.Println("No memories saved yet.")
		}
		for _, m := range state.Memories {
			fmt.Printf("[%d] %s (%s)\n", m.ID, m.Content, m.CreatedAt.Format("2006-01-02 15:04"))
		}
		fmt.Println()

	case "add":
		if len(parts) < 3 {
			fmt.Println(Yellow + "Specify memory text to add." + Reset)
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
		fmt.Printf("%s[✓] Local memory saved: \"%s\"%s\n", Green, content, Reset)

	case "clear-session":
		state.SessionHistory = nil
		fmt.Printf("%s[✓] Session memory cleared.%s\n", Green, Reset)
	}
}

// Main AI Processing Engine
func processAIQuery(input string) {
	if state.Config.APIKey == "" {
		fmt.Printf("%s[!] API Key missing. Please set NOOTY_API_KEY environment variable or edit ~/.nooty/config.json%s\n", Red, Reset)
		return
	}

	// Prepare Messages
	systemPrompt := "You are Nooty, a senior developer AI assistant."
	if state.Mode == "cli" {
		systemPrompt = fmt.Sprintf("You are NootyCLI Agent. You operate inside workspace: %s. Use tools to read/modify files when asked. Be precise and clean.", state.Workspace)
	}

	// Inject Memories into System Prompt
	if len(state.Memories) > 0 {
		systemPrompt += "\n\nUser Preferences & Memories:"
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

	// Attach Tools in Agent CLI Mode
	if state.Mode == "cli" {
		reqBody.Tools = getAvailableTools()
	}

	fmt.Printf("%s[Nooty thinking...]%s\r", Gray, Reset)
	resp, err := callNootyAPI(reqBody)
	if err != nil {
		fmt.Printf("%s[!] Error connecting to API: %v%s\n", Red, err, Reset)
		return
	}

	if len(resp.Choices) == 0 {
		fmt.Printf("%s[!] Empty response received.%s\n", Red, Reset)
		return
	}

	aiMsg := resp.Choices[0].Message

	// Handle Tool Calls (Agent Execution Loop)
	if len(aiMsg.ToolCalls) > 0 {
		state.SessionHistory = append(state.SessionHistory, Message{Role: "user", Content: input})
		state.SessionHistory = append(state.SessionHistory, aiMsg)

		for _, tool := range aiMsg.ToolCalls {
			executeToolCall(tool)
		}
	} else {
		// Standard Text Output
		fmt.Printf("\n%sNooty:%s %s\n\n", Bold+Green, Reset, aiMsg.Content)
		state.SessionHistory = append(state.SessionHistory, Message{Role: "user", Content: input})
		state.SessionHistory = append(state.SessionHistory, aiMsg)
	}
}

// Available Tools Specification
func getAvailableTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: ToolDefInReq{
				Name:        "list_files",
				Description: "List files and directories inside workspace",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string", "description": "Relative path inside workspace"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ToolDefInReq{
				Name:        "read_file",
				Description: "Read content of a file in workspace",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string", "description": "Relative path to file"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolDefInReq{
				Name:        "write_file",
				Description: "Create or write content to a file",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":    map[string]interface{}{"type": "string", "description": "Relative path to file"},
						"content": map[string]interface{}{"type": "string", "description": "Full file content"},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolDefInReq{
				Name:        "delete_file",
				Description: "Delete a file inside workspace",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string", "description": "Relative path to file"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolDefInReq{
				Name:        "run_command",
				Description: "Execute shell command in workspace",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{"type": "string", "description": "Bash command to execute"},
					},
					"required": []string{"command"},
				},
			},
		},
	}
}

// Tool Call Dispatcher & Approval Logic
func executeToolCall(tool ToolCall) {
	fnName := tool.Function.Name
	var args map[string]string
	_ = json.Unmarshal([]byte(tool.Function.Arguments), &args)

	fmt.Printf("\n%s➔ Agent requested tool:%s %s%s%s\n", Purple, Reset, Bold, fnName, Reset)

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
			typeStr := "[FILE]"
			if e.IsDir() {
				typeStr = "[DIR ]"
			}
			list = append(list, fmt.Sprintf("%s %s", typeStr, e.Name()))
		}
		sendToolResult(tool.ID, strings.Join(list, "\n"))

	case "read_file":
		relPath := args["path"]
		fullPath := filepath.Join(state.Workspace, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			sendToolResult(tool.ID, fmt.Sprintf("Error reading file: %v", err))
			return
		}
		fmt.Printf("   Reading file: %s%s%s (%d bytes)\n", Cyan, relPath, Reset, len(content))
		sendToolResult(tool.ID, string(content))

	case "write_file":
		relPath := args["path"]
		content := args["content"]
		fullPath := filepath.Join(state.Workspace, relPath)

		fmt.Printf("   File Target: %s%s%s\n", Yellow, relPath, Reset)
		if askApproval("Allow creating/modifying this file?") {
			_ = os.MkdirAll(filepath.Dir(fullPath), 0755)
			err := os.WriteFile(fullPath, []byte(content), 0644)
			if err != nil {
				sendToolResult(tool.ID, fmt.Sprintf("Error writing file: %v", err))
			} else {
				fmt.Printf("%s[✓] File written successfully.%s\n", Green, Reset)
				sendToolResult(tool.ID, "File written successfully.")
			}
		} else {
			sendToolResult(tool.ID, "User denied file write approval.")
		}

	case "delete_file":
		relPath := args["path"]
		fullPath := filepath.Join(state.Workspace, relPath)
		fmt.Printf("%s⚠️ High Risk Operation: Delete file %s%s\n", Red, relPath, Reset)
		if askApproval("Are you sure you want to DELETE this file?") {
			err := os.Remove(fullPath)
			if err != nil {
				sendToolResult(tool.ID, fmt.Sprintf("Error deleting: %v", err))
			} else {
				fmt.Printf("%s[✓] File deleted.%s\n", Green, Reset)
				sendToolResult(tool.ID, "File deleted successfully.")
			}
		} else {
			sendToolResult(tool.ID, "User denied deletion.")
		}

	case "run_command":
		cmdStr := args["command"]
		executeShellCommand(cmdStr, false)
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

func executeShellCommand(cmdStr string, manual bool) {
	fmt.Printf("%s➔ Executing command:%s %s%s%s\n", Yellow, Reset, Bold, cmdStr, Reset)
	if !askApproval("Allow running this shell command?") {
		fmt.Println("Command cancelled by user.")
		return
	}

	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = state.Workspace
	out, err := cmd.CombinedOutput()

	fmt.Printf("\n%s--- Command Output ---%s\n%s\n", Gray, Reset, string(out))
	if err != nil {
		fmt.Printf("%s[!] Command exited with error: %v%s\n", Red, err, Reset)
	}
}

// Connection Doctor & Anti-Sanction Smart DNS
func runConnectionDoctor() {
	fmt.Println(Bold + "\n--- Running Connection & Anti-Sanction Doctor ---" + Reset)
	fmt.Printf("1. Testing Provider Endpoint: %s\n", state.Config.BaseURL)

	client := http.Client{Timeout: 5 * time.Second}
	_, err := client.Get(state.Config.BaseURL)
	if err == nil {
		fmt.Printf("%s[✓] Direct connection healthy! (No sanctions/DNS blocks detected)%s\n\n", Green, Reset)
		return
	}

	fmt.Printf("%s[!] Direct connection failed/blocked: %v%s\n", Yellow, err, Reset)
	fmt.Println("2. Testing Smart DNS Anti-Sanction Fallback...")

	// Create custom dialer with Smart DNS (e.g., Shecan)
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
		fmt.Printf("%s[✓] Smart DNS Route (%s) successful! Anti-sanction ready.%s\n\n", Green, state.Config.CustomDNS, Reset)
	} else {
		fmt.Printf("%s[!] Smart DNS check failed. Check internet connection or API Key.%s\n\n", Red, Reset)
	}
}

// HTTP API Calling
func callNootyAPI(reqBody ChatCompletionReq) (*ChatCompletionResp, error) {
	jsonBytes, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(state.Config.BaseURL, "/"))

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
