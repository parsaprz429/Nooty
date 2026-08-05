// nooty.go — Nooty v0.1 Community Edition (single‑file, standard library only)
//
// Usage:
//   go run nooty.go               (or build: go build -o nooty nooty.go && ./nooty)
//   nooty                         starts interactive REPL
//   nooty --help                  show help
//
// Compatible with macOS and Linux.
// No external dependencies. Uses ANSI escape codes for styling.
// Memory stored as JSON files in ~/.nooty/ . Workspace metadata in .nooty/ .
//
// This file will be hosted at:
//   https://github.com/parsaprz429/Nooty/tree/main/NootyCLI/0.1.0/nooty.go

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ---------- ANSI helpers ----------
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	cyan   = "\033[36m"
)

// ---------- Data structures ----------

type Config struct {
	ProviderEndpoint string `json:"provider_endpoint"`
	APIKey           string `json:"api_key"`
	Model            string `json:"model"`
	Safety           string `json:"safety"` // "strict" or "balanced"
	Workspace        string `json:"workspace"`
}

type Memory struct {
	ID      int    `json:"id"`
	Tag     string `json:"tag"` // preference, project, fact, instruction
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
	Name      string
	Args      map[string]string
	Approval  string // "auto", "confirm", "danger"
	Execute   func() (string, error)
}

// ---------- Global state ----------
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
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Nooty v0.1 — Local‑first terminal intelligence\n\nUsage:\n  nooty [options]\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Setup directories
	var err error
	homeDir, err = os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot find home directory")
		os.Exit(1)
	}
	nootyDir = filepath.Join(homeDir, ".nooty")
	os.MkdirAll(nootyDir, 0700)
	os.MkdirAll(filepath.Join(nootyDir, "chats"), 0700)
	configFile = filepath.Join(nootyDir, "config.json")
	memFile = filepath.Join(nootyDir, "memories.json")

	loadConfig()
	loadMemories()

	if config.Workspace == "" {
		cwd, _ := os.Getwd()
		config.Workspace = cwd
	}
	workspace = config.Workspace

	// Print banner
	fmt.Printf("\n%s╭────────────────────────────────────────╮%s\n", cyan, reset)
	fmt.Printf("%s│              Nooty v0.1                │%s\n", cyan, reset)
	fmt.Printf("%s│     Local-first terminal intelligence   │%s\n", cyan, reset)
	fmt.Printf("%s╰────────────────────────────────────────╯%s\n\n", cyan, reset)
	fmt.Printf("Workspace: %s\n", workspace)
	fmt.Printf("Provider:  %s\n", config.ProviderEndpoint)
	fmt.Printf("Model:     %s\n", config.Model)
	fmt.Printf("Mode:      %s\n\n", modeLabel())
	fmt.Println("Type /help for commands.")

	// Start REPL
	repl()
}

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
	fmt.Println("\nGoodbye!")
}

func prompt() string {
	if currentMode == "cli" {
		return fmt.Sprintf("%snooty[cli]%s > ", cyan, reset)
	}
	return fmt.Sprintf("%snooty%s > ", green, reset)
}

func modeLabel() string {
	if currentMode == "cli" {
		return bold + "NootyCLI" + reset
	}
	return bold + "NootyChat" + reset
}

// ---------- Slash commands ----------

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
			fmt.Println(green + "Switched to NootyCLI Agent mode." + reset)
		} else {
			currentMode = "chat"
			fmt.Println(green + "Switched to NootyChat mode." + reset)
		}
	case "/workspace":
		handleWorkspace(parts[1:])
	case "/model":
		handleModel(parts[1:])
	case "/provider":
		handleProviderStatus()
	case "/connection":
		handleConnectionStatus()
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
		fmt.Println("Session cleared.")
	case "/exit":
		os.Exit(0)
	default:
		fmt.Printf("Unknown command: %s\n", parts[0])
	}
}

func printHelp() {
	fmt.Println(`Available commands:
  /help                        Show this help
  /mode [chat|cli]             Switch mode
  /workspace show|set <path>   Manage workspace
  /model show|set <name>       View/set model
  /provider status             Show provider info
  /connection status           Connection diagnostics
  /doctor                      Full connection check
  /memory list|add <text>|forget <id>|clear-session|clear-project|export
  /safety status|strict|balanced
  /history                     Show session history
  /clear                       Clear session memory
  /exit                        Quit

In CLI mode, start a line with ! to request shell execution (with approval).`)
}

