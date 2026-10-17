// nooty.go — NootyCLI v0.3.0 "Titan" – Ultra-Fast, Anti-Sanction Agentic Terminal Intelligence
// Single-file pure Go. Zero external dependencies.
// Cross-platform: macOS (Apple Silicon / Intel), Linux, Windows, WSL.
//
// 🚀 Build with ultra-light optimization:
//   go build -ldflags="-s -w" -trimpath -o nooty nooty.go
//   upx --best --lzma nooty  (optional, compresses binary to < 4MB)

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
	"syscall"
	"time"
)

// ============================================================================
// ANSI Color & Styling Engine
// ============================================================================

var (
	useColor   = true
	termWidth  = 80
	appVersion = "v0.3.0 Titan"
)

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
	ansiReset      = "\033[0m"
	ansiBold       = "\033[1m"
	ansiDim        = "\033[2m"
	ansiItalic     = "\033[3m"
	ansiUnderline  = "\033[4m"
	ansiBlack      = "\033[30m"
	ansiRed        = "\033[31m"
	ansiGreen      = "\033[32m"
	ansiYellow     = "\033[33m"
	ansiBlue       = "\033[34m"
	ansiMagenta    = "\033[35m"
	ansiCyan       = "\033[36m"
	ansiWhite      = "\033[37m"
	ansiBrightCyan = "\033[96m"
)

// ============================================================================
// Data Models & Presets
// ============================================================================

type Config struct {
	ProviderName     string            `json:"provider_name"`
	ProviderEndpoint string            `json:"provider_endpoint"`
	APIKey           string            `json:"api_key"`
	Model            string            `json:"model"`
	Safety           string            `json:"safety"`   // "strict", "balanced", "permissive"
	Mode             string            `json:"mode"`     // "chat", "cli", "auto"
	Workspace        string            `json:"workspace"`
	ProxyURL         string            `json:"proxy_url"` // "socks5://127.0.0.1:10808", "http://...", etc.
	CustomHeaders    map[string]string `json:"custom_headers,omitempty"`
}

type ProviderPreset struct {
	ID          string
	Name        string
	Endpoint    string
	DefaultMod  string
	EnvVar      string
	Description string
	IsFreeTier  bool
}

var providerPresets = []ProviderPreset{
	{
		ID:          "groq",
		Name:        "Groq Cloud (Ultra-Fast & Free Tier)",
		Endpoint:    "https://api.groq.com/openai/v1",
		DefaultMod:  "llama-3.3-70b-versatile",
		EnvVar:      "GROQ_API_KEY",
		Description: "500-1000 tokens/sec. High quality Llama 3.3 and Qwen models.",
		IsFreeTier:  true,
	},
	{
		ID:          "gemini",
		Name:        "Google Gemini (Official OpenAI-Compatible)",
		Endpoint:    "https://generativelanguage.googleapis.com/v1beta/openai",
		DefaultMod:  "gemini-2.5-flash",
		EnvVar:      "GEMINI_API_KEY",
		Description: "1M token context, high intelligence, generous free tier.",
		IsFreeTier:  true,
	},
	{
		ID:          "deepseek",
		Name:        "DeepSeek API (Affordable & High Intelligence)",
		Endpoint:    "https://api.deepseek.com/v1",
		DefaultMod:  "deepseek-chat",
		EnvVar:      "DEEPSEEK_API_KEY",
		Description: "DeepSeek V3 / R1 reasoning. Extremely low cost.",
		IsFreeTier:  false,
	},
	{
		ID:          "openrouter",
		Name:        "OpenRouter (Aggregator with Free & Paid Models)",
		Endpoint:    "https://openrouter.ai/api/v1",
		DefaultMod:  "meta-llama/llama-3.3-70b-instruct:free",
		EnvVar:      "OPENROUTER_API_KEY",
		Description: "Unified gateway with dozens of free models and all top providers.",
		IsFreeTier:  true,
	},
	{
		ID:          "ollama",
		Name:        "Ollama (100% Offline, Local & Zero-Internet)",
		Endpoint:    "http://localhost:11434/v1",
		DefaultMod:  "qwen2.5-coder:7b",
		EnvVar:      "",
		Description: "Runs locally on your machine. Privacy-first, zero cost.",
		IsFreeTier:  true,
	},
	{
		ID:          "openai",
		Name:        "OpenAI (Direct Official API)",
		Endpoint:    "https://api.openai.com/v1",
		DefaultMod:  "gpt-4o-mini",
		EnvVar:      "OPENAI_API_KEY",
		Description: "Industry standard GPT-4o, GPT-4o-mini, o3-mini models.",
		IsFreeTier:  false,
	},
	{
		ID:          "custom",
		Name:        "Custom OpenAI-Compatible Endpoint",
		Endpoint:    "",
		DefaultMod:  "",
		EnvVar:      "CUSTOM_API_KEY",
		Description: "Any self-hosted vLLM, LM-Studio, Cloudflare AI Gateway, or local server.",
		IsFreeTier:  true,
	},
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
	Name         string            `json:"name"`
	Args         map[string]string `json:"args"`
	SearchBlock  string            `json:"search_block,omitempty"`
	ReplaceBlock string            `json:"replace_block,omitempty"`
}

type DNSResolver struct {
	Name    string
	Address string
}

// ============================================================================
// Global State
// ============================================================================

var (
	config          Config
	memories        []Memory
	sessionMessages []Message
	workspace       string
	homeDir         string
	nootyDir        string
	memFile         string
	configFile      string

	fallbackDNS = []DNSResolver{
		{Name: "Direct Connection", Address: ""},
		{Name: "Electro DNS", Address: "78.157.42.100"},
		{Name: "Shecan Primary", Address: "178.22.122.100"},
		{Name: "Shecan Secondary", Address: "185.51.200.2"},
		{Name: "Begzar Primary", Address: "185.55.226.26"},
		{Name: "Begzar Secondary", Address: "185.55.225.25"},
		{Name: "403.online", Address: "10.202.10.202"},
		{Name: "Cloudflare DNS", Address: "1.1.1.1"},
		{Name: "Google DNS", Address: "8.8.8.8"},
	}
	activeDNSName = "Direct Connection"
	clientCache   sync.Map
)

// ============================================================================
// Entrypoint & Initialization
// ============================================================================

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "NootyCLI %s — Ultra-Fast Anti-Sanction Agentic Terminal Intelligence\n\nUsage:\n  nooty [options]\n\nOptions:\n", appVersion)
		flag.PrintDefaults()
	}
	flag.Parse()

	setupSignalHandling()

	var err error
	homeDir, err = os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	nootyDir = filepath.Join(homeDir, ".nooty")
	_ = os.MkdirAll(nootyDir, 0700)
	_ = os.MkdirAll(filepath.Join(nootyDir, "chats"), 0700)
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

	drawHeader()
	repl()
}

