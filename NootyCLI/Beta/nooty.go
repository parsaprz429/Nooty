// nooty.go — NootyCLI v0.3 "Radin Ultra" – Agentic Terminal Intelligence
// Single-file, zero external dependencies, cross-platform (macOS/Linux/Windows/WSL).
//
// Build:  go build -ldflags="-s -w" -o nooty nooty.go
//
// v0.3 highlights: patch_file/replace_in_file, parallel tools, live streaming
// subprocess output, token-budget context + auto-compaction, session persistence,
// checkpoints + /undo, concurrent DNS racing, global HTTP pool, retry/backoff,
// spinner, token telemetry, multiline & piped input, @file/@dir injection,
// find_files, lint_or_check, git_quick_commit, self-correction loop, signals,
// panic recovery, diff preview, .gitignore-aware walking.

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
	"sync/atomic"
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

type Session struct {
	Name     string    `json:"name"`
	Messages []Message `json:"messages"`
	Mode     string    `json:"mode"`
	Created  time.Time `json:"created"`
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
	Path    string    `json:"path"`
	Backup  string    `json:"backup"`
	Existed bool      `json:"existed"`
	Created time.Time `json:"created"`
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
	memFile         string
	configFile      string
	chatsDir        string
	checkpointsDir  string
	checkpointFile  string

	fallbackDNS = []DNSResolver{
		{Name: "Direct Connection", Address: ""},
		{Name: "Electro DNS", Address: "78.157.42.100"},
		{Name: "Shecan DNS #1", Address: "178.22.122.100"},
		{Name: "Shecan DNS #2", Address: "185.51.200.2"},
		{Name: "Begzar DNS #1", Address: "185.55.226.26"},
		{Name: "Begzar DNS #2", Address: "185.55.225.25"},
	}
	activeDNSName = "Direct Connection"

	globalTransport *http.Transport
	httpClients     sync.Map // dns address -> *http.Client

	agentRunning     atomic.Bool
	agentInterrupted atomic.Bool

	checkpoints   []Checkpoint
	tokenBudget   = 6000
	autoCompactTh = 20
)

// ---------- main ----------
func main() {
	promptFlag := flag.String("p", "", "Direct single prompt execution (non-interactive), then exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "NootyCLI v0.3 \"Radin Ultra\" — Agentic Terminal Intelligence\n\nUsage:\n  nooty [options]\n  cat file | nooty -p \"analyze this\"\n\nOptions:\n")
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
	checkpointFile = filepath.Join(nootyDir, "checkpoints.json")

	// Concurrent DNS racing: pick the fastest resolver before anything else.
	raceDNSResolvers()

	loadConfig()
	loadMemories()
	loadCheckpoints()

	if config.Workspace == "" {
		cwd, err := os.Getwd()
		if err == nil {
			config.Workspace = cwd
		} else {
			config.Workspace = homeDir
		}
	}
	workspace = config.Workspace

	setupSignalHandler()

	// ---------- Pipe / Non-interactive mode ----------
	stat, _ := os.Stdin.Stat()
	isPiped := (stat.Mode() & os.ModeCharDevice) == 0

	if isPiped || *promptFlag != "" {
		var fullPrompt string
		if isPiped {
			pipedData, _ := io.ReadAll(os.Stdin)
			fullPrompt = strings.TrimSpace(string(pipedData))
		}
		if *promptFlag != "" {
			if fullPrompt != "" {
				fullPrompt += "\n\n" + *promptFlag
			} else {
				fullPrompt = *promptFlag
			}
		}
		if strings.TrimSpace(fullPrompt) != "" {
			drawHeader()
			handleChat(strings.TrimSpace(fullPrompt))
			autoSaveSession("piped")
			fmt.Println()
			os.Exit(0)
		}
	}

	drawHeader()
	repl()
	fmt.Println(c(dim) + "\n👋 NootyCLI session ended. Goodbye!" + c(reset))
}

