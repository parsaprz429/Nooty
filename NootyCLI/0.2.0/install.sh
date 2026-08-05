#!/usr/bin/env bash
# ==============================================================================
# 🚀 NootyCLI Minimal & Fast Installer
# ==============================================================================

set -e

# Palette
RESET="\033[0m"
BOLD="\033[1m"
CYAN="\033[36m"
GREEN="\033[32m"
GRAY="\033[90m"
RED="\033[31m"

echo -e "\n${BOLD}${CYAN}⚡ Installing NootyCLI...${RESET}\n"

# Check Go Compiler
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go compiler is not installed.${RESET}"
    echo -e "${GRAY}Please install Go from https://go.dev/doc/install or via 'brew install go'${RESET}\n"
    exit 1
fi

# Build Binary
echo -e "${GRAY}• Compiling binary source...${RESET}"
go build -ldflags="-s -w" -o nooty main.go

# Install to /usr/local/bin
echo -e "${GRAY}• Registering binaries (nooty & nootycli)...${RESET}"
if [ -w /usr/local/bin ]; then
    mv nooty /usr/local/bin/nooty
    ln -sf /usr/local/bin/nooty /usr/local/bin/nootycli
else
    sudo mv nooty /usr/local/bin/nooty
    sudo ln -sf /usr/local/bin/nooty /usr/local/bin/nootycli
fi

chmod +x /usr/local/bin/nooty

echo -e "\n${GREEN}${BOLD}✓ Successfully installed NootyCLI!${RESET}"
echo -e "${GRAY}Run ${RESET}${BOLD}nooty${RESET}${GRAY} or ${RESET}${BOLD}nootycli${RESET}${GRAY} to launch.${RESET}\n"
