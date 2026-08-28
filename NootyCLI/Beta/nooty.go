// nooty.go — NootyCLI v0.3.0 "Radin Agent Core"
// Single-file, zero external dependencies.
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
	"sort"
	"strconv"
	"strings"
	"time"
)

var useColor = true

func init() {
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

type Config struct {
	ProviderEndpoint string `json:"provider_endpoint"`
	APIKey           string `json:"api_key"`
	Model            string `json:"model"`
	Safety           string `json:"safety"`
	Workspace        string `json:"workspace"`
}

type Memory struct {
	ID                  int `json:"id"`
	Tag, Content, Added string
}
type ProjectInfo struct {
	Type, TestCommand string
	FilesCount        int
	Rules             string
	IsGit             bool
}
type Snapshot struct{ ID, Timestamp, FilePath, BackupPath string }
type Message struct{ Role, Content string }
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
type DNSResolver struct{ Name, Address string }

var (
	config                                        Config
	memories                                      []Memory
	snapshots                                     []Snapshot
	sessionMessages                               []Message
	currentMode                                   = "cli"
	workspace, homeDir, nootyDir, projectNootyDir string
	memFile, configFile, snapshotDir, snapshotLog string
	projInfo                                      ProjectInfo
	fallbackDNS                                   = []DNSResolver{
		{"Direct Connection", ""}, {"Electro DNS", "78.157.42.100"},
		{"Shecan DNS #1", "178.22.122.100"}, {"Shecan DNS #2", "185.51.200.2"},
		{"Begzar DNS #1", "185.55.226.26"}, {"Begzar DNS #2", "185.55.225.25"},
	}
	activeDNSName = "Direct Connection"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "NootyCLI v0.3.0 — Autonomous Code Agent Core")
		fmt.Fprintln(os.Stderr, "Usage: nooty [options] [task|subcommand]")
		fmt.Fprintln(os.Stderr, "Commands: nooty \"task\", init, review, test, explain <file>, ask <question>, doctor")
		flag.PrintDefaults()
	}
	flag.Parse()
	var err error
	homeDir, err = os.UserHomeDir()
	if err != nil {
		fatal("Cannot locate user home directory")
	}
	nootyDir = filepath.Join(homeDir, ".nooty")
	_ = os.MkdirAll(filepath.Join(nootyDir, "chats"), 0700)
	configFile = filepath.Join(nootyDir, "config.json")
	memFile = filepath.Join(nootyDir, "memories.json")
	loadConfig()
	loadMemories()
	if config.Workspace == "" {
		config.Workspace, _ = os.Getwd()
		if config.Workspace == "" {
			config.Workspace = homeDir
		}
	}
	workspace = config.Workspace
	refreshWorkspaceState()

	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "init":
			runInit()
		case "doctor":
			runDoctor()
		case "review":
			runReview()
		case "test":
			runAutoTests()
		case "explain":
			target := "."
			if len(args) > 1 {
				target = args[1]
			}
			runExplain(target)
		case "ask":
			if len(args) > 1 {
				currentMode = "chat"
				handleChat(strings.Join(args[1:], " "))
			} else {
				fmt.Println("Usage: nooty ask <question>")
			}
		default:
			currentMode = "cli"
			drawHeader()
			task := strings.Join(args, " ")
			fmt.Printf("%s🎯 Direct Task:%s %s\n\n", c(bold)+c(yellow), c(reset), task)
			handleChat(task)
		}
		return
	}
	drawHeader()
	repl()
}

func fatal(msg string) { fmt.Fprintln(os.Stderr, "⚠ "+msg); os.Exit(1) }

func refreshWorkspaceState() {
	projectNootyDir = filepath.Join(workspace, ".nooty")
	snapshotDir = filepath.Join(projectNootyDir, "snapshots")
	snapshotLog = filepath.Join(projectNootyDir, "snapshots.json")
	_ = os.MkdirAll(snapshotDir, 0700)
	loadSnapshots()
	projInfo = detectProject(workspace)
}

func drawHeader() {
	width := 66
	line := strings.Repeat("─", width-2)
	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), centerText(" NOOTY CLI ", width-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), centerText("v0.3.0 Radin Agent Core — Code Agent Intelligence", width-2), c(cyan), c(reset))
	fmt.Println(c(cyan) + "├" + line + "┤" + c(reset))
	git := "Not a Git Repo"
	if projInfo.IsGit {
		git = "Git Enabled"
	}
	entries := [][]string{{"Provider", truncateString(config.ProviderEndpoint, 38)}, {"Model", config.Model}, {"Project", fmt.Sprintf("%s (%s)", projInfo.Type, git)}, {"Workspace", truncateString(formatPath(workspace), 38)}, {"DNS Shield", activeDNSName}, {"Mode / Safety", fmt.Sprintf("%s (%s)", strings.ToUpper(currentMode), config.Safety)}}
	for _, e := range entries {
		fmt.Printf("%s│%s %-12s: %s%-38s %s│%s\n", c(cyan), c(bold)+c(white), e[0], c(green), e[1], c(cyan), c(reset))
	}
	fmt.Println(c(cyan) + "└" + line + "┘" + c(reset))
	if projInfo.Rules != "" {
		fmt.Printf("%s📌 Project rules loaded from .nooty/rules.md%s\n", c(yellow), c(reset))
	}
	fmt.Printf("%s💡 /help  /undo  /changes  /review  /test  /context%s\n\n", c(dim), c(reset))
}