func setupSignalHandling() {
	cChan := make(chan os.Signal, 1)
	signal.Notify(cChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		for range cChan {
			fmt.Printf("\n%s👋 Process interrupted. Goodbye!%s\n", c(ansiDim), c(ansiReset))
			os.Exit(0)
		}
	}()
}

// ============================================================================
// Minimalist Terminal UI
// ============================================================================

func drawHeader() {
	width := 68
	line := strings.Repeat("─", width-2)

	fmt.Println(c(ansiCyan) + "┌" + line + "┐" + c(ansiReset))
	fmt.Printf("%s│%s%s%s│%s\n", c(ansiCyan), c(ansiBold)+c(ansiYellow), centerText("⚡ NOOTY CLI ⚡", width-2), c(ansiCyan), c(ansiReset))
	fmt.Printf("%s│%s%s%s│%s\n", c(ansiCyan), c(ansiDim), centerText(appVersion+" — Agentic Terminal Intelligence", width-2), c(ansiCyan), c(ansiReset))
	fmt.Println(c(ansiCyan) + "├" + line + "┤" + c(ansiReset))

	prettyWorkspace := formatPath(workspace)
	proxyDisplay := config.ProxyURL
	if proxyDisplay == "" {
		if p := os.Getenv("ALL_PROXY"); p != "" {
			proxyDisplay = p + " (env)"
		} else if p := os.Getenv("HTTP_PROXY"); p != "" {
			proxyDisplay = p + " (env)"
		} else {
			proxyDisplay = "Direct (none)"
		}
	}

	entries := [][]string{
		{"Provider", config.ProviderName},
		{"Endpoint", truncateString(config.ProviderEndpoint, 40)},
		{"Model", config.Model},
		{"API Key", maskAPIKey(config.APIKey)},
		{"DNS Shield", activeDNSName},
		{"Proxy", truncateString(proxyDisplay, 40)},
		{"Safety", strings.ToUpper(config.Safety)},
		{"Mode", strings.ToUpper(config.Mode) + " Mode"},
		{"Workspace", truncateString(prettyWorkspace, 40)},
	}

	for _, e := range entries {
		val := fmt.Sprintf("%-40s", e[1])
		fmt.Printf("%s│%s %-12s: %s%s %s│%s\n",
			c(ansiCyan), c(ansiBold)+c(ansiWhite), e[0], c(ansiGreen), val, c(ansiCyan), c(ansiReset))
	}
	fmt.Println(c(ansiCyan) + "└" + line + "┘" + c(ansiReset))
	fmt.Printf("%s💡 Commands: %s/help%s | %s/provider%s | %s/mode cli%s | %s/map%s | %s/doctor%s\n\n",
		c(ansiDim),
		c(ansiBold)+c(ansiGreen), c(ansiDim),
		c(ansiBold)+c(ansiCyan), c(ansiDim),
		c(ansiBold)+c(ansiYellow), c(ansiDim),
		c(ansiBold)+c(ansiMagenta), c(ansiDim),
		c(ansiBold)+c(ansiGreen), c(ansiReset),
	)
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
		return "(not configured / free)"
	}
	if len(key) <= 8 {
		return key[:1] + strings.Repeat("*", len(key)-1)
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// ============================================================================
// Interactive REPL
// ============================================================================

func repl() {
	scanner := bufio.NewScanner(os.Stdin)
	const maxCapacity = 2 * 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

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
		} else if strings.HasPrefix(line, "!") {
			handleShellBang(line[1:])
		} else {
			handleChat(line)
		}
	}
	fmt.Println(c(ansiDim) + "\n👋 NootyCLI session finished. Have a productive day!" + c(ansiReset))
}

func prompt() string {
	switch config.Mode {
	case "auto":
		return c(ansiBold) + c(ansiMagenta) + "⚡ nooty[auto]" + c(ansiYellow) + " ❯ " + c(ansiReset)
	case "cli":
		return c(ansiBold) + c(ansiCyan) + "🤖 nooty[agent]" + c(ansiYellow) + " ❯ " + c(ansiReset)
	default:
		return c(ansiBold) + c(ansiGreen) + "💬 nooty" + c(ansiWhite) + " ❯ " + c(ansiReset)
	}
}

// ============================================================================
// Command Dispatcher
// ============================================================================

func handleSlashCommand(cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "/help":
		printHelp()
	case "/provider":
		handleProviderWizard()
	case "/mode":
		handleMode(parts[1:])
	case "/model":
		handleModelCommand(parts[1:])
	case "/workspace":
		handleWorkspace(parts[1:])
	case "/map":
		printRepoMapInteractive()
	case "/dns":
		handleDNSCommand(parts[1:])
	case "/proxy":
		handleProxyCommand(parts[1:])
	case "/doctor":
		runDoctor()
	case "/safety":
		handleSafety(parts[1:])
	case "/memory":
		handleMemory(parts[1:])
	case "/history":
		showHistory()
	case "/config":
		handleManualConfig()
	case "/clear":
		clearScreen()
	case "/exit", "/quit":
		fmt.Println(c(ansiDim) + "Exiting NootyCLI..." + c(ansiReset))
		os.Exit(0)
	default:
		fmt.Printf("%s❌ Unknown command: %s. Type /help for assistance.%s\n", c(ansiRed), parts[0], c(ansiReset))
	}
}

func printHelp() {
	fmt.Println(c(ansiBold) + "\n📖 NootyCLI v0.3.0 Command Reference:" + c(ansiReset))
	fmt.Println(`
  /provider                    Switch AI provider (Groq, Gemini, DeepSeek, OpenRouter, Ollama, OpenAI)
  /model [list|set <id>|show]  Browse available models from provider or set active model
  /mode [chat|cli|auto]        Toggle mode:
                                - chat: Conversational terminal assistance
                                - cli: Agentic Plan & Execute with manual step confirmation
                                - auto: Autonomous agent mode with automated compiler checking
  /map                         Generate instant zero-dependency architectural map of workspace
  /dns [show|test]             Inspect and benchmark anti-sanction DNS fallback chain
  /proxy [show|set <url>|off]  Configure SOCKS5/HTTP proxy for geo-blocked providers
  /workspace [show|set <path>] View or switch the active working project directory
  /doctor                      Full connectivity, latency, DNS shield & API diagnostic
  /safety [strict|balanced|permissive] Configure confirmation policies for file writes/commands
  /memory [list|add|forget]    Manage persistent long-term memories and project context
  /history                     Show session interaction log
  /config                      Manually configure API keys and endpoints
  /clear                       Clear screen and reset conversation history
  /exit                        Exit NootyCLI

  💡 Direct Execution: Prefix any command with ! (e.g., !git status, !ls -la) to run it directly.`)
}

func clearScreen() {
	sessionMessages = nil
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	} else {
		fmt.Print("\033[H\033[2J")
	}
	drawHeader()
	fmt.Println(c(ansiGreen) + "✨ Session history & screen cleared." + c(ansiReset))
}