func handleWorkspace(args []string) {
	if len(args) == 0 {
		fmt.Printf("Workspace: %s\n", workspace)
		return
	}
	switch args[0] {
	case "show":
		fmt.Printf("Workspace: %s\n", workspace)
	case "set":
		if len(args) < 2 {
			fmt.Println("Usage: /workspace set <path>")
			return
		}
		path := args[1]
		absPath, err := filepath.Abs(path)
		if err != nil {
			fmt.Printf("Invalid path: %v\n", err)
			return
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			fmt.Println("Not a valid directory.")
			return
		}
		workspace = absPath
		config.Workspace = absPath
		saveConfig()
		fmt.Printf("Workspace set to: %s\n", workspace)
	default:
		fmt.Println("Unknown workspace subcommand.")
	}
}

func handleModel(args []string) {
	if len(args) == 0 {
		fmt.Printf("Model: %s\n", config.Model)
		return
	}
	switch args[0] {
	case "show":
		fmt.Printf("Model: %s\n", config.Model)
	case "set":
		if len(args) < 2 {
			fmt.Println("Usage: /model set <model-name>")
			return
		}
		config.Model = args[1]
		saveConfig()
		fmt.Printf("Model set to: %s\n", config.Model)
	default:
		fmt.Println("Unknown model subcommand.")
	}
}

func handleProviderStatus() {
	fmt.Printf("Provider endpoint: %s\n", config.ProviderEndpoint)
	fmt.Printf("Model: %s\n", config.Model)
	keySet := "not set"
	if config.APIKey != "" {
		keySet = "set"
	}
	fmt.Printf("API key: %s\n", keySet)
}

func handleConnectionStatus() {
	fmt.Println("Testing connection to provider...")
	err := checkProviderConnection()
	if err != nil {
		fmt.Printf("%sConnection failed: %v%s\n", red, err, reset)
	} else {
		fmt.Printf("%sConnection OK%s\n", green, reset)
	}
}

func runDoctor() {
	fmt.Println(bold + "Connection Doctor" + reset)
	fmt.Printf("Provider: %s\n", config.ProviderEndpoint)
	fmt.Printf("Model: %s\n", config.Model)
	fmt.Println("1. Direct route test:")
	err := checkProviderConnection()
	if err != nil {
		fmt.Printf("   %sFAILED: %v%s\n", red, err, reset)
		fmt.Println("   (No alternate profiles configured in v0.1; check your network/API key.)")
	} else {
		fmt.Printf("   %sOK%s\n", green, reset)
	}
	fmt.Println("Doctor complete.")
}

func checkProviderConnection() error {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", strings.TrimRight(config.ProviderEndpoint, "/")+"/models", nil)
	if config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func handleMemory(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: /memory list|add|forget|clear-session|clear-project|export")
		return
	}
	switch args[0] {
	case "list":
		if len(memories) == 0 {
			fmt.Println("No memories.")
			return
		}
		for _, m := range memories {
			fmt.Printf("[%d] %s (%s) %s\n", m.ID, m.Tag, m.Added, m.Content)
		}
	case "add":
		if len(args) < 2 {
			fmt.Println("Usage: /memory add <text>")
			return
		}
		text := strings.Join(args[1:], " ")
		tag := "fact"
		if strings.Contains(strings.ToLower(text), "prefer") || strings.Contains(strings.ToLower(text), "i prefer") {
			tag = "preference"
		}
		m := Memory{
			ID:      len(memories) + 1,
			Tag:     tag,
			Content: text,
			Added:   time.Now().Format(time.RFC3339),
		}
		memories = append(memories, m)
		saveMemories()
		fmt.Printf("Memory saved [%d] (%s): %s\n", m.ID, m.Tag, m.Content)
	case "forget":
		if len(args) < 2 {
			fmt.Println("Usage: /memory forget <id>")
			return
		}
		id := 0
		fmt.Sscanf(args[1], "%d", &id)
		newList := []Memory{}
		for _, m := range memories {
			if m.ID != id {
				newList = append(newList, m)
			}
		}
		memories = newList
		saveMemories()
		fmt.Printf("Forgotten memory %d.\n", id)
	case "clear-session":
		sessionMessages = nil
		fmt.Println("Session memory cleared.")
	case "clear-project":
		projDir := filepath.Join(workspace, ".nooty")
		os.RemoveAll(projDir)
		fmt.Println("Project memories cleared.")
	case "export":
		data, _ := json.MarshalIndent(memories, "", "  ")
		fmt.Println(string(data))
	default:
		fmt.Println("Unknown memory subcommand.")
	}
}