func formatPath(p string) string {
	if strings.HasPrefix(p, homeDir) {
		return "~" + strings.TrimPrefix(p, homeDir)
	}
	return p
}
func centerText(s string, w int) string {
	if len(s) >= w {
		return s[:w]
	}
	l := (w - len(s)) / 2
	return strings.Repeat(" ", l) + s + strings.Repeat(" ", w-l-len(s))
}
func truncateString(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return "..." + s[len(s)-n+3:]
}
func maskAPIKey(k string) string {
	if k == "" {
		return "(not configured)"
	}
	if len(k) <= 8 {
		return k[:1] + strings.Repeat("*", len(k)-1)
	}
	return k[:4] + strings.Repeat("*", len(k)-8) + k[len(k)-4:]
}
func exists(p string) bool { _, e := os.Stat(p); return e == nil }

func detectProject(dir string) ProjectInfo {
	p := ProjectInfo{Type: "Generic Codebase"}
	if exists(filepath.Join(dir, ".git")) {
		p.IsGit = true
	}
	if entries, e := os.ReadDir(dir); e == nil {
		p.FilesCount = len(entries)
	}
	switch {
	case exists(filepath.Join(dir, "go.mod")):
		p.Type = "Go Modules"
		p.TestCommand = "go test ./..."
	case exists(filepath.Join(dir, "package.json")):
		p.Type = "Node.js"
		p.TestCommand = "npm test"
	case exists(filepath.Join(dir, "pyproject.toml")) || exists(filepath.Join(dir, "requirements.txt")):
		p.Type = "Python"
		p.TestCommand = "pytest"
	case exists(filepath.Join(dir, "Cargo.toml")):
		p.Type = "Rust Cargo"
		p.TestCommand = "cargo test"
	case exists(filepath.Join(dir, "composer.json")):
		p.Type = "PHP Composer"
		p.TestCommand = "vendor/bin/phpunit"
	case exists(filepath.Join(dir, "pom.xml")) || exists(filepath.Join(dir, "build.gradle")):
		p.Type = "Java Project"
		p.TestCommand = "mvn test"
	}
	if b, e := os.ReadFile(filepath.Join(dir, ".nooty", "rules.md")); e == nil {
		p.Rules = strings.TrimSpace(string(b))
	}
	return p
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
	fmt.Println(c(dim) + "\n👋 NootyCLI session ended." + c(reset))
}
func prompt() string {
	if currentMode == "cli" {
		return c(bold) + c(cyan) + "🤖 nooty[agent]" + c(yellow) + " ❯ " + c(reset)
	}
	return c(bold) + c(green) + "⚡ nooty" + c(white) + " ❯ " + c(reset)
}

func handleSlashCommand(cmd string) {
	p := strings.Fields(cmd)
	if len(p) == 0 {
		return
	}
	switch p[0] {
	case "/help":
		printHelp()
	case "/mode":
		if len(p) > 1 && p[1] == "cli" {
			currentMode = "cli"
		} else {
			currentMode = "chat"
		}
		fmt.Println("✅ Mode: " + currentMode)
	case "/workspace":
		handleWorkspace(p[1:])
	case "/model":
		handleModelCommand(p[1:])
	case "/config":
		handleConfig()
	case "/dns":
		showDNSStatus()
	case "/doctor":
		runDoctor()
	case "/undo", "/revert":
		undoLastSnapshot()
	case "/changes":
		listSnapshots()
	case "/init":
		runInit()
	case "/review":
		runReview()
	case "/test":
		runAutoTests()
	case "/context":
		showContextBudget()
	case "/memory":
		handleMemory(p[1:])
	case "/safety":
		handleSafety(p[1:])
	case "/history":
		showHistory()
	case "/git":
		handleGitCommand(p[1:])
	case "/commit":
		runAutoCommit()
	case "/clear":
		sessionMessages = nil
		clearScreen()
		drawHeader()
	case "/exit":
		os.Exit(0)
	default:
		fmt.Printf("❌ Unknown command: %s. Type /help.\n", p[0])
	}
}

func printHelp() {
	fmt.Println(c(bold) + "\n📌 NootyCLI v0.3 Command Reference:" + c(reset))
	fmt.Println(`
  /help
  /mode [chat|cli]
  /init
  /undo | /revert
  /changes
  /review
  /test
  /context
  /commit
  /git <status|diff|log|branch>
  /config
  /workspace show|set <path>
  /model show|set <name>|list
  /dns
  /doctor
  /memory list|add|forget
  /safety strict|balanced|auto
  /history
  /clear
  /exit

  In Agent Mode: !<shell-command>`)
}
func clearScreen() {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	} else {
		fmt.Print("\033[H\033[2J")
	}
}

func handleConfig() {
	r := bufio.NewReader(os.Stdin)
	fmt.Println("\n⚙️ Nooty Configuration")
	fmt.Printf("Provider endpoint [%s]: ", config.ProviderEndpoint)
	if v, _ := r.ReadString('\n'); strings.TrimSpace(v) != "" {
		config.ProviderEndpoint = strings.TrimSpace(v)
	}
	fmt.Printf("API key [%s]: ", maskAPIKey(config.APIKey))
	if v, _ := r.ReadString('\n'); strings.TrimSpace(v) != "" {
		config.APIKey = strings.TrimSpace(v)
	}
	fmt.Printf("Model [%s]: ", config.Model)
	if v, _ := r.ReadString('\n'); strings.TrimSpace(v) != "" {
		config.Model = strings.TrimSpace(v)
	}
	saveConfig()
	fmt.Println("✅ Configuration saved.")
}