func handleMode(args []string) {
	if len(args) == 0 {
		fmt.Printf("Current mode: %s\nUsage: /mode chat | cli | auto\n", strings.ToUpper(config.Mode))
		return
	}
	m := strings.ToLower(args[0])
	switch m {
	case "chat":
		config.Mode = "chat"
		fmt.Println(c(ansiGreen) + "💬 Switched to Conversational Chat Mode." + c(ansiReset))
	case "cli", "agent":
		config.Mode = "cli"
		fmt.Println(c(ansiCyan) + "🤖 Switched to Agent Mode (Interactive Plan & Execute)." + c(ansiReset))
	case "auto":
		config.Mode = "auto"
		fmt.Println(c(ansiMagenta) + "⚡ Switched to Autonomous Mode (Auto-Approve Safe Actions + Auto-Compile)." + c(ansiReset))
	default:
		fmt.Println("Usage: /mode chat | cli | auto")
		return
	}
	saveConfig()
}

func handleSafety(args []string) {
	if len(args) == 0 {
		fmt.Printf("🛡️ Safety Level: %s\nUsage: /safety strict | balanced | permissive\n", config.Safety)
		return
	}
	lvl := strings.ToLower(args[0])
	if lvl == "strict" || lvl == "balanced" || lvl == "permissive" {
		config.Safety = lvl
		saveConfig()
		fmt.Printf("%s✅ Safety updated to %s.%s\n", c(ansiGreen), strings.ToUpper(config.Safety), c(ansiReset))
	} else {
		fmt.Println("Usage: /safety strict | balanced | permissive")
	}
}

// ============================================================================
// Provider Preset Wizard
// ============================================================================

func handleProviderWizard() {
	fmt.Println(c(ansiBold) + "\n🌐 AI Provider Selection Wizard" + c(ansiReset))
	fmt.Println("Choose an optimized provider preset:\n")

	for i, p := range providerPresets {
		freeTag := ""
		if p.IsFreeTier {
			freeTag = c(ansiGreen) + " [FREE TIER AVAILABLE]" + c(ansiReset)
		}
		activeTag := ""
		if config.ProviderEndpoint == p.Endpoint || (p.ID == "custom" && config.ProviderName == "Custom") {
			activeTag = c(ansiCyan) + " (Active)" + c(ansiReset)
		}

		fmt.Printf("  %s[%d]%s %s%s%s\n", c(ansiBold)+c(ansiYellow), i+1, c(ansiReset), p.Name, freeTag, activeTag)
		fmt.Printf("      %s%s%s\n", c(ansiDim), p.Description, c(ansiReset))
	}

	fmt.Print("\nSelect provider number [1-7] or [c]ancel: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "c" || input == "" {
		fmt.Println("Cancelled.")
		return
	}

	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(providerPresets) {
		fmt.Println("❌ Invalid choice.")
		return
	}

	selected := providerPresets[idx-1]
	config.ProviderName = selected.Name

	if selected.ID == "custom" {
		fmt.Print("Enter custom API base URL (e.g. http://localhost:8000/v1): ")
		endpoint, _ := reader.ReadString('\n')
		config.ProviderEndpoint = strings.TrimSpace(endpoint)

		fmt.Print("Enter API Key (press enter if none): ")
		key, _ := reader.ReadString('\n')
		config.APIKey = strings.TrimSpace(key)

		fmt.Print("Enter default model identifier: ")
		mod, _ := reader.ReadString('\n')
		config.Model = strings.TrimSpace(mod)
	} else {
		config.ProviderEndpoint = selected.Endpoint
		config.Model = selected.DefaultMod

		envVal := ""
		if selected.EnvVar != "" {
			envVal = os.Getenv(selected.EnvVar)
		}

		if envVal != "" {
			config.APIKey = envVal
			fmt.Printf("%s✔ Using API key detected from environment variable $%s%s\n", c(ansiGreen), selected.EnvVar, c(ansiReset))
		} else if selected.ID == "ollama" {
			config.APIKey = ""
			fmt.Println(c(ansiGreen) + "✔ Ollama does not require an API key." + c(ansiReset))
		} else {
			fmt.Printf("Enter API key for %s [%s]: ", selected.Name, maskAPIKey(config.APIKey))
			key, _ := reader.ReadString('\n')
			key = strings.TrimSpace(key)
			if key != "" {
				config.APIKey = key
			}
		}
	}

	saveConfig()
	fmt.Printf("\n%s✅ Provider set to %s (%s)%s\n\n", c(ansiGreen), config.ProviderName, config.Model, c(ansiReset))
}

func handleManualConfig() {
	fmt.Println(c(ansiBold) + "\n⚙️ Manual Configuration" + c(ansiReset))
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
	fmt.Println(c(ansiGreen) + "✅ Settings saved successfully!\n" + c(ansiReset))
}

// ============================================================================
// Model & Workspace Handlers
// ============================================================================

func handleModelCommand(args []string) {
	if len(args) == 0 || args[0] == "show" {
		fmt.Printf("🤖 Active Model: %s%s%s (Provider: %s)\n", c(ansiBold)+c(ansiGreen), config.Model, c(ansiReset), config.ProviderName)
		return
	}
	switch args[0] {
	case "set":
		if len(args) < 2 {
			fmt.Println("Usage: /model set <model-id>")
			return
		}
		config.Model = args[1]
		saveConfig()
		fmt.Printf("✅ Active model set to: %s\n", config.Model)
	case "list":
		selectModelInteractive()
	default:
		fmt.Println("Usage: /model show | set <id> | list")
	}
}

func selectModelInteractive() {
	fmt.Println("🔍 Fetching available models from provider...")
	models, err := fetchAvailableModels()
	if err != nil {
		fmt.Printf("%s❌ Model list error: %v%s\n", c(ansiRed), err, c(ansiReset))
		return
	}
	if len(models) == 0 {
		fmt.Println("⚠️ Provider returned an empty list of models.")
		return
	}

	sort.Strings(models)

	pageSize := 15
	totalPages := (len(models) + pageSize - 1) / pageSize
	page := 0

	for {
		fmt.Printf("\n%s📋 Available Models (Page %d/%d, Total: %d):%s\n", c(ansiBold), page+1, totalPages, len(models), c(ansiReset))
		start := page * pageSize
		end := start + pageSize
		if end > len(models) {
			end = len(models)
		}

		for i, m := range models[start:end] {
			activeTag := ""
			if m == config.Model {
				activeTag = c(ansiGreen) + " (Active)" + c(ansiReset)
			}
			fmt.Printf("  %s[%2d]%s %s%s\n", c(ansiBold)+c(ansiCyan), start+i+1, c(ansiReset), m, activeTag)
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
			fmt.Printf("%s✅ Active Model updated to: %s%s\n", c(ansiGreen), selected, c(ansiReset))
			return
		}
	}
}