// ---------- Concurrent DNS Racing ----------
// Probe all resolvers in parallel (1.5s timeout) and pick the fastest.
func raceDNSResolvers() {
	type probeResult struct {
		name string
		dur  time.Duration
	}
	results := make(chan probeResult, len(fallbackDNS))
	var wg sync.WaitGroup
	probeURL := "https://www.gstatic.com/generate_204"

	for _, r := range fallbackDNS {
		wg.Add(1)
		go func(r DNSResolver) {
			defer wg.Done()
			start := time.Now()
			if probeDNS(r, probeURL) {
				results <- probeResult{r.Name, time.Since(start)}
			}
		}(r)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	bestName, bestDur, any := "Direct Connection", time.Hour, false
	timeout := time.After(3 * time.Second)
loop:
	for {
		select {
		case r := <-results:
			any = true
			if r.dur < bestDur {
				bestDur, bestName = r.dur, r.name
			}
		case <-done:
			break loop
		case <-timeout:
			break loop
		}
	}
	if any {
		activeDNSName = bestName
	}
}

func probeDNS(r DNSResolver, url string) bool {
	client := &http.Client{
		Transport: transportForDNS(r.Address),
		Timeout:   1500 * time.Millisecond,
	}
	resp, err := client.Head(url)
	if err != nil {
		resp2, err2 := client.Get(url)
		if err2 != nil {
			return false
		}
		_ = resp2.Body.Close()
		return true
	}
	_ = resp.Body.Close()
	return true
}

// ---------- Network Transport Engine (Global Pool + DNS Shield) ----------
func initTransport() {
	if globalTransport != nil {
		return
	}
	globalTransport = &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	}
}

// transportForDNS clones the global shared transport with a custom dialer
// bound to the given anti-sanction DNS resolver.
func transportForDNS(dns string) *http.Transport {
	initTransport()
	t := globalTransport.Clone()
	t.DialContext = dnsDialer(dns)
	return t
}

// httpClientForDNS returns a cached client per DNS (sync.Map keyed by address).
func httpClientForDNS(dns string) *http.Client {
	if v, ok := httpClients.Load(dns); ok {
		return v.(*http.Client)
	}
	client := &http.Client{
		Transport: transportForDNS(dns),
		Timeout:   35 * time.Second,
	}
	actual, _ := httpClients.LoadOrStore(dns, client)
	return actual.(*http.Client)
}

func dnsDialer(dnsServer string) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if dnsServer == "" {
			d := net.Dialer{}
			return d.DialContext(ctx, network, address)
		}
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, network, dnsServer+":53")
			},
		}
		d := net.Dialer{Resolver: resolver}
		return d.DialContext(ctx, network, address)
	}
}

// doWithFallback iterates the resolver chain (with per-attempt retry/backoff).
func doWithFallback(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	var lastErr error
	for i, r := range fallbackDNS {
		resp, err := doWithRetry(method, url, body, headers, r)
		if err == nil {
			activeDNSName = r.Name
			return resp, nil
		}
		lastErr = err
		if i < len(fallbackDNS)-1 {
			fmt.Printf("%s⚠️ %s failed (%v). Trying %s...%s\n",
				c(yellow), r.Name, err, fallbackDNS[i+1].Name, c(reset))
		}
	}
	return nil, fmt.Errorf("network connection failed: all anti-sanction resolvers exhausted (last: %v)", lastErr)
}