func handleModelCommand(args []string) {
	if len(args) == 0 || args[0] == "show" {
		fmt.Printf("🤖 Active Model: %s\n", config.Model)
		return
	}
	switch args[0] {
	case "set":
		if len(args) < 2 {
			fmt.Println("Usage: /model set <name>")
			return
		}
		config.Model = args[1]
		saveConfig()
		fmt.Println("✅ Model: " + config.Model)
	case "list":
		selectModelInteractive()
	default:
		fmt.Println("Usage: /model show|set|list")
	}
}
func selectModelInteractive() {
	fmt.Println("🔍 Fetching models...")
	models, e := fetchAvailableModels()
	if e != nil {
		fmt.Println("❌", e)
		return
	}
	sort.Strings(models)
	for i, m := range models {
		fmt.Printf("[%d] %s\n", i+1, m)
	}
	fmt.Print("Select number (q=quit): ")
	r := bufio.NewReader(os.Stdin)
	v, _ := r.ReadString('\n')
	v = strings.TrimSpace(v)
	if v == "q" {
		return
	}
	n, e := strconv.Atoi(v)
	if e != nil || n < 1 || n > len(models) {
		fmt.Println("❌ Invalid selection")
		return
	}
	config.Model = models[n-1]
	saveConfig()
	fmt.Println("✅ Model set to:", config.Model)
}

func showDNSStatus() {
	fmt.Println(c(bold) + "\n🛡️ DNS Fallback Chain:" + c(reset))
	for i, d := range fallbackDNS {
		a := "System Default"
		if d.Address != "" {
			a = d.Address
		}
		mark := ""
		if d.Name == activeDNSName {
			mark = " [ACTIVE]"
		}
		fmt.Printf("%d. %-24s %s%s\n", i+1, d.Name, a, mark)
	}
}