func handleWorkspace(args []string) {
	if len(args) == 0 || args[0] == "show" {
		fmt.Printf("📁 Active Workspace: %s%s%s\n", c(ansiBold)+c(ansiCyan), formatPath(workspace), c(ansiReset))
		return
	}
	if args[0] == "set" {
		if len(args) < 2 {
			fmt.Println("Usage: /workspace set <path>")
			return
		}
		path, _ := filepath.Abs(args[1])
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			fmt.Println("❌ Specified directory does not exist.")
			return
		}
		workspace = path
		config.Workspace = path
		saveConfig()
		fmt.Printf("✅ Workspace updated to: %s\n", formatPath(workspace))
	} else {
		fmt.Println("Usage: /workspace show | set <path>")
	}
}

// ============================================================================
// Network, DNS Shield & Proxy Engine
// ============================================================================

func handleDNSCommand(args []string) {
	if len(args) > 0 && args[0] == "test" {
		benchmarkDNSResolvers()
		return
	}

	fmt.Println(c(ansiBold) + "\n🛡️ Anti-Sanction Smart DNS Shield Chain:" + c(ansiReset))
	for i, dns := range fallbackDNS {
		status := ""
		if dns.Name == activeDNSName {
			status = c(ansiGreen) + " [ACTIVE]" + c(ansiReset)
		}
		if dns.Address == "" {
			fmt.Printf("  %d. %-24s (System Default)%s\n", i+1, dns.Name, status)
		} else {
			fmt.Printf("  %d. %-24s (%s)%s\n", i+1, dns.Name, dns.Address, status)
		}
	}
	fmt.Println(c(ansiDim) + "💡 Run '/dns test' to benchmark response latency." + c(ansiReset) + "\n")
}

func handleProxyCommand(args []string) {
	if len(args) == 0 || args[0] == "show" {
		p := config.ProxyURL
		if p == "" {
			p = "None (Direct connection)"
		}
		fmt.Printf("🌐 Configured Proxy: %s%s%s\n", c(ansiBold)+c(ansiCyan), p, c(ansiReset))
		return
	}
	if args[0] == "off" || args[0] == "clear" {
		config.ProxyURL = ""
		saveConfig()
		fmt.Println(c(ansiGreen) + "✅ Proxy disabled. Direct connections enabled." + c(ansiReset))
		return
	}
	if args[0] == "set" {
		if len(args) < 2 {
			fmt.Println("Usage: /proxy set socks5://127.0.0.1:10808 or http://127.0.0.1:7890")
			return
		}
		raw := args[1]
		_, err := url.Parse(raw)
		if err != nil {
			fmt.Printf("❌ Invalid proxy URL: %v\n", err)
			return
		}
		config.ProxyURL = raw
		saveConfig()
		fmt.Printf("%s✅ Proxy set to: %s%s\n", c(ansiGreen), raw, c(ansiReset))
	} else {
		fmt.Println("Usage: /proxy show | set <url> | off")
	}
}

func benchmarkDNSResolvers() {
	fmt.Println(c(ansiBold) + "\n⏱️ Benchmarking DNS Resolvers against api.github.com..." + c(ansiReset))
	targetHost := "api.github.com"

	for _, resolver := range fallbackDNS {
		start := time.Now()
		var ips []net.IP
		var err error

		if resolver.Address == "" {
			ips, err = net.LookupIP(targetHost)
		} else {
			r := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{Timeout: 3 * time.Second}
					return d.DialContext(ctx, "udp", resolver.Address+":53")
				},
			}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			ips, err = r.LookupIP(ctx, "ip4", targetHost)
			cancel()
		}

		elapsed := time.Since(start)
		if err != nil {
			fmt.Printf("  ❌ %-22s: Failed (%v)\n", resolver.Name, err)
		} else {
			fmt.Printf("  %s✔%s  %-22s: %4d ms (Resolved %d IPs)\n",
				c(ansiGreen), c(ansiReset), resolver.Name, elapsed.Milliseconds(), len(ips))
		}
	}
	fmt.Println()
}

func getHTTPClient(dnsServer, proxyURL string) *http.Client {
	cacheKey := dnsServer + "|" + proxyURL
	if val, ok := clientCache.Load(cacheKey); ok {
		return val.(*http.Client)
	}

	var proxyFunc func(*http.Request) (*url.URL, error)
	if proxyURL != "" {
		parsedProxy, err := url.Parse(proxyURL)
		if err == nil {
			proxyFunc = http.ProxyURL(parsedProxy)
		}
	} else {
		proxyFunc = http.ProxyFromEnvironment
	}

	dialer := &net.Dialer{
		Timeout:   20 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	if dnsServer != "" {
		dialer.Resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, "udp", dnsServer+":53")
			},
		}
	}

	transport := &http.Transport{
		Proxy:                 proxyFunc,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second,
	}

	clientCache.Store(cacheKey, client)
	return client
}

func doWithFallback(method, targetURL string, body []byte, headers map[string]string) (*http.Response, error) {
	client := getHTTPClient(getActiveDNSAddress(), config.ProxyURL)
	resp, err := performRequest(client, method, targetURL, body, headers)
	if err == nil && isGoodResponse(resp) {
		return resp, nil
	}
	if resp != nil {
		_ = resp.Body.Close()
	}

	for _, resolver := range fallbackDNS {
		if resolver.Name == activeDNSName {
			continue
		}

		client = getHTTPClient(resolver.Address, config.ProxyURL)
		resp, err = performRequest(client, method, targetURL, body, headers)
		if err == nil && isGoodResponse(resp) {
			activeDNSName = resolver.Name
			fmt.Printf("%s🛡️ Anti-sanction route established via %s%s\n", c(ansiGreen), resolver.Name, c(ansiReset))
			return resp, nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	}

	if config.ProxyURL == "" {
		localProbingPorts := []string{"10808", "7890", "2080", "1080"}
		for _, port := range localProbingPorts {
			testProxy := "socks5://127.0.0.1:" + port
			client = getHTTPClient("", testProxy)
			resp, err = performRequest(client, method, targetURL, body, headers)
			if err == nil && isGoodResponse(resp) {
				config.ProxyURL = testProxy
				saveConfig()
				fmt.Printf("%s🛡️ Auto-detected active local proxy at %s! Saved to config.%s\n", c(ansiGreen), testProxy, c(ansiReset))
				return resp, nil
			}
			if resp != nil {
				_ = resp.Body.Close()
			}
		}
	}

	return nil, fmt.Errorf("all connection routes exhausted (HTTP 403/DNS block/timeout)")
}

func isGoodResponse(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	if resp.StatusCode == 403 || resp.StatusCode == 451 || resp.StatusCode == 502 {
		return false
	}
	return true
}

func getActiveDNSAddress() string {
	for _, r := range fallbackDNS {
		if r.Name == activeDNSName {
			return r.Address
		}
	}
	return ""
}

func performRequest(client *http.Client, method, targetURL string, body []byte, headers map[string]string) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, targetURL, reader)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	for k, v := range config.CustomHeaders {
		req.Header.Set(k, v)
	}

	return client.Do(req)
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
		return nil, fmt.Errorf("failed to parse models json")
	}

	var models []string
	for _, d := range result.Data {
		models = append(models, d.ID)
	}
	return models, nil
}

