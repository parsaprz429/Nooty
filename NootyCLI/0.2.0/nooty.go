// nooty.go — Nooty v0.2.0 "Radin" – Minimal, Agentic, Open‑Source
// Single‑file, zero dependencies, macOS & Linux.
//
// 🚀 One‑line install:
//   cd ~ && curl -O https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/0.2.0/nooty.go && go run nooty.go
//
// 🌍 Anti‑sanction DNS fallback:
//   Shecan (178.22.122.100, 185.51.200.2) & Begzar (185.55.226.26, …)
//
//   /config       → interactive setup (API key, provider, model)
//   /model list   → browse & select models
//   /dns          → show active DNS resolvers

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
	"strconv"
	"strings"
	"time"
)

// ---------- ANSI styling ----------
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

const banner = `
███╗   ██╗ ██████╗  ██████╗ ████████╗██╗   ██╗
████╗  ██║██╔═══██╗██╔═══██╗╚══██╔══╝╚██╗ ██╔╝
██╔██╗ ██║██║   ██║██║   ██║   ██║    ╚████╔╝ 
██║╚██╗██║██║   ██║██║   ██║   ██║     ╚██╔╝  
██║ ╚████║╚██████╔╝╚██████╔╝   ██║      ██║   
╚═╝  ╚═══╝ ╚═════╝  ╚═════╝    ╚═╝      ╚═╝   
                                               
███╗   ██╗ ██████╗  ██████╗ ████████╗██╗   ██╗
████╗  ██║██╔═══██╗██╔═══██╗╚══██╔══╝╚██╗ ██╔╝
██╔██╗ ██║██║   ██║██║   ██║   ██║    ╚████╔╝ 
██║╚██╗██║██║   ██║██║   ██║   ██║     ╚██╔╝  
██║ ╚████║╚██████╔╝╚██████╔╝   ██║      ██║   
╚═╝  ╚═══╝ ╚═════╝  ╚═════╝    ╚═╝      ╚═╝   
===============================================
`

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

	// DNS fallback resolvers
	fallbackDNS = []string{
		"",                    // system default
		"185.51.200.2",        // Shecan
		"178.22.122.100",      // Shecan
		"185.55.226.26",       // Begzar
		"185.55.225.25",       // Begzar
		"185.55.224.24",       // Begzar
	}
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Nooty v0.2.0 — Local‑first terminal intelligence\n\nUsage:\n  nooty [options]\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

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

	// Beautiful header
	fmt.Print(cyan + banner + reset)
	fmt.Printf("%sNootyCLI v0.2.0 — Radin%s\n\n", bold, reset)
	printHeaderInfo()
	fmt.Println("\nType /help for commands.\n")

	repl()
}

func printHeaderInfo() {
	maskedKey := maskAPIKey(config.APIKey)
	fmt.Printf("  %sProvider:%s  %s\n", dim, reset, config.ProviderEndpoint)
	fmt.Printf("  %sModel:    %s %s%s%s\n", dim, reset, bold, config.Model, reset)
	fmt.Printf("  %sAPI Key:  %s %s\n", dim, reset, maskedKey)
	fmt.Printf("  %sSafety:   %s %s\n", dim, reset, config.Safety)
	fmt.Printf("  %sWorkspace:%s %s\n", dim, reset, workspace)
}

func maskAPIKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 8 {
		return key[:1] + strings.Repeat("*", len(key)-1)
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// ---------- Interactive REPL ----------
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
		handleModelCommand(parts[1:])
	case "/config":
		handleConfig()
	case "/dns":
		showDNSStatus()
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
  /config                      Interactive setup (API key, provider, model)
  /workspace show|set <path>   Manage workspace
  /model show|set <name>|list  View/set/browse models
  /provider status             Show provider info
  /dns                         Show active DNS resolvers
  /connection status           Connection diagnostics
  /doctor                      Full connection check
  /memory list|add|forget|...
  /safety status|strict|balanced
  /history                     Show session history
  /clear                       Clear session memory
  /exit                        Quit

In CLI mode, start a line with ! to request shell execution (with approval).`)
}

func handleConfig() {
	fmt.Println("Nooty Configuration (leave empty to keep current value)")
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
	fmt.Println("✓ Configuration saved.")
}

func showDNSStatus() {
	fmt.Println("DNS fallback chain (tried in order):")
	for i, dns := range fallbackDNS {
		if dns == "" {
			fmt.Printf("  %d. System default\n", i+1)
		} else {
			fmt.Printf("  %d. %s\n", i+1, dns)
		}
	}
}

func handleModelCommand(args []string) {
	if len(args) == 0 {
		fmt.Printf("Current model: %s\n", config.Model)
		fmt.Println("Use /model list to browse available models, or /model set <name> to switch.")
		return
	}
	switch args[0] {
	case "show":
		fmt.Printf("Current model: %s\n", config.Model)
	case "set":
		if len(args) < 2 {
			fmt.Println("Usage: /model set <model-name>")
			return
		}
		config.Model = args[1]
		saveConfig()
		fmt.Printf("Model set to: %s\n", config.Model)
	case "list":
		selectModelInteractive()
	default:
		fmt.Println("Unknown model subcommand. Use show, set, or list.")
	}
}

func selectModelInteractive() {
	fmt.Println("Fetching available models (using DNS fallback if needed)...")
	models, err := fetchAvailableModels()
	if err != nil {
		fmt.Printf("%sFailed to fetch models: %v%s\n", red, err, reset)
		return
	}
	if len(models) == 0 {
		fmt.Println("No models returned.")
		return
	}
	fmt.Printf("\n%sAvailable models:%s\n", bold, reset)
	pageSize := 20
	totalPages := (len(models) + pageSize - 1) / pageSize
	page := 0
	for {
		start := page * pageSize
		end := start + pageSize
		if end > len(models) {
			end = len(models)
		}
		for i, m := range models[start:end] {
			fmt.Printf("  %s[%d]%s %s\n", bold, start+i+1, reset, m)
		}
		fmt.Printf("\nPage %d/%d. Enter number to select, 'n' next, 'p' prev, 'q' quit: ", page+1, totalPages)
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
				fmt.Println("Invalid selection.")
				continue
			}
			selected := models[num-1]
			config.Model = selected
			saveConfig()
			fmt.Printf("%sModel set to: %s%s\n", green, selected, reset)
			return
		}
	}
}

// ---------- Network helpers with DNS fallback ----------
func dnsDialer(dnsServer string) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{}
		return d.DialContext(ctx, network, dnsServer+":53")
	}
}

func httpClientForDNS(dns string) *http.Client {
	if dns == "" {
		return &http.Client{Timeout: 30 * time.Second} // system default
	}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial:     dnsDialer(dns),
	}
	dialer := &net.Dialer{Resolver: resolver}
	return &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext},
		Timeout:   30 * time.Second,
	}
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "EOF")
}

func doWithFallback(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	var lastErr error
	for _, dns := range fallbackDNS {
		client := httpClientForDNS(dns)
		var req *http.Request
		var err error
		if body != nil {
			req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
		} else {
			req, err = http.NewRequest(method, url, nil)
		}
		if err != nil {
			lastErr = err
			continue
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if isRetryableError(err) {
				continue
			}
			return nil, err
		}
		// If status indicates possible sanction, try next DNS
		if resp.StatusCode == 403 || resp.StatusCode == 451 {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d (possible sanction)", resp.StatusCode)
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

func fetchAvailableModels() ([]string, error) {
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/models"
	headers := map[string]string{}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer "+config.APIKey
	}
	resp, err := doWithFallback("GET", endpoint, nil, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	// OpenAI-compatible: {"data":[{"id":"model1"},...]}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		// alternative: {"models":[{"name":"..."}]}
		var alt struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err2 := json.Unmarshal(body, &alt); err2 == nil && len(alt.Models) > 0 {
			var models []string
			for _, m := range alt.Models {
				models = append(models, m.Name)
			}
			return models, nil
		}
		return nil, fmt.Errorf("cannot parse models response: %v", err)
	}
	var models []string
	for _, d := range result.Data {
		models = append(models, d.ID)
	}
	return models, nil
}

func checkProviderConnection() error {
	_, err := fetchAvailableModels() // reuse, just check reachability
	return err
}

// ---------- Chat & agent ----------
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
	sysPrompt := `You are Nooty, a helpful terminal assistant.

When in chat mode: Answer questions conversationally. Use memories when relevant.

When in CLI mode: You have access to tools. To use a tool, you MUST respond with ONLY:
TOOL: tool_name key1=value1 key2=value2

Available tools:
- list_files (path=relative_path or "." for current)
- read_file (path=relative_path)
- search_code (query=text, path=relative_path)
- file_info (path=relative_path)
- git_status
- git_diff
- create_file (path=relative_path, content="file content here")
- write_file (path=relative_path, content="file content here")
- create_directory (path=relative_path)
- delete_file (path=relative_path)
- delete_directory (path=relative_path)
- run_command (command="shell command", timeout=seconds)

IMPORTANT: Use EXACT format TOOL: name key=value. Wait for result before next step.`

	if currentMode == "cli" {
		sysPrompt += "\nYou are in CLI mode. Execute tools step by step."
	}

	relevant := getRelevantMemories(userInput)
	if len(relevant) > 0 {
		sysPrompt += "\n\nUser memories:\n"
		for _, m := range relevant {
			sysPrompt += fmt.Sprintf("- [%s] %s\n", m.Tag, m.Content)
		}
	}
	msgs = append(msgs, Message{Role: "system", Content: sysPrompt})

	historyLimit := 10
	start := 0
	if len(sessionMessages) > historyLimit {
		start = len(sessionMessages) - historyLimit
	}
	msgs = append(msgs, sessionMessages[start:]...)

	userMsg := Message{Role: "user", Content: userInput}
	msgs = append(msgs, userMsg)
	sessionMessages = append(sessionMessages, userMsg)
	return msgs
}

func getRelevantMemories(query string) []Memory {
	queryLower := strings.ToLower(query)
	var res []Memory
	for _, m := range memories {
		if strings.Contains(strings.ToLower(m.Content), queryLower) ||
			strings.Contains(strings.ToLower(m.Tag), queryLower) {
			res = append(res, m)
		}
	}
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
	jsonData, _ := json.Marshal(reqPayload)

	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer "+config.APIKey
	}

	resp, err := doWithFallback("POST", endpoint, jsonData, headers)
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

	reader := bufio.NewReader(resp.Body)
	var fullContent strings.Builder
	fmt.Print(cyan)
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
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			fmt.Print(choice.Delta.Content)
			fullContent.WriteString(choice.Delta.Content)
		}
	}
	fmt.Print(reset + "\n")
	sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: fullContent.String()})
}

// ---------- Agent loop ----------
func runAgentLoop(messages []Message) {
	msgs := messages
	for i := 0; i < 10; i++ {
		respText, err := getModelResponseText(msgs)
		if err != nil {
			fmt.Printf("%sError: %v%s\n", red, err, reset)
			return
		}
		toolCall := extractToolCall(respText)
		if toolCall == nil {
			fmt.Print(cyan + respText + reset + "\n")
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText})
			return
		}
		fmt.Printf("\n%s→ Tool: %s%s\n", yellow, toolCall.Name, reset)
		for k, v := range toolCall.Args {
			fmt.Printf("  %s: %s\n", k, v)
		}
		toolResult, approved := executeAgentTool(toolCall)
		if !approved {
			msgs = append(msgs, Message{Role: "assistant", Content: respText})
			msgs = append(msgs, Message{Role: "user", Content: "User cancelled."})
			continue
		}
		if len(toolResult) > 2000 {
			toolResult = toolResult[:2000] + "\n... (truncated)"
		}
		fmt.Printf("\n%s→ Result:%s\n%s\n", dim, reset, toolResult)
		msgs = append(msgs, Message{Role: "assistant", Content: respText})
		msgs = append(msgs, Message{Role: "user", Content: fmt.Sprintf("Tool '%s' result:\n%s", toolCall.Name, toolResult)})
	}
	fmt.Printf("%s⚠ Agent iteration limit reached.%s\n", yellow, reset)
}

func getModelResponseText(messages []Message) (string, error) {
	reqPayload := ChatRequest{
		Model:    config.Model,
		Messages: messages,
		Stream:   false,
	}
	jsonData, _ := json.Marshal(reqPayload)

	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
	headers := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer "+config.APIKey
	}

	resp, err := doWithFallback("POST", endpoint, jsonData, headers)
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
		if strings.HasPrefix(line, "TOOL:") || strings.HasPrefix(line, "TOOL：") {
			return parseToolLine(line)
		}
	}
	re := regexp.MustCompile(`(?i)(?:```)?\s*TOOL:\s*(\w+)\s+(.*?)(?:```)?$`)
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
			value := match[2]
			if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
				(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
				value = value[1 : len(value)-1]
			}
			value = strings.ReplaceAll(value, `\n`, "\n")
			value = strings.ReplaceAll(value, `\t`, "\t")
			args[key] = value
		}
	}
	if len(args) == 0 && strings.TrimSpace(argsStr) != "" {
		switch name {
		case "list_files":
			args["path"] = strings.TrimSpace(argsStr)
			if args["path"] == "" {
				args["path"] = "."
			}
		default:
			parts := strings.Fields(argsStr)
			if len(parts) >= 1 {
				args["arg1"] = parts[0]
			}
		}
	}
	return &ToolCall{Name: name, Args: args}
}

type ToolCall struct {
	Name string
	Args map[string]string
}

func executeAgentTool(tc *ToolCall) (string, bool) {
	needsApproval := true
	switch tc.Name {
	case "list_files", "read_file", "search_code", "file_info", "git_status", "git_diff":
		needsApproval = false
	}
	if needsApproval {
		if tc.Name == "delete_file" || tc.Name == "delete_directory" {
			fmt.Printf("%s⚠ DANGEROUS: %s will permanently delete files!%s\n", red, tc.Name, reset)
			fmt.Print("Type DELETE to confirm: ")
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
			return "(empty directory)", nil
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
			if err != nil || info.IsDir() || info.Size() > 1_000_000 || strings.HasPrefix(info.Name(), ".") {
				return nil
			}
			data, _ := os.ReadFile(p)
			if strings.Contains(string(data), query) {
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
		return fmt.Sprintf("Size: %d\nMode: %s\nModTime: %s", info.Size(), info.Mode(), info.ModTime()), nil
	case "git_status":
		cmd := exec.Command("git", "status", "--short")
		cmd.Dir = workspace
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git status failed: %v", err)
		}
		s := string(out)
		if s == "" {
			s = "(clean working tree)"
		}
		return s, nil
	case "git_diff":
		cmd := exec.Command("git", "diff")
		cmd.Dir = workspace
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git diff failed: %v", err)
		}
		s := string(out)
		if s == "" {
			s = "(no changes)"
		}
		return s, nil
	case "create_file", "write_file":
		path := safeJoin(workspace, args["path"])
		content := args["content"]
		os.MkdirAll(filepath.Dir(path), 0755)
		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✓ %s (%d bytes)", path, len(content)), nil
	case "create_directory":
		path := safeJoin(workspace, args["path"])
		err := os.MkdirAll(path, 0755)
		if err != nil {
			return "", err
		}
		return "✓ " + path, nil
	case "delete_file":
		path := safeJoin(workspace, args["path"])
		err := os.Remove(path)
		if err != nil {
			return "", err
		}
		return "✓ Deleted " + path, nil
	case "delete_directory":
		path := safeJoin(workspace, args["path"])
		err := os.RemoveAll(path)
		if err != nil {
			return "", err
		}
		return "✓ Deleted " + path, nil
	case "run_command":
		cmdStr := args["command"]
		timeout := 60
		if t, ok := args["timeout"]; ok {
			fmt.Sscanf(t, "%d", &timeout)
		}
		cmd := exec.Command("bash", "-c", cmdStr)
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
			out := outBuf.String()
			if errBuf.Len() > 0 {
				out += "\n[stderr]\n" + errBuf.String()
			}
			if err != nil {
				return out + fmt.Sprintf("\nExit error: %v", err), nil
			}
			if out == "" {
				out = "(no output)"
			}
			return out, nil
		case <-time.After(time.Duration(timeout) * time.Second):
			cmd.Process.Kill()
			return "", fmt.Errorf("timed out after %ds", timeout)
		}
	}
	return "", fmt.Errorf("unknown tool: %s", name)
}

func safeJoin(base, rel string) string {
	abs := filepath.Join(base, rel)
	abs = filepath.Clean(abs)
	if !strings.HasPrefix(abs, base) {
		return base
	}
	return abs
}

// ---------- Shell bang ----------
func handleShellBang(cmd string) {
	cmd = strings.TrimSpace(cmd)
	fmt.Printf("\n%s→ Shell command:%s %s\n\n", yellow, reset, cmd)
	fmt.Print("Allow? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	resp, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(resp)) == "y" {
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
	if err := cmd.Run(); err != nil {
		fmt.Printf("%sCommand failed: %v%s\n", red, err, reset)
	}
}

// ---------- Other commands ----------
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
		path, _ := filepath.Abs(args[1])
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			fmt.Println("Not a valid directory.")
			return
		}
		workspace = path
		config.Workspace = path
		saveConfig()
		fmt.Printf("Workspace set to: %s\n", workspace)
	default:
		fmt.Println("Unknown workspace subcommand.")
	}
}

func handleProviderStatus() {
	fmt.Printf("Provider: %s\n", config.ProviderEndpoint)
	fmt.Printf("Model: %s\n", config.Model)
	fmt.Printf("API key: %s\n", maskAPIKey(config.APIKey))
}

func handleConnectionStatus() {
	fmt.Println("Testing connection (with DNS fallback)...")
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
	fmt.Printf("API Key: %s\n", maskAPIKey(config.APIKey))
	fmt.Println("Testing with DNS chain...")
	err := checkProviderConnection()
	if err != nil {
		fmt.Printf("%sFAILED: %v%s\n", red, err, reset)
	} else {
		fmt.Printf("%sOK%s\n", green, reset)
	}
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
		for _, m := range memories {
			if strings.EqualFold(m.Content, text) {
				fmt.Printf("⚠ Already exists [%d]\n", m.ID)
				return
			}
		}
		tag := "fact"
		if strings.Contains(strings.ToLower(text), "prefer") || strings.Contains(strings.ToLower(text), "ترجیح") {
			tag = "preference"
		} else if strings.Contains(strings.ToLower(text), "project") || strings.Contains(strings.ToLower(text), "پروژه") {
			tag = "project"
		}
		m := Memory{ID: len(memories) + 1, Tag: tag, Content: text, Added: time.Now().Format(time.RFC3339)}
		memories = append(memories, m)
		saveMemories()
		fmt.Printf("✓ Saved [%d] (%s)\n", m.ID, m.Tag)
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
		fmt.Printf("✓ Forgotten %d.\n", id)
	case "clear-session":
		sessionMessages = nil
		fmt.Println("✓ Session cleared.")
	case "clear-project":
		os.RemoveAll(filepath.Join(workspace, ".nooty"))
		fmt.Println("✓ Project memories cleared.")
	case "export":
		data, _ := json.MarshalIndent(memories, "", "  ")
		fmt.Println(string(data))
	}
}

func handleSafety(args []string) {
	if len(args) == 0 {
		fmt.Printf("Safety: %s\n", config.Safety)
		return
	}
	if args[0] == "status" {
		fmt.Printf("Safety: %s\n", config.Safety)
	} else if args[0] == "strict" || args[0] == "balanced" {
		config.Safety = args[0]
		saveConfig()
		fmt.Printf("Safety set to %s.\n", config.Safety)
	} else {
		fmt.Println("Usage: /safety [strict|balanced|status]")
	}
}

func showHistory() {
	if len(sessionMessages) == 0 {
		fmt.Println("No messages.")
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

// ---------- Persistence ----------
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
	json.Unmarshal(data, &config)
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