func handleSafety(args []string) {
	if len(args) == 0 {
		fmt.Printf("Safety policy: %s\n", config.Safety)
		return
	}
	switch args[0] {
	case "status":
		fmt.Printf("Safety policy: %s\n", config.Safety)
	case "strict", "balanced":
		config.Safety = args[0]
		saveConfig()
		fmt.Printf("Safety policy set to %s.\n", config.Safety)
	default:
		fmt.Println("Usage: /safety [strict|balanced|status]")
	}
}

func handleShellBang(cmd string) {
	cmd = strings.TrimSpace(cmd)
	fmt.Printf("\n%s→ Requested shell command:%s\n%s\n\n", yellow, reset, cmd)
	fmt.Print("Allow execution? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	if response == "y" || response == "yes" {
		executeShell(cmd, true)
	} else {
		fmt.Println("Cancelled.")
	}
}

func executeShell(command string, showOutput bool) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = workspace
	cmd.Env = os.Environ()
	if showOutput {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	err := cmd.Run()
	if err != nil {
		fmt.Printf("%sCommand failed: %v%s\n", red, err, reset)
	}
}

func showHistory() {
	if len(sessionMessages) == 0 {
		fmt.Println("No messages in session.")
		return
	}
	for _, msg := range sessionMessages {
		role := "User"
		if msg.Role == "assistant" {
			role = "Nooty"
		}
		fmt.Printf("%s%s:%s %s\n", bold, role, reset, msg.Content)
	}
}

// ---------- Chat & Agent logic ----------

func handleChat(input string) {
	// Build context with system prompt and memories
	messages := buildMessages(input)
	// If in CLI mode, we run the agent loop that may call tools
	if currentMode == "cli" {
		runAgentLoop(messages)
		return
	}
	// Chat mode: stream response
	streamResponse(messages)
}

func buildMessages(userInput string) []Message {
	var msgs []Message
	// System prompt
	sysPrompt := "You are Nooty, a helpful terminal assistant. "
	if currentMode == "cli" {
		sysPrompt += "You have access to tools for reading/writing files and running commands. Describe what you will do and ask for confirmation when required. Keep answers concise."
	} else {
		sysPrompt += "You are in chat mode. Answer questions, explain concepts, and use the local memory to personalise responses."
	}
	// Append relevant memories
	relevant := getRelevantMemories(userInput)
	if len(relevant) > 0 {
		sysPrompt += "\n\nUser memories:\n"
		for _, m := range relevant {
			sysPrompt += fmt.Sprintf("- [%s] %s\n", m.Tag, m.Content)
		}
	}
	msgs = append(msgs, Message{Role: "system", Content: sysPrompt})
	// Append session history (last N messages to keep context short)
	historyLimit := 10
	start := 0
	if len(sessionMessages) > historyLimit {
		start = len(sessionMessages) - historyLimit
	}
	msgs = append(msgs, sessionMessages[start:]...)
	// Append new user message
	userMsg := Message{Role: "user", Content: userInput}
	msgs = append(msgs, userMsg)
	sessionMessages = append(sessionMessages, userMsg)
	return msgs
}

func getRelevantMemories(query string) []Memory {
	queryLower := strings.ToLower(query)
	var res []Memory
	for _, m := range memories {
		if strings.Contains(strings.ToLower(m.Content), queryLower) || strings.Contains(strings.ToLower(m.Tag), queryLower) {
			res = append(res, m)
		}
	}
	// Return at most 5
	if len(res) > 5 {
		res = res[:5]
	}
	return res
}

func streamResponse(messages []Message) {
	reqPayload := ChatRequest{
		Model:    config.Model,
		Messages: messages,
		Stream:   true,
	}
	jsonData, err := json.Marshal(reqPayload)
	if err != nil {
		fmt.Printf("Error marshalling request: %v\n", err)
		return
	}
	httpReq, err := http.NewRequest("POST", strings.TrimRight(config.ProviderEndpoint, "/")+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+config.APIKey)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		fmt.Printf("%sProvider request failed: %v%s\n", red, err, reset)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("%sProvider error %d: %s%s\n", red, resp.StatusCode, string(body), reset)
		return
	}
	// Read SSE stream
	reader := bufio.NewReader(resp.Body)
	var fullContent strings.Builder
	fmt.Print(cyan)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
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
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			fmt.Print(choice.Delta.Content)
			fullContent.WriteString(choice.Delta.Content)
		}
	}
	fmt.Print(reset + "\n")
	// Append assistant message to session
	assistantMsg := Message{Role: "assistant", Content: fullContent.String()}
	sessionMessages = append(sessionMessages, assistantMsg)
}

// ---------- Agent tool loop (NootyCLI) ----------