// ============================================================================
// Zero-Dependency Mini Repo-Map (Project Outline Generator)
// ============================================================================

func printRepoMapInteractive() {
	fmt.Printf("%s🗺️ Scanning workspace (%s) for symbols...%s\n", c(ansiYellow), formatPath(workspace), c(ansiReset))
	outline := generateRepoMap(workspace)
	if outline == "" {
		fmt.Println("No source code declarations detected in workspace.")
		return
	}
	fmt.Println(c(ansiBold) + c(ansiCyan) + "\n=== Workspace Architectural Skeleton ===" + c(ansiReset))
	fmt.Println(outline)
	fmt.Println(c(ansiDim) + "========================================" + c(ansiReset) + "\n")
}

func generateRepoMap(root string) string {
	var sb strings.Builder
	fileCount := 0

	reGo := regexp.MustCompile(`(?m)^(func\s+(?:\([^)]+\)\s+)?\w+|type\s+\w+\s+(?:struct|interface))`)
	rePy := regexp.MustCompile(`(?m)^(def\s+\w+|class\s+\w+)`)
	reJS := regexp.MustCompile(`(?m)^(export\s+(?:default\s+)?(?:class|function|const|let)\s+\w+|function\s+\w+|class\s+\w+|interface\s+\w+|type\s+\w+)`)
	reRs := regexp.MustCompile(`(?m)^(pub\s+(?:fn|struct|enum|trait|type)\s+\w+|fn\s+\w+|struct\s+\w+)`)

	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || fileCount >= 40 {
			return nil
		}
		if info.IsDir() {
			if isIgnoredDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Size() > 500_000 || info.Size() == 0 {
			return nil
		}

		ext := filepath.Ext(p)
		var targetRe *regexp.Regexp

		switch ext {
		case ".go":
			targetRe = reGo
		case ".py":
			targetRe = rePy
		case ".js", ".ts", ".jsx", ".tsx":
			targetRe = reJS
		case ".rs":
			targetRe = reRs
		default:
			return nil
		}

		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}

		matches := targetRe.FindAllString(string(data), 15)
		if len(matches) > 0 {
			rel, _ := filepath.Rel(root, p)
			sb.WriteString(fmt.Sprintf("\n📄 %s:\n", rel))
			for _, m := range matches {
				sb.WriteString(fmt.Sprintf("   • %s\n", strings.TrimSpace(m)))
			}
			fileCount++
		}

		return nil
	})

	return sb.String()
}

func isIgnoredDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "build", ".idea", ".vscode", "__pycache__", ".venv", "venv", "target":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// ============================================================================
// Chat & Agent Loop Execution
// ============================================================================

func handleChat(input string) {
	messages := buildMessages(input)

	if config.Mode == "cli" || config.Mode == "auto" {
		runAgentLoop(messages)
		return
	}

	streamResponse(messages)
}

func buildMessages(userInput string) []Message {
	var msgs []Message

	sysPrompt := fmt.Sprintf(`You are NootyCLI %s, an elite autonomous terminal software engineering agent.
Working directory: %s

CAPABILITIES & PROTOCOL:
When in CHAT mode: Give concise, mathematically rigorous and expert software engineering answers.

When in CLI / AGENT mode:
You act as an autonomous workspace agent. To perform actions, you MUST issue tools using EXACTLY this syntax:

TOOL: tool_name key1="value1" key2="value2"

CRITICAL FOR CODE MODIFICATIONS:
Prefer 'replace_in_file' over 'write_file' for existing files!
'replace_in_file' requires: path, search, replace.
Example:
TOOL: replace_in_file path="main.go" search="port = 8080" replace="port = 3000"

AVAILABLE TOOLS:
- replace_in_file (path="relative_path", search="exact_existing_block", replace="new_block")
- write_file (path="relative_path", content="full_content")
- read_file (path="relative_path", start_line="1", end_line="100")
- list_files (path="relative_path")
- tree (path="relative_path", max_depth="3")
- search_code (query="text", path="relative_path")
- file_info (path="relative_path")
- delete_file (path="relative_path")
- git_status
- git_diff (path="relative_path")
- run_command (command="shell_command", timeout="seconds")
- get_repo_map (path=".")
- check_syntax (path="relative_path")

RULES:
1. Issue only ONE tool invocation per step.
2. Read files first if uncertain.
3. Once task is fully solved, state your final summary without calling any tools.`, appVersion, workspace)

	repoMap := generateRepoMap(workspace)
	if repoMap != "" && len(repoMap) < 2500 {
		sysPrompt += "\n\nWorkspace Structure Map:\n" + repoMap
	}

	relevant := getRelevantMemories(userInput)
	if len(relevant) > 0 {
		sysPrompt += "\n\nUser Context & Memories:\n"
		for _, m := range relevant {
			sysPrompt += fmt.Sprintf("- [%s] %s\n", m.Tag, m.Content)
		}
	}

	msgs = append(msgs, Message{Role: "system", Content: sysPrompt})

	histLimit := 12
	start := 0
	if len(sessionMessages) > histLimit {
		start = len(sessionMessages) - histLimit
	}
	msgs = append(msgs, sessionMessages[start:]...)

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

func streamResponse(messages []Message) {
	reqPayload := ChatRequest{Model: config.Model, Messages: messages, Stream: true}
	jsonData, _ := json.Marshal(reqPayload)
	endpoint := strings.TrimRight(config.ProviderEndpoint, "/") + "/chat/completions"
	headers := map[string]string{"Content-Type": "application/json"}
	if config.APIKey != "" {
		headers["Authorization"] = "Bearer " + config.APIKey
	}

	resp, err := doWithFallback("POST", endpoint, jsonData, headers)
	if err != nil {
		fmt.Printf("%s❌ Request error: %v%s\n", c(ansiRed), err, c(ansiReset))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("%s❌ Provider Error %d: %s%s\n", c(ansiRed), resp.StatusCode, string(body), c(ansiReset))
		return
	}

	reader := bufio.NewReader(resp.Body)
	var fullContent strings.Builder
	fmt.Print(c(ansiCyan))

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
				fmt.Print(choice.Delta.Content)
				fullContent.WriteString(choice.Delta.Content)
			}
		}
	}

	fmt.Print(c(ansiReset) + "\n\n")
	sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: fullContent.String()})
}

// ============================================================================
// Agentic Plan & Execute Loop with Compiler-in-the-Loop
// ============================================================================