func dnsDialer(server string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{}
		return d.DialContext(ctx, network, server+":53")
	}
}
func httpClientForDNS(dns string) *http.Client {
	if dns == "" {
		return &http.Client{Timeout: 40 * time.Second}
	}
	resolver := &net.Resolver{PreferGo: true, Dial: dnsDialer(dns)}
	d := &net.Dialer{Resolver: resolver}
	return &http.Client{Transport: &http.Transport{DialContext: d.DialContext}, Timeout: 40 * time.Second}
}
func doWithFallback(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	var lastErr error
	for i, d := range fallbackDNS {
		client := httpClientForDNS(d.Address)
		var req *http.Request
		var e error
		if body != nil {
			req, e = http.NewRequest(method, url, bytes.NewReader(body))
		} else {
			req, e = http.NewRequest(method, url, nil)
		}
		if e != nil {
			return nil, e
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, e := client.Do(req)
		if e == nil && resp.StatusCode != 403 && resp.StatusCode != 451 {
			activeDNSName = d.Name
			return resp, nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		if e != nil {
			lastErr = e
		}
		if i < len(fallbackDNS)-1 {
			fmt.Printf("%s⚠️ Network failed via %s → trying %s%s\n", c(yellow), d.Name, fallbackDNS[i+1].Name, c(reset))
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("all DNS resolvers exhausted")
}

func fetchAvailableModels() ([]string, error) {
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/models"
	h := map[string]string{}
	if config.APIKey != "" {
		h["Authorization"] = "Bearer " + config.APIKey
	}
	resp, e := doWithFallback("GET", endpoint, nil, h)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	var r struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if e = json.Unmarshal(b, &r); e != nil {
		return nil, e
	}
	out := make([]string, 0, len(r.Data))
	for _, m := range r.Data {
		out = append(out, m.ID)
	}
	return out, nil
}

func handleChat(input string) {
	m := buildMessages(input)
	if currentMode == "cli" {
		runAgentLoop(m)
	} else {
		streamResponse(m)
	}
}
func buildMessages(userInput string) []Message {
	sys := `You are NootyCLI v0.3, an autonomous code agent. In chat mode answer as a concise senior engineer. In CLI mode inspect, edit, test, and verify the workspace. To call exactly one tool, output one line in this exact form: TOOL: tool_name key="value". Tools: read_file, write_file, apply_patch, delete_file, list_files, search_files, search_code, find_symbol, file_info, tree, git_status, git_diff, git_log, git_branch, run_command, run_test, run_linter, project_info, env_info. apply_patch uses <<<<<<< SEARCH / ======= / >>>>>>> REPLACE blocks. Never invent a tool.`
	if projInfo.Rules != "" {
		sys += "\nProject Rules:\n" + projInfo.Rules
	}
	msgs := []Message{{Role: "system", Content: sys}}
	start := 0
	if len(sessionMessages) > 12 {
		start = len(sessionMessages) - 12
	}
	msgs = append(msgs, sessionMessages[start:]...)
	u := Message{Role: "user", Content: userInput}
	msgs = append(msgs, u)
	sessionMessages = append(sessionMessages, u)
	return msgs
}
func getRelevantMemories(q string) []Memory {
	q = strings.ToLower(q)
	var out []Memory
	for _, m := range memories {
		if strings.Contains(strings.ToLower(m.Content), q) || strings.Contains(strings.ToLower(m.Tag), q) {
			out = append(out, m)
		}
	}
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}
func showContextBudget() {
	n := 1800 + len(projInfo.Rules)
	for _, m := range sessionMessages {
		n += len(m.Content)
	}
	fmt.Printf("\n📊 Estimated context: ~%d tokens (%d messages)\n", n/4, len(sessionMessages))
}

func streamResponse(messages []Message) {
	payload, _ := json.Marshal(ChatRequest{Model: config.Model, Messages: messages, Stream: true})
	h := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" {
		h["Authorization"] = "Bearer " + config.APIKey
	}
	resp, e := doWithFallback("POST", strings.TrimRight(config.ProviderEndpoint, "/")+"/chat/completions", payload, h)
	if e != nil {
		fmt.Println("❌", e)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ Provider Error %d: %s\n", resp.StatusCode, b)
		return
	}
	r := bufio.NewReader(resp.Body)
	var full strings.Builder
	fmt.Print(c(cyan))
	for {
		line, e := r.ReadString('\n')
		if e != nil {
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
		var ch ChatStreamChunk
		if json.Unmarshal([]byte(data), &ch) == nil {
			for _, x := range ch.Choices {
				fmt.Print(x.Delta.Content)
				full.WriteString(x.Delta.Content)
			}
		}
	}
	fmt.Print(c(reset) + "\n\n")
	sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: full.String()})
}

func getModelResponseText(messages []Message) (string, error) {
	payload, _ := json.Marshal(ChatRequest{Model: config.Model, Messages: messages, Stream: false})
	h := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" {
		h["Authorization"] = "Bearer " + config.APIKey
	}
	resp, e := doWithFallback("POST", strings.TrimRight(config.ProviderEndpoint, "/")+"/chat/completions", payload, h)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	var r struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if e = json.Unmarshal(b, &r); e != nil {
		return "", e
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("empty choices")
	}
	return r.Choices[0].Message.Content, nil
}

func runAgentLoop(messages []Message) {
	planMsgs := append(append([]Message{}, messages...), Message{Role: "user", Content: "Create a concise numbered execution plan. Do not execute tools yet."})
	fmt.Print(c(yellow) + "🤔 Planning... " + c(reset))
	plan, e := getModelResponseText(planMsgs)
	if e != nil {
		fmt.Println("❌", e)
		return
	}
	fmt.Println("\n" + c(cyan) + "📋 Plan:" + c(reset) + "\n" + plan + "\n")
	if config.Safety != "auto" && !confirm("Approve plan? [Y/n]: ") {
		fmt.Println("🛑 Cancelled.")
		return
	}
	msgs := append(append([]Message{}, messages...), Message{Role: "assistant", Content: plan}, Message{Role: "user", Content: "Plan approved. Execute step by step with one TOOL call at a time. After changes, run tests and verify."})
	for i := 0; i < 12; i++ {
		text, e := getModelResponseText(msgs)
		if e != nil {
			fmt.Println("❌", e)
			return
		}
		tc := extractToolCall(text)
		if tc == nil {
			fmt.Println("\n" + c(green) + text + c(reset))
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: text})
			return
		}
		fmt.Printf("\n%s🔧 Action %d: %s%s\n", c(yellow), i+1, tc.Name, c(reset))
		res, ok := executeAgentTool(tc)
		if !ok {
			msgs = append(msgs, Message{Role: "user", Content: "Action denied."})
			continue
		}
		if len(res) > 2500 {
			res = summarizeToolOutput(res)
		}
		fmt.Println(res)
		msgs = append(msgs, Message{Role: "assistant", Content: text}, Message{Role: "user", Content: "Tool output:\n" + res})
	}
	fmt.Println(c(yellow) + "⚠️ Agent step limit reached." + c(reset))
}
func confirm(prompt string) bool {
	fmt.Print(prompt)
	r := bufio.NewReader(os.Stdin)
	v, _ := r.ReadString('\n')
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "" || v == "y" || v == "yes"
}
func summarizeToolOutput(s string) string {
	if len(s) <= 2500 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= 40 {
		return truncateString(s, 2400) + "\n... (output truncated)"
	}
	return strings.Join(lines[:25], "\n") + fmt.Sprintf("\n\n... [%d lines omitted] ...\n\n", len(lines)-40) + strings.Join(lines[len(lines)-15:], "\n")
}

func extractToolCall(text string) *ToolCall {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TOOL:") {
			return parseToolLine(line)
		}
	}
	return nil
}
func parseToolLine(line string) *ToolCall {
	line = strings.TrimSpace(strings.TrimPrefix(line, "TOOL:"))
	parts := strings.SplitN(line, " ", 2)
	name := parts[0]
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}
	return parseToolArgs(name, args)
}
func parseToolArgs(name, s string) *ToolCall {
	args := map[string]string{}
	re := regexp.MustCompile(`(\w+)=("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|\S+)`)
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		v := m[2]
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		v = strings.ReplaceAll(v, "\\n", "\n")
		v = strings.ReplaceAll(v, "\\t", "\t")
		args[m[1]] = v
	}
	return &ToolCall{Name: name, Args: args}
}

