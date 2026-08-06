#!/usr/bin/env bash
# ==============================================================================
# 🚀 NootyCLI Auto Installer v0.2.2 "Radin Pro"
# ==============================================================================

set -e

VERSION="0.2.2"
SOURCE_URL="https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/0.2.2/nooty.go"
BINARY_NAME="nooty"
TEMP_DIR="/tmp/nooty-install-$$"

if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    MAGENTA='\033[0;35m'
    CYAN='\033[0;36m'
    WHITE='\033[1;37m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    RED=''; GREEN=''; YELLOW=''; BLUE=''; MAGENTA=''; CYAN=''; WHITE=''; BOLD=''; NC=''
fi

echo -e "${CYAN}┌──────────────────────────────────────────────────────────┐${NC}"
echo -e "${CYAN}│${MAGENTA}${BOLD}   _  _  ___   ___  _____ __   __  ___  _    ___        ${CYAN}│${NC}"
echo -e "${CYAN}│${MAGENTA}${BOLD}  | \| |/ _ \ / _ \|_   _|\ \ / / / __|| |  |_ _|       ${CYAN}│${NC}"
echo -e "${CYAN}│${MAGENTA}${BOLD}  | .` | (_) | (_) | | |   \ V / | (__ | |__ | |        ${CYAN}│${NC}"
echo -e "${CYAN}│${MAGENTA}${BOLD}  |_|\_|\___/ \___/  |_|    |_|   \___||____|___|       ${CYAN}│${NC}"
echo -e "${CYAN}├──────────────────────────────────────────────────────────┤${NC}"
echo -e "${CYAN}│${WHITE}  NootyCLI v${VERSION} — Agentic Terminal Intelligence       ${CYAN}│${NC}"
echo -e "${CYAN}└──────────────────────────────────────────────────────────┘${NC}"
echo

echo -e " ${BLUE}ℹ${NC} Detecting operating system..."
OS="$(uname -s)"
ARCH="$(uname -m)"
echo -e " ${GREEN}✔${NC} System identified: ${BOLD}${OS} (${ARCH})${NC}"

echo -e " ${BLUE}ℹ${NC} Checking Go toolchain..."
if command -v go >/dev/null 2>&1; then
    GO_VER="$(go version | awk '{print $3}')"
    echo -e " ${GREEN}✔${NC} Found Go Engine: ${BOLD}${GO_VER}${NC}"
else
    echo -e " ${YELLOW}⚠${NC} Go toolchain not found. Installing Go..."
    if [ "$OS" = "Darwin" ]; then
        brew install go
    elif [ "$OS" = "Linux" ]; then
        sudo apt-get update -qq && sudo apt-get install -y -qq golang-go
    else
        echo -e " ${RED}✖${NC} Please install Go manually: https://go.dev/doc/install"
        exit 1
    fi
fi

mkdir -p "$TEMP_DIR"
trap 'rm -rf "$TEMP_DIR"' EXIT

echo -e " ${BLUE}ℹ${NC} Downloading NootyCLI core source..."
if ! curl -fsSL "$SOURCE_URL" -o "$TEMP_DIR/nooty.go"; then
    FALLBACK_URL="https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/nooty.go"
    curl -fsSL "$FALLBACK_URL" -o "$TEMP_DIR/nooty.go" || { echo -e " ${RED}✖${NC} Download failed."; exit 1; }
fi
echo -e " ${GREEN}✔${NC} Source code downloaded."

echo -e " ${BLUE}ℹ${NC} Compiling binary executable..."
(cd "$TEMP_DIR" && go build -ldflags="-s -w" -o "$BINARY_NAME" nooty.go)
echo -e " ${GREEN}✔${NC} Compilation successful."

TARGET_DIR="/usr/local/bin"
if [ ! -w "$TARGET_DIR" ]; then
    if command -v sudo >/dev/null 2>&1; then
        sudo mv "$TEMP_DIR/$BINARY_NAME" "$TARGET_DIR/"
        sudo chmod +x "$TARGET_DIR/$BINARY_NAME"
    else
        TARGET_DIR="$HOME/.local/bin"
        mkdir -p "$TARGET_DIR"
        mv "$TEMP_DIR/$BINARY_NAME" "$TARGET_DIR/"
        chmod +x "$TARGET_DIR/$BINARY_NAME"
    fi
else
    mv "$TEMP_DIR/$BINARY_NAME" "$TARGET_DIR/"
    chmod +x "$TARGET_DIR/$BINARY_NAME"
fi

echo -e " ${GREEN}✔${NC} Installed to ${TARGET_DIR}/${BINARY_NAME}"

mkdir -p "$HOME/.nooty/chats"

echo
echo -e "${GREEN}🎉 NootyCLI v${VERSION} successfully installed!${NC}"
echo -e "Run ${CYAN}${BOLD}nooty${NC} to start."