func runAgentLoop(messages []Message) {
	isAuto := (config.Mode == "auto")

	planPrompt := append(messages, Message{Role: "user", Content: "Formulate a concise, bulleted action plan to solve this request."})
	fmt.Print(c(ansiYellow) + "🤔 Analyzing workspace & architecting plan... " + c(ansiReset))

	planText, err := getModelResponseText(planPrompt)
	if err != nil {
		fmt.Printf("%s❌ Planning failed: %v%s\n", c(ansiRed), err, c(ansiReset))
		return
	}

	fmt.Println("\n" + c(ansiCyan) + c(ansiBold) + "📋 Proposed Execution Plan:" + c(ansiReset))
	fmt.Println(c(ansiCyan) + planText + c(ansiReset) + "\n")

	if !isAuto {
		fmt.Print(c(ansiBold) + "Approve execution plan? [Y/n]: " + c(ansiReset))
		reader := bufio.NewReader(os.Stdin)
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))
		if confirm == "n" || confirm == "no" {
			fmt.Println("🛑 Execution cancelled by user.")
			return
		}
	}

	msgs := append(messages,
		Message{Role: "assistant", Content: planText},
		Message{Role: "user", Content: "Plan approved. Proceed step by step using TOOL commands."},
	)

	maxSteps := 15
	for step := 1; step <= maxSteps; step++ {
		fmt.Printf("%s⏳ Agent step [%d/%d] thinking...%s\r", c(ansiDim), step, maxSteps, c(ansiReset))

		respText, err := getModelResponseText(msgs)
		if err != nil {
			fmt.Printf("\n%s❌ Agent Execution Error: %v%s\n", c(ansiRed), err, c(ansiReset))
			return
		}

		toolCall := extractToolCall(respText)
		if toolCall == nil {
			fmt.Printf("\n%s%s%s\n\n", c(ansiGreen), respText, c(ansiReset))
			sessionMessages = append(sessionMessages, Message{Role: "assistant", Content: respText})
			return
		}

		fmt.Printf("\n%s🔧 Action [%d]: %s%s%s\n", c(ansiBold)+c(ansiYellow), step, c(ansiBrightCyan), toolCall.Name, c(ansiReset))
		for k, v := range toolCall.Args {
			preview := v
			if len(preview) > 80 {
				preview = preview[:77] + "..."
			}
			preview = strings.ReplaceAll(preview, "\n", "\\n")
			fmt.Printf("   %s%s%s: %s\n", c(ansiDim), k, c(ansiReset), preview)
		}

		toolResult, approved := executeAgentTool(toolCall)
		if !approved {
			msgs = append(msgs,
				Message{Role: "assistant", Content: respText},
				Message{Role: "user", Content: "Action aborted by user safety policy."},
			)
			continue
		}

		compilerFeedback := ""
		if toolCall.Name == "replace_in_file" || toolCall.Name == "write_file" {
			targetPath := toolCall.Args["path"]
			if targetPath != "" {
				chkResult, _ := runTool("check_syntax", map[string]string{"path": targetPath})
				if strings.Contains(chkResult, "SYNTAX ERROR") || strings.Contains(chkResult, "COMPILE ERROR") {
					compilerFeedback = fmt.Sprintf("\n⚠️ COMPILER DETECTED ERRORS AFTER EDIT:\n%s\nPlease self-correct this file immediately.", chkResult)
					fmt.Printf("%s%s%s\n", c(ansiRed), compilerFeedback, c(ansiReset))
				}
			}
		}

		truncatedOutput := toolResult
		if len(truncatedOutput) > 3000 {
			truncatedOutput = truncatedOutput[:3000] + "\n... (output truncated for brevity)"
		}

		fmt.Printf("%s📄 Result:%s %s\n", c(ansiDim), c(ansiReset), strings.TrimSpace(truncatedOutput))

		feedback := fmt.Sprintf("Tool '%s' result:\n%s%s", toolCall.Name, toolResult, compilerFeedback)
		msgs = append(msgs,
			Message{Role: "assistant", Content: respText},
			Message{Role: "user", Content: feedback},
		)
	}

	fmt.Printf("%s⚠️ Agent reached max step threshold (%d steps).%s\n", c(ansiYellow), maxSteps, c(ansiReset))
}

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

// ============================================================================
// Multi-Format Tool Parser
// ============================================================================

func extractToolCall(text string) *ToolCall {
	if strings.Contains(text, "<<<<<<< SEARCH") && strings.Contains(text, ">>>>>>> REPLACE") {
		tc := parseBlockDiff(text)
		if tc != nil {
			return tc
		}
	}

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TOOL:") || strings.HasPrefix(line, "TOOL：") {
			return parseToolLine(line)
		}
	}

	reJSON := regexp.MustCompile(`(?s)\{(?:\s*"tool"|\s*"name")\s*:\s*"(\w+)"\s*,\s*"args"\s*:\s*(\{.*?\})\s*\}`)
	if m := reJSON.FindStringSubmatch(text); len(m) == 3 {
		args := map[string]string{}
		_ = json.Unmarshal([]byte(m[2]), &args)
		return &ToolCall{Name: m[1], Args: args}
	}

	re := regexp.MustCompile(`(?i)TOOL:\s*(\w+)\s+(.*)`)
	matches := re.FindStringSubmatch(text)
	if len(matches) >= 3 {
		return parseToolArgs(matches[1], matches[2])
	}

	return nil
}