func executeAgentTool(tc *ToolCall) (string, bool) {
	needs := false
	switch config.Safety {
	case "strict":
		switch tc.Name {
		case "write_file", "apply_patch", "delete_file", "run_command", "run_test", "run_linter":
			needs = true
		}
	case "balanced":
		switch tc.Name {
		case "delete_file", "run_command", "run_test", "run_linter":
			needs = true
		}
	}
	if tc.Name == "delete_file" && needs {
		fmt.Print("Type DELETE to confirm: ")
		r := bufio.NewReader(os.Stdin)
		v, _ := r.ReadString('\n')
		if strings.TrimSpace(v) != "DELETE" {
			return "Delete cancelled.", false
		}
	} else if needs && !confirm("Confirm tool execution? [Y/n]: ") {
		return "Operation cancelled.", false
	}
	if tc.Name == "run_command" && isDangerousCommand(tc.Args["command"]) {
		fmt.Println(c(red) + "⚠️ Dangerous command detected; explicit confirmation required." + c(reset))
		if !confirm("Continue? [Y/n]: ") {
			return "Blocked by safety policy.", false
		}
	}
	res, e := runTool(tc.Name, tc.Args)
	if e != nil {
		return "Tool Error: " + e.Error(), true
	}
	return res, true
}
func isDangerousCommand(cmd string) bool {
	for _, p := range []string{`rm\s+-rf\s+/`, `rm\s+-rf\s+\*`, `mkfs`, `dd\s+if=`, `>\s*/dev/`, `chmod\s+-R\s+777`, `shutdown`, `reboot`, `drop\s+database`} {
		if ok, _ := regexp.MatchString(`(?i)`+p, cmd); ok {
			return true
		}
	}
	return false
}

func runTool(name string, args map[string]string) (string, error) {
	switch name {
	case "read_file":
		b, e := os.ReadFile(safeJoin(workspace, args["path"]))
		return string(b), e
	case "write_file":
		p := args["path"]
		full := safeJoin(workspace, p)
		old := ""
		if b, e := os.ReadFile(full); e == nil {
			old = string(b)
		}
		if e := createSnapshot(p); e != nil {
			return "", e
		}
		if e := os.MkdirAll(filepath.Dir(full), 0755); e != nil {
			return "", e
		}
		if e := atomicWrite(full, []byte(args["content"]), 0644); e != nil {
			return "", e
		}
		return "✅ File written:\n" + produceDiff(p, old, args["content"]), nil
	case "apply_patch":
		p := args["path"]
		full := safeJoin(workspace, p)
		b, e := os.ReadFile(full)
		if e != nil {
			return "", e
		}
		old := string(b)
		nw, e := applyPatchContent(old, args["patch"])
		if e != nil {
			return "", e
		}
		if e = createSnapshot(p); e != nil {
			return "", e
		}
		if e = atomicWrite(full, []byte(nw), 0644); e != nil {
			return "", e
		}
		return "✅ Patch applied:\n" + produceDiff(p, old, nw), nil
	case "delete_file":
		p := args["path"]
		full := safeJoin(workspace, p)
		if e := createSnapshot(p); e != nil {
			return "", e
		}
		return "✅ Removed: " + p, os.Remove(full)
	case "list_files":
		p := workspace
		if args["path"] != "" {
			p = safeJoin(workspace, args["path"])
		}
		ents, e := os.ReadDir(p)
		if e != nil {
			return "", e
		}
		names := []string{}
		for _, x := range ents {
			if strings.HasPrefix(x.Name(), ".") {
				continue
			}
			if x.IsDir() {
				names = append(names, x.Name()+"/")
			} else {
				names = append(names, x.Name())
			}
		}
		sort.Strings(names)
		if len(names) == 0 {
			return "(directory empty)", nil
		}
		return strings.Join(names, "\n"), nil
	case "search_files":
		return searchFiles(args["pattern"]), nil
	case "search_code":
		return searchCode(args["query"], args["path"]), nil
	case "find_symbol":
		return findSymbol(args["name"]), nil
	case "file_info":
		i, e := os.Stat(safeJoin(workspace, args["path"]))
		if e != nil {
			return "", e
		}
		return fmt.Sprintf("Path: %s\nSize: %d\nMode: %s\nModified: %s", safeJoin(workspace, args["path"]), i.Size(), i.Mode(), i.ModTime().Format(time.RFC3339)), nil
	case "tree":
		p := workspace
		if args["path"] != "" {
			p = safeJoin(workspace, args["path"])
		}
		return dirTree(p, ""), nil
	case "git_status":
		return runGit("status", "--short")
	case "git_diff":
		return runGit("diff")
	case "git_log":
		count := args["count"]
		if count == "" {
			count = "5"
		}
		return runGit("log", "-n", count, "--oneline")
	case "git_branch":
		return runGit("branch", "-a")
	case "run_command":
		t := 60
		if v, e := strconv.Atoi(args["timeout"]); e == nil && v > 0 && v <= 600 {
			t = v
		}
		return executeShellCommand(args["command"], t)
	case "run_test":
		if projInfo.TestCommand == "" {
			return "No test command detected.", nil
		}
		return executeShellCommand(projInfo.TestCommand, 90)
	case "run_linter":
		if projInfo.Type == "Go Modules" {
			return executeShellCommand("go vet ./...", 60)
		}
		return "No default linter configured.", nil
	case "project_info":
		return fmt.Sprintf("Type: %s\nTests: %s\nFiles: %d\nGit: %v\nRules: %v", projInfo.Type, projInfo.TestCommand, projInfo.FilesCount, projInfo.IsGit, projInfo.Rules != ""), nil
	case "env_info":
		return fmt.Sprintf("OS: %s\nArch: %s\nRuntime: %s\nWorkspace: %s", runtime.GOOS, runtime.GOARCH, runtime.Version(), workspace), nil
	}
	return "", fmt.Errorf("unknown tool: %s", name)
}