// doWithRetry: up to 3 attempts, exponential backoff (1s then 2s) on 5xx/timeout.
func doWithRetry(method, url string, body []byte, headers map[string]string, r DNSResolver) (*http.Response, error) {
	client := httpClientForDNS(r.Address)
	var lastResp *http.Response
	var lastErr error
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
		if err == nil && resp.StatusCode < 500 && resp.StatusCode != 403 && resp.StatusCode != 451 {
			return resp, nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		lastResp, lastErr = resp, err
		if attempt < 2 {
			time.Sleep(time.Duration(1<<attempt) * time.Second) // 1s, 2s
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	code := 0
	if lastResp != nil {
		code = lastResp.StatusCode
	}
	return nil, fmt.Errorf("HTTP %d after retries", code)
}

func apiHeaders() map[string]string {
	headers := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}
	return headers
}

func chatCompletionsURL() string {
	return strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
}

func fetchAvailableModels() ([]string, error) {
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/models"
	resp, err := doWithFallback("GET", endpoint, nil, apiHeaders())
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

// ---------- Minimal Sleek Header ----------
func drawHeader() {
	width := 64
	line := strings.Repeat("─", width-2)

	fmt.Println(c(cyan) + "┌" + line + "┐" + c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(bold)+c(yellow), centerText(" NOOTY CLI ", width-2), c(cyan), c(reset))
	fmt.Printf("%s│%s%s%s│%s\n", c(cyan), c(dim), centerText("v0.3 Radin Ultra — Agentic Terminal Intelligence", width-2), c(cyan), c(reset))
	fmt.Println(c(cyan) + "├" + line + "┤" + c(reset))

	entries := [][]string{
		{"Provider", truncateString(config.ProviderEndpoint, 38)},
		{"Model", config.Model},
		{"API Key", maskAPIKey(config.APIKey)},
		{"Workspace", truncateString(formatPath(workspace), 38)},
		{"DNS Shield", activeDNSName + " (raced)"},
		{"Context", fmt.Sprintf("~%d / %d tokens", estimateTokens(sessionMessages), tokenBudget)},
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

// ---------- Signal Handling (Ctrl+C) ----------
func setupSignalHandler() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		for range sigCh {
			if agentRunning.Load() {
				agentInterrupted.Store(true)
				fmt.Printf("\n%s⏸ Agent interrupted. Session preserved.%s\n", c(yellow), c(reset))
			} else {
				fmt.Println("\n👋 Goodbye!")
				autoSaveSession("autosave")
				os.Exit(0)
			}
		}
	}()
}

// ---------- Interactive REPL Engine ----------
func repl() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for {
		fmt.Print(prompt())
		if !scanner.Scan() {
			break
		}
		line := strings.TrimRight(scanner.Text(), "\r\n")

		// Multiline mode: """ ... """
		if strings.TrimSpace(line) == `"""` {
			var block []string
			for scanner.Scan() {
				inner := strings.TrimRight(scanner.Text(), "\r\n")
				if strings.TrimSpace(inner) == `"""` {
					break
				}
				block = append(block, inner)
			}
			line = strings.Join(block, "\n")
			if strings.TrimSpace(line) == "" {
				continue
			}
		}

		line = strings.TrimSpace(line)
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
	autoSaveSession("autosave")
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
	case "/compact":
		compactHistory(false)
	case "/export":
		exportChat()
	case "/session":
		handleSession(parts[1:])
	case "/undo":
		undoCheckpoint()
	case "/commit":
		gitQuickCommit()
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
		autoSaveSession("autosave")
		os.Exit(0)
	default:
		fmt.Printf("❌ Unknown command: %s. Type /help for assistance.\n", parts[0])
	}
}

func printHelp() {
	fmt.Println(c(bold) + "\n📌 NootyCLI v0.3 \"Radin Ultra\" Command Reference:" + c(reset))
	fmt.Println(`
  /help                        Show command help overview
  /mode [chat|cli]             Toggle Chat or Agentic CLI Execution Mode
  /config                      Interactive wizard for API key, endpoint & model
  /workspace show|set <path>   Manage current working directory
  /model show|set <name>|list  View, switch, or browse models interactively
  /dns                         Display Anti-Sanction Smart DNS Shield status
  /doctor                      Run full connection and API health check
  /memory list|add|forget      Manage long-term persistent agent context
  /safety strict|balanced      Set safety confirmation policies
  /history                     Show session log with token count per message
  /compact                     Summarize older messages into a context block
  /session list|load <n>|save <n>   Manage persistent chat sessions
  /undo                        Restore the most recent file checkpoint
  /commit                      Generate & apply a conventional git commit
  /export                      Export current chat to Markdown
  /clear                       Reset screen & session memory
  /exit                        Terminate NootyCLI session

  💡 Agent Mode: prefix commands with ! for direct shell execution.
  💡 Multiline input: start and end with """ on its own line.
  💡 Inject context: use @file.go or @src/ inside your message.
  💡 Pipe mode: cat logs.txt | nooty -p "find the error"`)
}

func handleConfig() {
	fmt.Println(c(bold) + "\n⚙️ Nooty Configuration Wizard" + c(reset))
	fmt.Println("Press Enter to keep existing settings.\n")
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Provider endpoint [%s]: ", config.ProviderEndpoint)
	ep, _ := reader.ReadString('\n')
	if ep = strings.TrimSpace(ep); ep != "" {
		config.ProviderEndpoint = ep
	}

	fmt.Printf("API key [%s]: ", maskAPIKey(config.APIKey))
	key, _ := reader.ReadString('\n')
	if key = strings.TrimSpace(key); key != "" {
		config.APIKey = key
	}

	fmt.Printf("Model [%s]: ", config.Model)
	mod, _ := reader.ReadString('\n')
	if mod = strings.TrimSpace(mod); mod != "" {
		config.Model = mod
	}

	saveConfig()
	fmt.Println(c(green) + "✅ Configuration saved successfully!\n" + c(reset))
}

func showDNSStatus() {
	fmt.Println(c(bold) + "\n🛡️ Anti-Sanction Smart DNS Fallback Chain (parallel-raced):" + c(reset))
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
	reader := bufio.NewReader(os.Stdin)

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

// ---------- Token Estimation & Context Management ----------
func estimateTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)/4 + 4
	}
	return total
}

