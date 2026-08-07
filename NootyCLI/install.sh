#!/usr/bin/env bash
# ==============================================================================
# 🌌 NootyCLI Installer v0.2.2 — Smart Minimal Cross-Platform Deployer
# ==============================================================================
set -e

VERSION="0.2.2"
REPO_RAW="https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI"
BINARY_NAME="nooty"
TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t 'nooty_install')"

# Auto cleanup temporary directory on exit
trap 'rm -rf "$TMP_DIR"' EXIT

# Terminal Styling & Colors
if [ -t 1 ]; then
    BOLD='\033[1m'; CYAN='\033[0;36m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; DIM='\033[2m'; NC='\033[0m'
else
    BOLD=''; CYAN=''; GREEN=''; YELLOW=''; RED=''; DIM=''; NC=''
fi

# Sleek Minimal Progress Animation
draw_progress() {
    local duration=${1:-0.6}
    local steps=20
    local sleep_val
    sleep_val=$(awk "BEGIN {print $duration/$steps}" 2>/dev/null || echo "0.03")

    for ((i=0; i<=steps; i++)); do
        local pct=$((i * 100 / steps))
        local filled=$((pct / 5))
        local empty=$((20 - filled))
        
        local bar=""
        for ((f=0; f<filled; f++)); do bar="${bar}█"; done
        for ((e=0; e<empty; e++)); do bar="${bar}░"; done
        
        printf "\r ${CYAN}⚡${NC} [${GREEN}%s${NC}] ${BOLD}%3d%%${NC}" "$bar" "$pct"
        sleep "$sleep_val"
    done
    echo ""
}

echo
echo -e "${CYAN}${BOLD}🌌 NOOTY CLI ${NC}${DIM}v${VERSION}${NC} ${DIM}— Agentic Terminal Intelligence${NC}"
echo -e "${DIM}──────────────────────────────────────────────────────────────${NC}"

# 1. Cross-Platform OS & Architecture Detection (Linux / macOS / Windows)
printf " ${CYAN}🔍 Environment :${NC} "
OS="$(uname -s)"
ARCH="$(uname -m)"
IS_WINDOWS=0

case "$OS" in
    Darwin) 
        SYS_NAME="macOS (${ARCH})" 
        ;;
    Linux)
        if grep -qi microsoft /proc/version 2>/dev/null; then
            SYS_NAME="WSL / Linux (${ARCH})"
        else
            SYS_NAME="Linux (${ARCH})"
        fi
        ;;
    MINGW*|MSYS*|CYGWIN*)
        SYS_NAME="Windows (${ARCH})"
        IS_WINDOWS=1
        BINARY_NAME="nooty.exe"
        ;;
    *)
        echo -e "${RED}Unsupported Operating System: $OS${NC}"
        exit 1
        ;;
esac
echo -e "${GREEN}${SYS_NAME}${NC}"

# 2. Go Toolchain Check & Auto-Install
printf " ${CYAN}⚡ Go Engine   :${NC} "
if command -v go >/dev/null 2>&1; then
    GO_VER="$(go version | awk '{print $3}')"
    echo -e "${GREEN}Detected (${GO_VER})${NC}"
else
    echo -e "${YELLOW}Installing Go Toolchain...${NC}"
    if [ "$OS" = "Darwin" ]; then
        if ! command -v brew >/dev/null 2>&1; then
            echo -e "${RED}Homebrew is required to install Go on macOS.${NC}"
            exit 1
        fi
        brew install go >/dev/null 2>&1
    elif [ "$OS" = "Linux" ]; then
        if command -v apt-get >/dev/null 2>&1; then
            sudo apt-get update -qq && sudo apt-get install -y -qq golang-go >/dev/null 2>&1
        elif command -v dnf >/dev/null 2>&1; then
            sudo dnf install -y -q golang >/dev/null 2>&1
        elif command -v pacman >/dev/null 2>&1; then
            sudo pacman -S --noconfirm go >/dev/null 2>&1
        fi
    elif [ "$IS_WINDOWS" -eq 1 ]; then
        if command -v winget >/dev/null 2>&1; then
            winget install --id GoLang.Go -e --silent >/dev/null 2>&1
        elif command -v choco >/dev/null 2>&1; then
            choco install golang -y >/dev/null 2>&1
        else
            echo -e "${RED}Please install Go manually on Windows: https://go.dev/dl/${NC}"
            exit 1
        fi
    fi
    echo -e " ${GREEN}✔ Go Engine ready!${NC}"
fi

# 3. Direct Core Source Fetch (Root NootyCLI/nooty.go)
printf " ${CYAN}📥 Fetching Core:${NC} "
SOURCE_URL="${REPO_RAW}/nooty.go"

if curl -fsSL "$SOURCE_URL" -o "$TMP_DIR/nooty.go" 2>/dev/null; then
    echo -e "${GREEN}Latest Main Engine Fetched${NC}"
else
    echo -e "${RED}Failed to download core source from GitHub.${NC}"
    exit 1
fi

# 4. Binary Compilation
printf " ${CYAN}🛠️  Building    :${NC} "
(
    cd "$TMP_DIR"
    go build -ldflags="-s -w" -o "$BINARY_NAME" nooty.go >/dev/null 2>&1
)
echo -e "${GREEN}Optimized Binary Compiled${NC}"

# 5. Smart Binary Deployment
printf " ${CYAN}📦 Deploying   :${NC} "
if [ "$IS_WINDOWS" -eq 1 ]; then
    INSTALL_DIR="$HOME/bin"
    mkdir -p "$INSTALL_DIR"
    mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/"
    echo -e "${GREEN}${INSTALL_DIR}/${BINARY_NAME}${NC}"
    NEED_PATH_WARN=1
else
    INSTALL_DIR="/usr/local/bin"
    if [ -w "$INSTALL_DIR" ]; then
        mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/"
        chmod +x "$INSTALL_DIR/$BINARY_NAME"
    else
        if command -v sudo >/dev/null 2>&1; then
            sudo mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/"
            sudo chmod +x "$INSTALL_DIR/$BINARY_NAME"
        else
            INSTALL_DIR="$HOME/.local/bin"
            mkdir -p "$INSTALL_DIR"
            mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/"
            chmod +x "$INSTALL_DIR/$BINARY_NAME"
            NEED_PATH_WARN=1
        fi
    fi
    echo -e "${GREEN}${INSTALL_DIR}/${BINARY_NAME}${NC}"
fi

# 6. Smooth Completion Animation
draw_progress 0.5

echo -e "${DIM}──────────────────────────────────────────────────────────────${NC}"
echo -e "${GREEN}${BOLD}✨ NootyCLI v${VERSION} Installed Successfully!${NC}"
echo -e "   🚀 Run ${CYAN}${BOLD}nooty${NC} in your terminal to begin."

if [ "${NEED_PATH_WARN:-0}" -eq 1 ]; then
    echo -e "   ${YELLOW}⚠️  Note: Make sure '${INSTALL_DIR}' is added to your PATH environment variable.${NC}"
fi
echo
