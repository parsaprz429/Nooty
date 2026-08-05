#!/usr/bin/env bash
# ==============================================================================
# 🚀 NootyCLI v0.2.0 One-Line Installer
# ==============================================================================

set -e

# Palette
RESET="\033[0m"
BOLD="\033[1m"
CYAN="\033[36m"
GREEN="\033[32m"
GRAY="\033[90m"
RED="\033[31m"

echo -e "\n${BOLD}${CYAN}⚡ Installing NootyCLI v0.2.0...${RESET}\n"

# Check Go Compiler
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go is not installed on this system.${RESET}"
    echo -e "${GRAY}Please install Go first (e.g., 'brew install go') and try again.${RESET}\n"
    exit 1
fi

# Create Temp Build Directory
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo -e "${GRAY}• Downloading source code from GitHub...${RESET}"
curl -sSL "https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/0.2.0/main.go" -o "$TMP_DIR/main.go"

# Compile inside Temp Dir
echo -e "${GRAY}• Compiling NootyCLI binary...${RESET}"
cd "$TMP_DIR"
go mod init nooty > /dev/null 2>&1 || true
go build -ldflags="-s -w" -o nooty main.go

# Install to /usr/local/bin
echo -e "${GRAY}• Registering binary in system path...${RESET}"
if [ -w /usr/local/bin ]; then
    mv nooty /usr/local/bin/nooty
    ln -sf /usr/local/bin/nooty /usr/local/bin/nootycli
else
    sudo mv nooty /usr/local/bin/nooty
    sudo ln -sf /usr/local/bin/nooty /usr/local/bin/nootycli
fi

chmod +x /usr/local/bin/nooty

echo -e "\n${GREEN}${BOLD}✓ NootyCLI successfully installed!${RESET}"
echo -e "${GRAY}Run ${RESET}${BOLD}nooty${RESET}${GRAY} or ${RESET}${BOLD}nootycli${RESET}${GRAY} to launch.${RESET}\n"