// injectFileContext scans input for @file or @dir tokens and embeds contents.
var atRefRe = regexp.MustCompile(`@([a-zA-Z0-9_./\-]+)`)

func injectFileContext(input string) string {
	matches := atRefRe.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return input
	}
	var sb strings.Builder
	for _, m := range matches {
		ref := strings.Trim(m[1], `"'.,`)
		if ref == "" {
			continue
		}
		abs := safeJoin(workspace, ref)
		info, err := os.Stat(abs)
		if err != nil {
			continue // not a real path — leave as-is
		}
		if info.IsDir() {
			sb.WriteString(fmt.Sprintf("\n\n[Directory @%s]:\n", ref))
			sb.WriteString(dirTree(abs, ""))
		} else if info.Size() < 200_000 {
			data, err := os.ReadFile(abs)
			if err != nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("\n\n[File @%s]:\n%s", ref, string(data)))
		}
	}
	if sb.Len() == 0 {
		return input
	}
	return input + "\n" + sb.String()
}

// buildMessages assembles system prompt + compacted history + @file injection.
func buildMessages(userInput string) []Message {
	userInput = injectFileContext(userInput)

	var msgs []Message
	sysPrompt := `You are NootyCLI v0.3 "Radin Ultra", an autonomous agentic terminal AI assistant.

When in CHAT mode: Provide concise, expert terminal and software engineering responses.

When in CLI mode: You act as an autonomous workspace agent.
To execute tools, reply STRICTLY using this exact syntax:
TOOL: tool_name key1="value1" key2="value2"

You may issue MULTIPLE TOOL: lines in one response when the calls are
independent of each other; they will run in parallel.

Available Workspace Tools:
- list_files (path="relative_path")
- tree (path="relative_path")
- read_file (path="relative_path")
- write_file (path="relative_path", content="full_content")
- create_file (path="relative_path", content="initial_content")
- patch_file (path="relative_path", old="exact_existing_text", new="replacement")
- replace_in_file (path="relative_path", old="exact_existing_text", new="replacement")
- append_file (path="relative_path", content="text_to_append")
- delete_file (path="relative_path")
- find_files (pattern="*.go" or "**/*.go")
- search_code (query="text", path="relative_path")
- file_info (path="relative_path")
- lint_or_check (target="relative_path_or_empty")
- run_command (command="shell_cmd", timeout="seconds")
- run_and_verify (command="go build ./...")   -> returns BUILD_FAILED: prefix on failure
- git_status
- git_diff
- git_quick_commit (message="conventional commit message")

IMPORTANT: PREFER patch_file / replace_in_file over full write_file rewrites.
After code changes, use run_and_verify (e.g. "go build ./...") to check your work.`

	relevant := getRelevantMemories(userInput)
	if len(relevant) > 0 {
		sysPrompt += "\n\nUser Context & Memories:\n"
		for _, m := range relevant {
			sysPrompt += fmt.Sprintf("- [%s] %s\n", m.Tag, m.Content)
		}
	}

	msgs = append(msgs, Message{Role: "system", Content: sysPrompt})
	msgs = append(msgs, fitHistory(sessionMessages)...)

	userMsg := Message{Role: "user", Content: userInput}
	msgs = append(msgs, userMsg)
	sessionMessages = append(sessionMessages, userMsg)

	// Auto-compact when message count crosses the threshold.
	if len(sessionMessages) >= autoCompactTh {
		compactHistory(true)
	}
	return msgs
}

