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

// Minimal Modern Palette
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
	Cyan    = "\033[36m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Red     = "\033[31m"
	Gray    = "\033[90m"
	Magenta = "\033[35m"
)

type Config struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	Safety    string `json:"safety"`
	ActiveDNS string `json:"active_dns"`
}

type Memory struct {
	ID        int       `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

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

type AppState struct {
	Mode           string
	Workspace      string
	Config         Config
	Memories       []Memory
	SessionHistory []Message
	HomeDir        string
	NootyDir       string
}

var state AppState

var dnsPool = []struct {
	Name string
	IP   string
}{
	{"Electro", "10.202.10.202"},
	{"Shecan", "178.22.122.100"},
	{"Begzar", "185.55.226.26"},
}

var presetModels = []struct {
	Name string
	ID   string
}{
	{"DeepSeek V4 Flash (Fast)", "deepseek-v4-flash"},
	{"Qwen 2.5 Coder 32B", "qwen/qwen-2.5-coder-32b-instruct"},
	{"Claude 3.5 Sonnet", "anthropic/claude-3.5-sonnet"},
	{"GPT-4o Mini", "openai/gpt-4o-mini"},
}

func main() {
	initAppState()
	renderMinimalHeader()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		promptPrefix := fmt.Sprintf("%s%snooty%s %s[%s]%s ❯ ", Bold, Cyan, Reset, Gray, state.Mode, Reset)
		fmt.Print(promptPrefix)

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			handleCommand(input, scanner)
			continue
		}

		if strings.HasPrefix(input, "!") {
			if state.Mode != "cli" {
				fmt.Printf("%s[!] Raw shell execution requires 'cli' mode.%s\n", Yellow, Reset)
				continue
			}
			cmdStr := strings.TrimPrefix(input, "!")
			executeShellCommand(cmdStr)
			continue
		}

		processAIQueryAutonomous(input, scanner)
	}
}

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
			Model:     "deepseek-v4-flash",
			Safety:    "strict",
			ActiveDNS: "Direct",
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
		state.Config.Model = "deepseek-v4-flash"
	}
	if envKey := os.Getenv("NOOTY_API_KEY"); envKey != "" {
		state.Config.APIKey = envKey
	}

	loadMemories()
}

func maskAPIKey(key string) string {
	if key == "" {
		return fmt.Sprintf("%s[Not Set]%s", Red, Reset)
	}
	if len(key) <= 12 {
		return "••••••••••••"
	}
	return key[:6] + "••••" + key[len(key)-4:]
}

// Clean Minimal Header
func renderMinimalHeader() {
	fmt.Print("\033[H\033[2J") // Clear Screen

	fmt.Printf("%s%s⚡ NOOTY CLI %sv0.2.0%s\n", Bold, Cyan, Gray, Reset)
	fmt.Printf("%sModel:%s %s  %sKey:%s %s  %sDir:%s %s\n",
		Gray, Reset, state.Config.Model,
		Gray, Reset, maskAPIKey(state.Config.APIKey),
		Gray, Reset, truncatePath(state.Workspace, 25),
	)
	fmt.Printf("%sCommands:%s %s/config%s • %s/model%s • %s/mode%s • %s/help%s\n",
		Gray, Reset, Bold, Reset, Bold, Reset, Bold, Reset, Bold, Reset)
	fmt.Printf("%s%s%s\n\n", Gray, strings.Repeat("─", 55), Reset)
}

func truncatePath(p string, max int) string {
	if len(p) <= max {
		return p
	}
	return "..." + p[len(p)-max+3:]
}

func handleCommand(input string, scanner *bufio.Scanner) {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/help":
		fmt.Println(Bold + "\n── Commands ──" + Reset)
		fmt.Println("  /config          - Configure API Key & Endpoints")
		fmt.Println("  /model           - Switch AI Models")
		fmt.Println("  /mode chat|cli   - Toggle Modes")
		fmt.Println("  /doctor          - Test Connectivity & DNS")
		fmt.Println("  /clear           - Clear Screen")
		fmt.Println("  /exit            - Quit\n")

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
				renderMinimalHeader()
			}
		} else {
			fmt.Printf("Current mode: %s%s%s\n", Green, state.Mode, Reset)
		}

	case "/clear":
		renderMinimalHeader()

	case "/doctor":
		runConnectionDoctor()

	case "/exit":
		fmt.Println("Bye! 👋")
		os.Exit(0)

	default:
		fmt.Printf("%s[!] Unknown command. Type /help%s\n", Red, Reset)
	}
}

func configureSystemInteractive(scanner *bufio.Scanner) {
	fmt.Println(Bold + "\n⚙️ Settings" + Reset)
	fmt.Printf("Current Key: %s\n", maskAPIKey(state.Config.APIKey))
	fmt.Print(Yellow + "Enter new API Key (press Enter to skip): " + Reset)
	if scanner.Scan() {
		newKey := strings.TrimSpace(scanner.Text())
		if newKey != "" {
			state.Config.APIKey = newKey
		}
	}
	saveConfig()
	renderMinimalHeader()
	fmt.Println(Green + "✓ Configuration saved!\n" + Reset)
}

func selectModelInteractive(scanner *bufio.Scanner) {
	fmt.Println(Bold + "\n🤖 Select Model" + Reset)
	for idx, m := range presetModels {
		fmt.Printf("  [%d] %-25s (%s)\n", idx+1, m.Name, m.ID)
	}
	fmt.Print(Yellow + "Choice: " + Reset)
	if scanner.Scan() {
		choiceStr := strings.TrimSpace(scanner.Text())
		choice, err := strconv.Atoi(choiceStr)
		if err == nil && choice >= 1 && choice <= len(presetModels) {
			state.Config.Model = presetModels[choice-1].ID
			saveConfig()
			renderMinimalHeader()
			fmt.Printf("%s✓ Switched to %s%s\n\n", Green, state.Config.Model, Reset)
		}
	}
}

func processAIQueryAutonomous(input string, scanner *bufio.Scanner) {
	if state.Config.APIKey == "" {
		fmt.Printf("%s[!] Missing API Key. Use /config to set it.%s\n", Red, Reset)
		return
	}

	systemPrompt := "You are Nooty, a smart developer assistant."
	if state.Mode == "cli" {
		systemPrompt = fmt.Sprintf("You are NootyCLI autonomous agent operating in workspace: %s. Use available tools to complete tasks.", state.Workspace)
	}

	messages := []Message{{Role: "system", Content: systemPrompt}}
	messages = append(messages, state.SessionHistory...)
	messages = append(messages, Message{Role: "user", Content: input})

	for loopCount := 0; loopCount < 8; loopCount++ {
		reqBody := ChatCompletionReq{
			Model:    state.Config.Model,
			Messages: messages,
		}

		if state.Mode == "cli" {
			reqBody.Tools = getAvailableTools()
		}

		fmt.Printf("%s⏳ Thinking...%s\r", Gray, Reset)
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
			return
		}

		aiMsg := resp.Choices[0].Message
		messages = append(messages, aiMsg)

		if len(aiMsg.ToolCalls) > 0 {
			for _, tool := range aiMsg.ToolCalls {
				result := executeToolCall(tool, scanner)
				messages = append(messages, Message{
					Role:       "tool",
					ToolCallID: tool.ID,
					Content:    result,
				})
			}
			continue
		}

		if aiMsg.Content != "" {
			fmt.Printf("\n%sNooty:%s %s\n\n", Bold+Cyan, Reset, aiMsg.Content)
		}

		state.SessionHistory = append(state.SessionHistory, Message{Role: "user", Content: input}, aiMsg)
		break
	}
}

func getAvailableTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: ToolDefInReq{
				Name:        "run_command",
				Description: "Run terminal bash command",
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

func executeToolCall(tool ToolCall, scanner *bufio.Scanner) string {
	var args map[string]string
	_ = json.Unmarshal([]byte(tool.Function.Arguments), &args)

	if tool.Function.Name == "run_command" {
		cmdStr := args["command"]
		fmt.Printf("%s➔ Execute:%s %s\n", Yellow, Reset, cmdStr)
		fmt.Print(Gray + "Allow execution? [y/N]: " + Reset)
		if scanner.Scan() && strings.ToLower(strings.TrimSpace(scanner.Text())) == "y" {
			cmd := exec.Command("bash", "-c", cmdStr)
			cmd.Dir = state.Workspace
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v. Output: %s", err, string(out))
			}
			return string(out)
		}
		return "User denied execution."
	}
	return "Tool executed."
}

func executeShellCommand(cmdStr string) {
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = state.Workspace
	out, _ := cmd.CombinedOutput()
	fmt.Printf("%s\n", string(out))
}

func runConnectionDoctor() {
	fmt.Println(Bold + "\n🔍 Checking Connection..." + Reset)
	client := http.Client{Timeout: 3 * time.Second}
	_, err := client.Get(state.Config.BaseURL)
	if err == nil {
		fmt.Println(Green + "✓ Direct Connection Active!" + Reset + "\n")
		return
	}
	fmt.Println(Yellow + "⚠️ Direct connection blocked. Checking DNS pool..." + Reset)
}

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