func runAgentLoop(messages []Message) {
	// Simplified agent: we send the messages to the model, then parse if it wants tools.
	// For v0.1, we will implement a basic tool-calling by asking the model to reply with
	// a JSON tool call if it wants. But since standard library and no structured output,
	// we can use a simple text parsing: if model output contains a tool marker like
	// "TOOL: read_file path=..." we parse it. To keep the code simple, we'll use a prompt
	// that instructs the model to output tool commands in a parseable format.
	// This is a prototype; a full implementation would require function calling support.
	// For now we demonstrate the concept.

	// Build a system prompt that asks the model to respond with tool actions when needed.
	agentSys := `You are NootyCLI, an agent with access to the workspace.
You can use the following tools by replying with a line exactly like:
TOOL: <tool_name> key1=value1 key2=value2 ...
Available tools:
- list_files (path=relative)
- read_file (path=relative)
- search_code (query=search term, path=relative scope)
- file_info (path=relative)
- git_status
- git_diff
- create_file (path=relative, content=...)
- write_file (path=relative, content=...)
- create_directory (path=relative)
- delete_file (path=relative) [requires confirmation]
- delete_directory (path=relative) [requires confirmation]
- run_command (command=..., timeout=seconds) [requires confirmation]

If you want to perform an action, output exactly one TOOL: line. The user will see it and approve if necessary. After receiving tool result, continue.
If you have a final answer, just output plain text.
Always work within the workspace: ` + workspace + `
Current memory context and conversation provided.`

	// Prepend agent system message
	msgs := []Message{{Role: "system", Content: agentSys}}
	msgs = append(msgs, messages[1:]...) // skip original sys prompt

	// Loop until model gives a final answer without tool calls.
	for i := 0; i < 5; i++ { // max 5 tool calls to avoid infinite loops
		respText, err := getModelResponseText(msgs)
		if err != nil {
			fmt.Printf("%sError: %v%s\n", red, err, reset)
			return
		}
		// Check for TOOL: line
		toolCall := extractToolCall(respText)
		if toolCall == nil {
			// Final answer
			fmt.Print(cyan + respText + reset + "\n")
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText})
			return
		}
		// We have a tool call; display and ask approval
		toolResult, approved := executeAgentTool(toolCall)
		if !approved {
			fmt.Println("Action cancelled by user.")
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText + "\n[User cancelled tool execution]"})
			return
		}
		// Show result (short)
		fmt.Printf("\n%s→ Tool result:%s\n%s\n", yellow, reset, toolResult)
		// Append tool call and result as messages
		msgs = append(msgs, Message{Role: "assistant", Content: respText})
		msgs = append(msgs, Message{Role: "user", Content: "Tool result: " + toolResult})
	}
	fmt.Println("(Agent loop limit reached)")
}

func getModelResponseText(messages []Message) (string, error) {
	reqPayload := ChatRequest{
		Model:    config.Model,
		Messages: messages,
		Stream:   false,
	}
	jsonData, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequest("POST", strings.TrimRight(config.ProviderEndpoint, "/")+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+config.APIKey)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
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
		return "", fmt.Errorf("no response choices")
	}
	return result.Choices[0].Message.Content, nil
}