// fitHistory keeps history within the token budget; older messages are
// summarized (not silently dropped) via a background non-streaming call.
func fitHistory(history []Message) []Message {
	if estimateTokens(history) <= tokenBudget {
		return history
	}
	kept := []Message{}
	total := 0
	for i := len(history) - 1; i >= 0; i-- {
		t := len(history[i].Content)/4 + 4
		if total+t > tokenBudget/2 {
			break
		}
		total += t
		kept = append([]Message{history[i]}, kept...)
	}
	if len(kept) == 0 {
		return history
	}
	old := history[:len(history)-len(kept)]
	summary := summarizeMessages(old)
	if summary == "" {
		return history
	}
	out := []Message{{Role: "system", Content: "Context (auto-compacted summary of earlier conversation):\n" + summary}}
	return append(out, kept...)
}

// summarizeMessages asks the model (non-streaming) to summarize old messages.
func summarizeMessages(older []Message) string {
	if len(older) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Summarize these conversation messages into a compact project context block (max 200 words). Keep file names, decisions and key facts:\n\n")
	for _, m := range older {
		role := m.Role
		if role == "assistant" {
			role = "AI"
		} else {
			role = "User"
		}
		content := m.Content
		if len(content) > 800 {
			content = content[:800] + "..."
		}
		sb.WriteString(role + ": " + content + "\n")
	}
	resp, err := getModelResponseText([]Message{{Role: "user", Content: sb.String()}})
	if err != nil {
		return ""
	}
	return resp
}

// compactHistory: /compact command — summarize everything except last 3 messages.
func compactHistory(auto bool) {
	if len(sessionMessages) < 6 {
		if !auto {
			fmt.Println("💬 Not enough history to compact.")
		}
		return
	}
	fmt.Printf("%s🧠 Compacting context...%s\n", c(dim), c(reset))
	recent := sessionMessages[len(sessionMessages)-3:]
	older := sessionMessages[:len(sessionMessages)-3]
	summary := summarizeMessages(older)
	if summary == "" {
		fmt.Println(c(yellow) + "⚠️ Compaction failed (API error); history unchanged." + c(reset))
		return
	}
	sessionMessages = append([]Message{{Role: "system", Content: "Context: " + summary}}, recent...)
	fmt.Printf("%s✅ Compacted: %d messages → summary + %d recent. New estimate: ~%d tokens%s\n",
		c(green), len(older)+len(recent), len(recent), estimateTokens(sessionMessages), c(reset))
}

// ---------- Chat Execution ----------
func handleChat(input string) {
	messages := buildMessages(input)
	if currentMode == "cli" {
		runAgentLoop(messages)
		return
	}
	streamResponse(messages)
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

// streamResponse streams a chat-mode answer token-by-token with telemetry.
func streamResponse(messages []Message) {
	reqPayload := ChatRequest{Model: config.Model, Messages: messages, Stream: true}
	jsonData, _ := json.Marshal(reqPayload)

	resp, err := doWithFallback("POST", chatCompletionsURL(), jsonData, apiHeaders())
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

	var fullContent strings.Builder
	streamChunks(resp.Body, &fullContent, true)

	sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: fullContent.String()})
	autoSaveSession("autosave")
}

// streamChunks reads SSE data and prints deltas as they arrive.
func streamChunks(body io.Reader, full *strings.Builder, echo bool) {
	reader := bufio.NewReader(body)
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
				if echo {
					fmt.Print(choice.Delta.Content)
				}
				full.WriteString(choice.Delta.Content)
			}
		}
	}
}

// ---------- Spinner ----------
func showSpinner(stop <-chan struct{}) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	for {
		select {
		case <-stop:
			fmt.Print("\r" + strings.Repeat(" ", 24) + "\r")
			return
		default:
			fmt.Printf("\r%s%s thinking...%s", c(cyan), frames[i%len(frames)], c(reset))
			time.Sleep(80 * time.Millisecond)
			i++
		}
	}
}
