#!/usr/bin/env bash
# ==============================================================================
# 🚀 NootyCLI Universal Smart Installer v0.1.0
# Built for macOS & Linux | Anti-Sanction & Zero-Dependency Ready
# ==============================================================================

set -e

# ANSI Color Palette
RESET="\033[0m"
BOLD="\033[1m"
GREEN="\033[32m"
CYAN="\033[36m"
YELLOW="\033[33m"
RED="\033[31m"
MAGENTA="\033[35m"

# Dynamic Progress Bar Function
show_progress() {
    local duration=$1
    local task_name=$2
    local progress=0
    local width=30

    echo -ne "${CYAN}⏳ $task_name...${RESET}\n"
    while [ $progress -le 100 ]; do
        local filled=$((progress * width / 100))
        local empty=$((width - filled))
        local bar=$(printf "%${filled}s" | tr ' ' '█')
        local spaces=$(printf "%${empty}s" | tr ' ' '░')

        echo -ne "\r${MAGENTA}[${bar}${spaces}]${RESET} ${BOLD}${progress}%${RESET}"
        sleep $(awk "BEGIN {print $duration / 100}")
        progress=$((progress + 5))
    done
    echo -e "\n${GREEN}✓ Done!${RESET}\n"
}

clear
echo -e "${BOLD}${CYAN}"
cat << "EOF"
  _  _  ___   ___  _____ __   __  ___  _     ___ 
 | \| |/ _ \ / _ \|_   _|\ \ / / / __|| |   |_ _|
 | .` | (_) | (_) | | |   \ V / | (__ | |__  | | 
 |_|\_|\___/ \___/  |_|    |_|   \___||____||___|
EOF
echo -e "${RESET}${BOLD}=== NootyCLI v0.1.0 Installer ===${RESET}\n"

# 1. Detect Operating System and Architecture
OS="$(uname -s)"
ARCH="$(uname -m)"

echo -e "${YELLOW}🔍 Detecting System Environment...${RESET}"
echo -e "   • OS: ${BOLD}$OS${RESET}"
echo -e "   • Architecture: ${BOLD}$ARCH${RESET}\n"

# 2. Check and Install Go Compiler if missing
if ! command -v go &> /dev/null; then
    echo -e "${RED}⚠️  Go compiler not found! Starting automatic Go installation...${RESET}"
    
    GO_VERSION="1.22.5"
    if [ "$OS" = "Darwin" ]; then
        if command -v brew &> /dev/null; then
            echo -e "${CYAN}📦 Installing Go via Homebrew...${RESET}"
            brew install go
        else
            echo -e "${RED}❌ Homebrew not found. Please install Go manually from https://go.dev/doc/install${RESET}"
            exit 1
        fi
    elif [ "$OS" = "Linux" ]; then
        show_progress 2 "Downloading Official Go $GO_VERSION binary"
        GO_TAR="go${GO_VERSION}.linux-amd64.tar.gz"
        if [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
            GO_TAR="go${GO_VERSION}.linux-arm64.tar.gz"
        fi
        
        curl -sSL "https://golang.org/dl/$GO_TAR" -o /tmp/go.tar.gz
        sudo rm -rf /usr/local/go
        sudo tar -C /usr/local -xzf /tmp/go.tar.gz
        rm /tmp/go.tar.gz
        export PATH=$PATH:/usr/local/go/bin
    fi
else
    echo -e "${GREEN}✓ Go compiler detected:${RESET} $(go version)"
fi

# 3. Compile NootyCLI Binary
echo -e "\n${YELLOW}🛠️  Compiling NootyCLI v0.1.0 (Zero-Dependency)...${RESET}"
show_progress 1.5 "Building Binary Code"

go build -ldflags="-s -w" -o nooty main.go

if [ ! -f ./nooty ]; then
    echo -e "${RED}❌ Compilation failed! Check main.go source code.${RESET}"
    exit 1
fi

# 4. Install Binary to PATH
echo -e "${YELLOW}🚀 Installing Nooty CLI to system PATH (/usr/local/bin)...${RESET}"
if [ -w /usr/local/bin ]; then
    mv nooty /usr/local/bin/nooty
else
    sudo mv nooty /usr/local/bin/nooty
fi

chmod +x /usr/local/bin/nooty

echo -e "\n${GREEN}${BOLD}🎉 Installation Complete!${RESET}"
echo -e "${CYAN}Type ${BOLD}'nooty'${RESET}${CYAN} in your terminal to launch NootyCLI.${RESET}\n"