func extractToolCall(text string) *ToolCall {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TOOL:") {
			parts := strings.SplitN(line[5:], " ", 2)
			if len(parts) < 1 {
				continue
			}
			name := strings.TrimSpace(parts[0])
			args := map[string]string{}
			if len(parts) > 1 {
				// parse key=value pairs (allow values with spaces if quoted? simple for now)
				re := regexp.MustCompile(`(\w+)=("[^"]*"|\S+)`)
				matches := re.FindAllStringSubmatch(parts[1], -1)
				for _, m := range matches {
					if len(m) == 3 {
						val := m[2]
						if strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`) {
							val = val[1 : len(val)-1]
						}
						args[m[1]] = val
					}
				}
			}
			return &ToolCall{Name: name, Args: args}
		}
	}
	return nil
}

func executeAgentTool(tc *ToolCall) (string, bool) {
	// Determine if approval needed
	needsApproval := true
	switch tc.Name {
	case "list_files", "read_file", "search_code", "file_info", "git_status", "git_diff":
		needsApproval = false // safe reads
	}
	if needsApproval {
		fmt.Printf("\n%s→ NootyCLI wants to execute tool: %s%s\n", yellow, tc.Name, reset)
		for k, v := range tc.Args {
			fmt.Printf("  %s: %s\n", k, v)
		}
		if tc.Name == "delete_file" || tc.Name == "delete_directory" {
			fmt.Printf("%s⚠ Sensitive operation. Type DELETE to confirm:%s ", red, reset)
			reader := bufio.NewReader(os.Stdin)
			confirm, _ := reader.ReadString('\n')
			if strings.TrimSpace(confirm) != "DELETE" {
				return "", false
			}
		} else {
			fmt.Print("Allow? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(strings.ToLower(confirm))
			if confirm != "y" && confirm != "yes" {
				return "", false
			}
		}
	}
	// Execute tool
	result, err := runTool(tc.Name, tc.Args)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}
	return result, true
}

func runTool(name string, args map[string]string) (string, error) {
	switch name {
	case "list_files":
		path := workspace
		if p, ok := args["path"]; ok && p != "" {
			path = safeJoin(workspace, p)
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return "", err
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return strings.Join(names, "\n"), nil
	case "read_file":
		path := safeJoin(workspace, args["path"])
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	case "search_code":
		query := args["query"]
		scope := workspace
		if s, ok := args["path"]; ok && s != "" {
			scope = safeJoin(workspace, s)
		}
		var results []string
		filepath.Walk(scope, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			// skip binaries, large files
			if info.Size() > 1_000_000 {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			if strings.Contains(string(data), query) {
				rel, _ := filepath.Rel(workspace, p)
				results = append(results, rel)
			}
			return nil
		})
		return strings.Join(results, "\n"), nil
	case "file_info":
		path := safeJoin(workspace, args["path"])
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Size: %d bytes, Mode: %s, ModTime: %s", info.Size(), info.Mode(), info.ModTime()), nil
	case "git_status":
		cmd := exec.Command("git", "status", "--short")
		cmd.Dir = workspace
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git status failed: %v", err)
		}
		return string(out), nil
	case "git_diff":
		cmd := exec.Command("git", "diff")
		cmd.Dir = workspace
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git diff failed: %v", err)
		}
		return string(out), nil
	case "create_file":
		path := safeJoin(workspace, args["path"])
		content := args["content"]
		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			return "", err
		}
		return "File created: " + path, nil
	case "write_file":
		path := safeJoin(workspace, args["path"])
		content := args["content"]
		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			return "", err
		}
		return "File written: " + path, nil
	case "create_directory":
		path := safeJoin(workspace, args["path"])
		err := os.MkdirAll(path, 0755)
		if err != nil {
			return "", err
		}
		return "Directory created: " + path, nil
	case "delete_file":
		path := safeJoin(workspace, args["path"])
		err := os.Remove(path)
		if err != nil {
			return "", err
		}
		return "Deleted: " + path, nil
	case "delete_directory":
		path := safeJoin(workspace, args["path"])
		err := os.RemoveAll(path)
		if err != nil {
			return "", err
		}
		return "Deleted directory: " + path, nil
	case "run_command":
		command := args["command"]
		timeout := 60
		if t, ok := args["timeout"]; ok {
			fmt.Sscanf(t, "%d", &timeout)
		}
		cmd := exec.Command("bash", "-c", command)
		cmd.Dir = workspace
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		err := cmd.Start()
		if err != nil {
			return "", err
		}
		done := make(chan error)
		go func() { done <- cmd.Wait() }()
		select {
		case err = <-done:
			result := outBuf.String()
			if errBuf.Len() > 0 {
				result += "\n[stderr]\n" + errBuf.String()
			}
			if err != nil {
				return result + fmt.Sprintf("\nExit error: %v", err), nil
			}
			return result, nil
		case <-time.After(time.Duration(timeout) * time.Second):
			cmd.Process.Kill()
			return "", fmt.Errorf("command timed out")
		}
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func safeJoin(base, rel string) string {
	// Ensure the resulting path stays inside workspace
	abs := filepath.Join(base, rel)
	abs = filepath.Clean(abs)
	if !strings.HasPrefix(abs, base) {
		// fallback to base
		return base
	}
	return abs
}

// ---------- Config & memory persistence ----------

func loadConfig() {
	data, err := os.ReadFile(configFile)
	if err != nil {
		// defaults
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
	json.Unmarshal(data, &config)
	// env overrides for API key if not set
	if config.APIKey == "" {
		config.APIKey = os.Getenv("OPENAI_API_KEY")
		if config.APIKey == "" {
			config.APIKey = os.Getenv("NOOTY_API_KEY")
		}
	}
}

func saveConfig() {
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configFile, data, 0600)
}

func loadMemories() {
	data, err := os.ReadFile(memFile)
	if err != nil {
		memories = []Memory{}
		return
	}
	json.Unmarshal(data, &memories)
}

func saveMemories() {
	data, _ := json.MarshalIndent(memories, "", "  ")
	os.WriteFile(memFile, data, 0600)
}