func atomicWrite(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".nooty-tmp"
	if e := os.WriteFile(tmp, data, perm); e != nil {
		return e
	}
	if e := os.Rename(tmp, path); e != nil {
		_ = os.Remove(tmp)
		return e
	}
	return nil
}
func searchFiles(pattern string) string {
	var out []string
	_ = filepath.Walk(workspace, func(p string, info os.FileInfo, e error) error {
		if e != nil || info == nil || info.IsDir() || strings.Contains(p, string(filepath.Separator)+".git"+string(filepath.Separator)) || strings.Contains(p, string(filepath.Separator)+"node_modules"+string(filepath.Separator)) {
			return nil
		}
		if strings.Contains(strings.ToLower(info.Name()), strings.ToLower(pattern)) {
			r, _ := filepath.Rel(workspace, p)
			out = append(out, r)
		}
		return nil
	})
	if len(out) == 0 {
		return "No matching files found."
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}
func searchCode(query, scope string) string {
	root := workspace
	if scope != "" && scope != "." {
		root = safeJoin(workspace, scope)
	}
	var out []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, e error) error {
		if e != nil || info == nil || info.IsDir() || info.Size() > 1000000 || strings.Contains(p, string(filepath.Separator)+".git"+string(filepath.Separator)) || strings.Contains(p, string(filepath.Separator)+"node_modules"+string(filepath.Separator)) {
			return nil
		}
		b, e := os.ReadFile(p)
		if e == nil && strings.Contains(string(b), query) {
			r, _ := filepath.Rel(workspace, p)
			out = append(out, r)
		}
		return nil
	})
	if len(out) == 0 {
		return "No matches found."
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}
func findSymbol(sym string) string {
	var out []string
	_ = filepath.Walk(workspace, func(p string, info os.FileInfo, e error) error {
		if e != nil || info == nil || info.IsDir() || info.Size() > 500000 {
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return nil
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, sym) && (strings.Contains(line, "func ") || strings.Contains(line, "class ") || strings.Contains(line, "function ") || strings.Contains(line, "def ")) {
				r, _ := filepath.Rel(workspace, p)
				out = append(out, fmt.Sprintf("%s:%d -> %s", r, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if len(out) == 0 {
		return "No symbol definitions found."
	}
	return strings.Join(out, "\n")
}
func dirTree(root, indent string) string {
	ents, e := os.ReadDir(root)
	if e != nil {
		return e.Error()
	}
	var b strings.Builder
	shown := 0
	for _, x := range ents {
		if strings.HasPrefix(x.Name(), ".") || x.Name() == "node_modules" || x.Name() == "vendor" {
			continue
		}
		shown++
		prefix := indent + "├── "
		next := indent + "│   "
		if shown == len(ents) {
			prefix = indent + "└── "
			next = indent + "    "
		}
		b.WriteString(prefix + x.Name() + "\n")
		if x.IsDir() {
			b.WriteString(dirTree(filepath.Join(root, x.Name()), next))
		}
	}
	return b.String()
}

func applyPatchContent(original, patch string) (string, error) {
	re := regexp.MustCompile(`(?s)<<<<<<< SEARCH\r?\n(.*?)\r?\n=======\r?\n(.*?)\r?\n>>>>>>> REPLACE`)
	m := re.FindAllStringSubmatch(patch, -1)
	if len(m) == 0 {
		return "", fmt.Errorf("invalid patch format")
	}
	result := original
	for _, x := range m {
		if !strings.Contains(result, x[1]) {
			return "", fmt.Errorf("search block not found")
		}
		result = strings.Replace(result, x[1], x[2], 1)
	}
	return result, nil
}
func produceDiff(file, oldText, newText string) string {
	ol := strings.Split(oldText, "\n")
	nl := strings.Split(newText, "\n")
	var b strings.Builder
	b.WriteString("--- " + file + "\n+++ " + file + "\n")
	max := len(ol)
	if len(nl) > max {
		max = len(nl)
	}
	changes := 0
	for i := 0; i < max; i++ {
		var a, z string
		if i < len(ol) {
			a = ol[i]
		}
		if i < len(nl) {
			z = nl[i]
		}
		if a != z {
			if i < len(ol) {
				b.WriteString("- " + a + "\n")
			}
			if i < len(nl) {
				b.WriteString("+ " + z + "\n")
			}
			changes++
		}
	}
	if changes == 0 {
		return "(no line changes)"
	}
	return b.String()
}

func executeShellCommand(command string, timeout int) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Dir = workspace
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if e := cmd.Start(); e != nil {
		return "", e
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case e := <-done:
		s := out.String()
		if errOut.Len() > 0 {
			s += "\n[stderr]\n" + errOut.String()
		}
		if e != nil {
			s += fmt.Sprintf("\nExit status: %v", e)
		}
		if s == "" {
			s = "(no output)"
		}
		return s, nil
	case <-time.After(time.Duration(timeout) * time.Second):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("execution timed out (%ds)", timeout)
	}
}
func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workspace
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if e := cmd.Run(); e != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = e.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	s := strings.TrimSpace(out.String())
	if s == "" {
		s = "(no output)"
	}
	return s, nil
}

func runInit() {
	_ = os.MkdirAll(snapshotDir, 0700)
	rules := filepath.Join(projectNootyDir, "rules.md")
	if !exists(rules) {
		sample := "# NootyCLI Project Rules\n- Maintain clean architecture.\n- Write tests for new functions.\n- Do not modify database migrations automatically.\n- Prefer returned errors over panics.\n"
		if e := os.WriteFile(rules, []byte(sample), 0644); e != nil {
			fmt.Println("❌", e)
			return
		}
	}
	ctx := filepath.Join(projectNootyDir, "context.json")
	if !exists(ctx) {
		_ = os.WriteFile(ctx, []byte("{\n  \"project_notes\": \"\"\n}\n"), 0644)
	}
	refreshWorkspaceState()
	fmt.Printf("✅ .nooty initialized in %s\n", workspace)
}

func createSnapshot(rel string) error {
	full := safeJoin(workspace, rel)
	b, e := os.ReadFile(full)
	if e != nil {
		if os.IsNotExist(e) {
			return nil
		}
		return e
	}
	ts := time.Now().Format("20060102-150405.000000000")
	name := fmt.Sprintf("%s_%s.bak", ts, filepath.Base(rel))
	path := filepath.Join(snapshotDir, name)
	if e = atomicWrite(path, b, 0600); e != nil {
		return e
	}
	snapshots = append(snapshots, Snapshot{ID: fmt.Sprintf("snap_%d", len(snapshots)+1), Timestamp: time.Now().Format(time.RFC3339Nano), FilePath: rel, BackupPath: path})
	return saveSnapshots()
}
func undoLastSnapshot() {
	if len(snapshots) == 0 {
		fmt.Println("⚠️ No snapshots.")
		return
	}
	last := snapshots[len(snapshots)-1]
	b, e := os.ReadFile(last.BackupPath)
	if e != nil {
		fmt.Println("❌", e)
		return
	}
	if e = atomicWrite(safeJoin(workspace, last.FilePath), b, 0644); e != nil {
		fmt.Println("❌", e)
		return
	}
	snapshots = snapshots[:len(snapshots)-1]
	_ = saveSnapshots()
	fmt.Println("✅ Reverted:", last.FilePath)
}
func listSnapshots() {
	if len(snapshots) == 0 {
		fmt.Println("📜 No snapshots.")
		return
	}
	for _, s := range snapshots {
		fmt.Printf("• %s  %s  %s\n", s.ID, s.FilePath, s.Timestamp)
	}
}
func loadSnapshots() {
	b, e := os.ReadFile(snapshotLog)
	if e != nil {
		snapshots = []Snapshot{}
		return
	}
	if json.Unmarshal(b, &snapshots) != nil {
		snapshots = []Snapshot{}
	}
}
func saveSnapshots() error {
	b, e := json.MarshalIndent(snapshots, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(snapshotLog, b, 0600)
}

func safeJoin(base, rel string) string {
	baseAbs, _ := filepath.Abs(base)
	if rel == "" {
		return baseAbs
	}
	candidate := rel
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(baseAbs, candidate)
	}
	candidate, _ = filepath.Abs(candidate)
	r, e := filepath.Rel(baseAbs, candidate)
	if e != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return baseAbs
	}
	return candidate
}

func runReview() {
	diff, e := runGit("diff")
	if e != nil || diff == "(no output)" {
		fmt.Println("ℹ️ No uncommitted changes.")
		return
	}
	messages := []Message{{Role: "system", Content: "You are a senior code reviewer. Find correctness, security, performance, and maintainability issues. Be concise."}, {Role: "user", Content: "Review this git diff:\n\n" + truncateString(diff, 20000)}}
	streamResponse(messages)
}
func runExplain(target string) {
	p := safeJoin(workspace, target)
	b, e := os.ReadFile(p)
	if e != nil {
		fmt.Println("❌", e)
		return
	}
	messages := []Message{{Role: "system", Content: "You are a software architect. Explain the target code and its architecture clearly."}, {Role: "user", Content: "Explain " + target + ":\n\n" + truncateString(string(b), 20000)}}
	streamResponse(messages)
}
func runAutoCommit() {
	diff, e := runGit("diff")
	if e != nil || diff == "(no output)" {
		fmt.Println("ℹ️ No changes.")
		return
	}
	msg, e := getModelResponseText([]Message{{Role: "system", Content: "Return ONLY one Conventional Commit message line."}, {Role: "user", Content: truncateString(diff, 20000)}})
	if e != nil {
		fmt.Println("❌", e)
		return
	}
	msg = strings.ReplaceAll(strings.TrimSpace(msg), "\n", " ")
	fmt.Println("Suggested commit:", msg)
	if !confirm("Commit? [Y/n]: ") {
		return
	}
	out, e := runGit("add", "-A")
	if e != nil {
		fmt.Println("❌", e)
		return
	}
	_ = out
	if out, e = runGit("commit", "-m", msg); e != nil {
		fmt.Println("❌", e)
		return
	}
	fmt.Println(out)
}
func runAutoTests() {
	if projInfo.TestCommand == "" {
		fmt.Println("⚠️ No test runner detected.")
		return
	}
	fmt.Println("🧪", projInfo.TestCommand)
	out, e := executeShellCommand(projInfo.TestCommand, 90)
	fmt.Println(out)
	if e != nil {
		fmt.Println("❌", e)
	}
}
func handleGitCommand(args []string) {
	if len(args) == 0 {
		args = []string{"status", "--short"}
	}
	out, e := runGit(args...)
	if e != nil {
		fmt.Println("❌", e)
	} else {
		fmt.Println(out)
	}
}
func handleShellBang(cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	fmt.Println("⚡", cmd)
	if confirm("Execute? [Y/n]: ") {
		out, e := executeShellCommand(cmd, 120)
		fmt.Println(out)
		if e != nil {
			fmt.Println("❌", e)
		}
	}
}
func handleWorkspace(args []string) {
	if len(args) == 0 || args[0] == "show" {
		fmt.Println("📁 Workspace:", formatPath(workspace))
		return
	}
	if args[0] != "set" || len(args) < 2 {
		fmt.Println("Usage: /workspace set <path>")
		return
	}
	p, e := filepath.Abs(args[1])
	if e != nil {
		fmt.Println("❌", e)
		return
	}
	i, e := os.Stat(p)
	if e != nil || !i.IsDir() {
		fmt.Println("❌ Directory does not exist.")
		return
	}
	workspace = p
	config.Workspace = p
	saveConfig()
	refreshWorkspaceState()
	fmt.Println("✅ Workspace:", formatPath(workspace))
}
func runDoctor() {
	fmt.Println("\n🏥 NootyCLI Doctor")
	fmt.Println("Provider:", config.ProviderEndpoint)
	fmt.Println("Model:", config.Model)
	fmt.Println("API Key:", maskAPIKey(config.APIKey))
	fmt.Println("Workspace:", formatPath(workspace))
	fmt.Println("Project:", projInfo.Type, "Git:", projInfo.IsGit)
	if _, e := fetchAvailableModels(); e != nil {
		fmt.Println("❌ Provider:", e)
	} else {
		fmt.Println("✅ Provider: OK via", activeDNSName)
	}
}
func handleMemory(args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "list":
		if len(memories) == 0 {
			fmt.Println("🧠 No memories.")
			return
		}
		for _, m := range memories {
			fmt.Printf("[%d] (%s) %s\n", m.ID, m.Tag, m.Content)
		}
	case "add":
		if len(args) < 2 {
			fmt.Println("Usage: /memory add <text>")
			return
		}
		m := Memory{ID: len(memories) + 1, Tag: "context", Content: strings.Join(args[1:], " "), Added: time.Now().Format(time.RFC3339)}
		memories = append(memories, m)
		saveMemories()
		fmt.Println("✅ Saved memory", m.ID)
	case "forget":
		if len(args) < 2 {
			return
		}
		id, _ := strconv.Atoi(args[1])
		next := memories[:0]
		for _, m := range memories {
			if m.ID != id {
				next = append(next, m)
			}
		}
		memories = next
		saveMemories()
		fmt.Println("✅ Removed", id)
	}
}
func handleSafety(args []string) {
	if len(args) == 0 {
		fmt.Println("🛡️ Safety:", config.Safety)
		return
	}
	m := strings.ToLower(args[0])
	if m != "strict" && m != "balanced" && m != "auto" {
		fmt.Println("Usage: /safety strict|balanced|auto")
		return
	}
	config.Safety = m
	saveConfig()
	fmt.Println("✅ Safety:", m)
}
func showHistory() {
	if len(sessionMessages) == 0 {
		fmt.Println("💬 No history.")
		return
	}
	for _, m := range sessionMessages {
		fmt.Printf("%s: %s\n", m.Role, truncateString(m.Content, 200))
	}
}

func loadConfig() {
	b, e := os.ReadFile(configFile)
	if e != nil {
		config = Config{ProviderEndpoint: "https://api.openai.com/v1", APIKey: os.Getenv("OPENAI_API_KEY"), Model: "gpt-4o-mini", Safety: "balanced"}
		if config.APIKey == "" {
			config.APIKey = os.Getenv("NOOTY_API_KEY")
		}
		return
	}
	_ = json.Unmarshal(b, &config)
	if config.APIKey == "" {
		config.APIKey = os.Getenv("OPENAI_API_KEY")
		if config.APIKey == "" {
			config.APIKey = os.Getenv("NOOTY_API_KEY")
		}
	}
	if config.Safety == "" {
		config.Safety = "balanced"
	}
}
func saveConfig() { b, _ := json.MarshalIndent(config, "", "  "); _ = atomicWrite(configFile, b, 0600) }
func loadMemories() {
	b, e := os.ReadFile(memFile)
	if e != nil {
		memories = []Memory{}
		return
	}
	if json.Unmarshal(b, &memories) != nil {
		memories = []Memory{}
	}
}
func saveMemories() {
	b, _ := json.MarshalIndent(memories, "", "  ")
	_ = atomicWrite(memFile, b, 0600)
}