func parseBlockDiff(text string) *ToolCall {
	reHead := regexp.MustCompile(`(?i)TOOL:\s*(?:replace_in_file|edit_file)\s+path=["']?([^"'\s]+)["']?`)
	headMatch := reHead.FindStringSubmatch(text)
	targetPath := ""
	if len(headMatch) >= 2 {
		targetPath = headMatch[1]
	}

	sIdx := strings.Index(text, "<<<<<<< SEARCH\n")
	eIdx := strings.Index(text, "\n=======\n")
	rIdx := strings.Index(text, "\n>>>>>>> REPLACE")

	if sIdx != -1 && eIdx != -1 && rIdx != -1 && sIdx < eIdx && eIdx < rIdx {
		searchStr := text[sIdx+len("<<<<<<< SEARCH\n") : eIdx]
		replaceStr := text[eIdx+len("\n=======\n") : rIdx]

		args := map[string]string{
			"path":    targetPath,
			"search":  searchStr,
			"replace": replaceStr,
		}
		return &ToolCall{Name: "replace_in_file", Args: args}
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
			val := match[2]
			if (strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`)) || (strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				val = val[1 : len(val)-1]
			}
			val = strings.ReplaceAll(val, "\\n", "\n")
			val = strings.ReplaceAll(val, "\\t", "\t")
			val = strings.ReplaceAll(val, `\"`, `"`)
			args[key] = val
		}
	}

	if len(args) == 0 && strings.TrimSpace(argsStr) != "" {
		args["path"] = strings.TrimSpace(argsStr)
	}

	return &ToolCall{Name: name, Args: args}
}

// ============================================================================
// Built-in Tool Implementations
// ============================================================================

func executeAgentTool(tc *ToolCall) (string, bool) {
	needsApproval := false

	switch tc.Name {
	case "delete_file":
		needsApproval = true
	case "run_command":
		if config.Safety == "strict" || config.Mode != "auto" {
			needsApproval = true
		}
	case "write_file", "create_file", "replace_in_file":
		if config.Safety == "strict" {
			needsApproval = true
		}
	}

	if needsApproval {
		if tc.Name == "delete_file" {
			fmt.Printf("%s⚠️ SAFETY WARNING: Permanently delete file '%s'?%s\n", c(ansiRed), tc.Args["path"], c(ansiReset))
			fmt.Print("Type 'DELETE' to confirm: ")
			reader := bufio.NewReader(os.Stdin)
			confirm, _ := reader.ReadString('\n')
			if strings.TrimSpace(confirm) != "DELETE" {
				return "Operation aborted by safety policy.", false
			}
		} else {
			fmt.Printf("Approve action '%s'? [Y/n]: ", tc.Name)
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

func runTool(name string, args map[string]string) (string, error) {
	switch name {

	case "replace_in_file":
		targetPath := safeJoin(workspace, args["path"])
		searchStr := args["search"]
		replaceStr := args["replace"]

		if searchStr == "" {
			return "", fmt.Errorf("search string cannot be empty")
		}

		data, err := os.ReadFile(targetPath)
		if err != nil {
			return "", fmt.Errorf("cannot read file: %v", err)
		}

		content := string(data)

		// 1. Try exact match
		if strings.Contains(content, searchStr) {
			count := strings.Count(content, searchStr)
			if count > 1 {
				return "", fmt.Errorf("search block appears %d times in file. Provide more surrounding context", count)
			}
			newContent := strings.Replace(content, searchStr, replaceStr, 1)
			if err := os.WriteFile(targetPath, []byte(newContent), 0644); err != nil {
				return "", err
			}
			return fmt.Sprintf("✅ Replaced %d bytes in '%s'", len(searchStr), args["path"]), nil
		}

		// 2. Try whitespace-normalized match
		normContent := strings.ReplaceAll(content, "\r\n", "\n")
		normSearch := strings.ReplaceAll(searchStr, "\r\n", "\n")

		if strings.Contains(normContent, normSearch) {
			newContent := strings.Replace(normContent, normSearch, replaceStr, 1)
			if err := os.WriteFile(targetPath, []byte(newContent), 0644); err != nil {
				return "", err
			}
			return fmt.Sprintf("✅ Replaced (with normalized line endings) in '%s'", args["path"]), nil
		}

		return "", fmt.Errorf("search block not found in '%s'. Verify exact line content", args["path"])

	case "write_file", "create_file":
		targetPath := safeJoin(workspace, args["path"])
		content := args["content"]
		_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
		err := os.WriteFile(targetPath, []byte(content), 0644)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ File written (%d bytes): %s", len(content), args["path"]), nil

	case "read_file":
		targetPath := safeJoin(workspace, args["path"])
		data, err := os.ReadFile(targetPath)
		if err != nil {
			return "", err
		}

		lines := strings.Split(string(data), "\n")
		startLine := 1
		endLine := len(lines)

		if s, ok := args["start_line"]; ok {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				startLine = n
			}
		}
		if e, ok := args["end_line"]; ok {
			if n, err := strconv.Atoi(e); err == nil && n >= startLine && n <= len(lines) {
				endLine = n
			}
		}

		var sb strings.Builder
		for i := startLine - 1; i < endLine; i++ {
			sb.WriteString(fmt.Sprintf("%4d | %s\n", i+1, lines[i]))
		}
		return sb.String(), nil

	case "list_files":
		targetPath := workspace
		if p, ok := args["path"]; ok && p != "" && p != "." {
			targetPath = safeJoin(workspace, p)
		}
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			return "", err
		}
		var names []string
		for _, e := range entries {
			if isIgnoredDir(e.Name()) {
				continue
			}
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

	case "tree":
		targetPath := workspace
		if p, ok := args["path"]; ok && p != "" && p != "." {
			targetPath = safeJoin(workspace, p)
		}
		maxDepth := 3
		if md, ok := args["max_depth"]; ok {
			if n, err := strconv.Atoi(md); err == nil {
				maxDepth = n
			}
		}
		return buildDirTree(targetPath, "", 0, maxDepth), nil

	case "search_code":
		query := args["query"]
		scope := workspace
		if s, ok := args["path"]; ok && s != "" {
			scope = safeJoin(workspace, s)
		}

		var results []string
		_ = filepath.Walk(scope, func(p string, info os.FileInfo, err error) error {
			if err != nil || len(results) >= 20 {
				return nil
			}
			if info.IsDir() {
				if isIgnoredDir(info.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if info.Size() > 1_000_000 {
				return nil
			}

			data, err := os.ReadFile(p)
			if err == nil {
				content := string(data)
				if strings.Contains(content, query) {
					rel, _ := filepath.Rel(workspace, p)
					lines := strings.Split(content, "\n")
					for lineNum, line := range lines {
						if strings.Contains(line, query) {
							results = append(results, fmt.Sprintf("%s:%d: %s", rel, lineNum+1, strings.TrimSpace(line)))
							if len(results) >= 20 {
								break
							}
						}
					}
				}
			}
			return nil
		})

		if len(results) == 0 {
			return "No matches found.", nil
		}
		return strings.Join(results, "\n"), nil

	case "check_syntax":
		targetPath := safeJoin(workspace, args["path"])
		ext := filepath.Ext(targetPath)
		var cmd *exec.Cmd

		switch ext {
		case ".go":
			cmd = exec.Command("go", "vet", targetPath)
		case ".py":
			cmd = exec.Command("python3", "-m", "py_compile", targetPath)
		case ".js", ".mjs":
			cmd = exec.Command("node", "--check", targetPath)
		case ".sh", ".bash":
			cmd = exec.Command("bash", "-n", targetPath)
		case ".json":
			data, err := os.ReadFile(targetPath)
			if err != nil {
				return fmt.Sprintf("COMPILE ERROR: %v", err), nil
			}
			if !json.Valid(data) {
				return "SYNTAX ERROR: Invalid JSON syntax", nil
			}
			return "✅ JSON syntax verified valid.", nil
		default:
			return "Syntax checking not configured for extension " + ext, nil
		}

		cmd.Dir = workspace
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("SYNTAX ERROR:\n%s", string(out)), nil
		}
		return "✅ Syntax clean. No errors found.", nil

	case "file_info":
		targetPath := safeJoin(workspace, args["path"])
		info, err := os.Stat(targetPath)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Path: %s\nSize: %d bytes\nMode: %s\nModified: %s",
			targetPath, info.Size(), info.Mode(), info.ModTime().Format(time.RFC3339)), nil

	case "delete_file":
		targetPath := safeJoin(workspace, args["path"])
		if err := os.Remove(targetPath); err != nil {
			return "", err
		}
		return "✅ File removed: " + args["path"], nil

	case "git_status":
		cmd := exec.Command("git", "status", "--short")
		cmd.Dir = workspace
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git error: %v", err)
		}
		s := string(out)
		if s == "" {
			s = "(clean working directory)"
		}
		return s, nil

	case "git_diff":
		cmd := exec.Command("git", "diff")
		if p, ok := args["path"]; ok && p != "" {
			cmd = exec.Command("git", "diff", p)
		}
		cmd.Dir = workspace
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git error: %v", err)
		}
		s := string(out)
		if s == "" {
			s = "(no uncommitted changes)"
		}
		return s, nil

	case "get_repo_map":
		return generateRepoMap(workspace), nil

	case "run_command":
		cmdStr := args["command"]
		timeout := 60
		if t, ok := args["timeout"]; ok {
			_, _ = fmt.Sscanf(t, "%d", &timeout)
		}

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", cmdStr)
		} else {
			cmd = exec.Command("sh", "-c", cmdStr)
		}

		cmd.Dir = workspace
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf

		if err := cmd.Start(); err != nil {
			return "", err
		}

		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		select {
		case err := <-done:
			out := outBuf.String()
			if errBuf.Len() > 0 {
				out += "\n[stderr]\n" + errBuf.String()
			}
			if err != nil {
				return out + fmt.Sprintf("\n(Exit code: %v)", err), nil
			}
			if out == "" {
				out = "(command executed successfully with no output)"
			}
			return out, nil
		case <-time.After(time.Duration(timeout) * time.Second):
			_ = cmd.Process.Kill()
			return "", fmt.Errorf("command execution timed out (%d sec)", timeout)
		}
	}

	return "", fmt.Errorf("unknown tool: %s", name)
}

func buildDirTree(root, indent string, currentDepth, maxDepth int) string {
	if currentDepth > maxDepth {
		return ""
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}

	var out string
	validEntries := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		if !isIgnoredDir(e.Name()) {
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
			out += buildDirTree(filepath.Join(root, e.Name()), childIndent, currentDepth+1, maxDepth)
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

// ============================================================================
// Direct Shell Engine
// ============================================================================

func handleShellBang(cmd string) {
	cmd = strings.TrimSpace(cmd)
	fmt.Printf("\n%s⚡ Direct Shell:%s %s\n", c(ansiYellow), c(ansiReset), cmd)
	fmt.Print("Execute command? [Y/n]: ")
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
		fmt.Printf("%s❌ Command exited with error: %v%s\n", c(ansiRed), err, c(ansiReset))
	}
}

// ============================================================================
// Diagnostics Doctor
// ============================================================================

func runDoctor() {
	fmt.Println(c(ansiBold) + "\n🏥 NootyCLI Diagnostic Doctor" + c(ansiReset))
	fmt.Printf("• App Version       : %s\n", appVersion)
	fmt.Printf("• Go Runtime        : %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("• Workspace         : %s\n", formatPath(workspace))
	fmt.Printf("• Active Provider   : %s\n", config.ProviderName)
	fmt.Printf("• Endpoint          : %s\n", config.ProviderEndpoint)
	fmt.Printf("• Active Model      : %s\n", config.Model)
	fmt.Printf("• Proxy Configured  : %s\n", config.ProxyURL)
	fmt.Printf("• DNS Resolver      : %s (%s)\n", activeDNSName, getActiveDNSAddress())

	fmt.Print("• Provider API Check: ")
	models, err := fetchAvailableModels()
	if err != nil {
		fmt.Printf("%sFAILED (%v)%s\n", c(ansiRed), err, c(ansiReset))
		fmt.Printf("  %s💡 Tip: Try '/provider' to switch to Groq or Gemini, or check '/proxy set'.%s\n", c(ansiYellow), c(ansiReset))
	} else {
		fmt.Printf("%sOK (%d models accessible)%s\n", c(ansiGreen), len(models), c(ansiReset))
	}

	fmt.Print("• Git Status Check  : ")
	gStatus, gErr := runTool("git_status", nil)
	if gErr != nil {
		fmt.Printf("%sWarning (not a git repo)%s\n", c(ansiYellow), c(ansiReset))
	} else {
		fmt.Printf("%sOK (%s)%s\n", c(ansiGreen), strings.TrimSpace(gStatus), c(ansiReset))
	}
	fmt.Println()
}

// ============================================================================
// Memory & History
// ============================================================================

func handleMemory(args []string) {
	if len(args) == 0 || args[0] == "list" {
		if len(memories) == 0 {
			fmt.Println("🧠 No persistent memories stored.")
			return
		}
		fmt.Println(c(ansiBold) + "\n🧠 Persistent Memories:" + c(ansiReset))
		for _, m := range memories {
			fmt.Printf("  [%d] (%s) %s\n", m.ID, m.Tag, m.Content)
		}
		fmt.Println()
		return
	}

	switch args[0] {
	case "add":
		if len(args) < 2 {
			fmt.Println("Usage: /memory add <context-note>")
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
		fmt.Printf("✅ Saved memory [%d]: %s\n", m.ID, text)

	case "forget":
		if len(args) < 2 {
			fmt.Println("Usage: /memory forget <id>")
			return
		}
		id, _ := strconv.Atoi(args[1])
		var filtered []Memory
		for _, m := range memories {
			if m.ID != id {
				filtered = append(filtered, m)
			}
		}
		memories = filtered
		saveMemories()
		fmt.Printf("✅ Memory [%d] removed.\n", id)

	case "clear":
		memories = []Memory{}
		saveMemories()
		fmt.Println("✅ All memories cleared.")
	}
}

func showHistory() {
	if len(sessionMessages) == 0 {
		fmt.Println("💬 History is empty.")
		return
	}
	fmt.Println(c(ansiBold) + "\n📜 Session Log:" + c(ansiReset))
	for _, msg := range sessionMessages {
		role := "👤 User"
		if msg.Role == "assistant" {
			role = "🤖 Nooty"
		}
		fmt.Printf("%s%s:%s %s\n", c(ansiBold), role, c(ansiReset), msg.Content)
	}
	fmt.Println()
}

// ============================================================================
// Persistence
// ============================================================================

func loadConfig() {
	data, err := os.ReadFile(configFile)
	if err != nil {
		config = Config{
			ProviderName:     "Groq Cloud (Ultra-Fast & Free Tier)",
			ProviderEndpoint: "https://api.groq.com/openai/v1",
			APIKey:           os.Getenv("GROQ_API_KEY"),
			Model:            "llama-3.3-70b-versatile",
			Safety:           "balanced",
			Mode:             "cli",
		}
		if config.APIKey == "" {
			config.APIKey = os.Getenv("OPENAI_API_KEY")
			if config.APIKey != "" {
				config.ProviderName = "OpenAI"
				config.ProviderEndpoint = "https://api.openai.com/v1"
				config.Model = "gpt-4o-mini"
			}
		}
		return
	}
	_ = json.Unmarshal(data, &config)

	if config.Mode == "" {
		config.Mode = "cli"
	}
	if config.Safety == "" {
		config.Safety = "balanced"
	}
	if config.ProviderName == "" {
		config.ProviderName = "Custom / OpenAI-Compatible"
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
