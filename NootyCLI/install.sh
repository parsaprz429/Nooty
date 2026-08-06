#!/usr/bin/env bash
# ==============================================================================
# ⚡ NootyCLI v0.2.2 - Next-Gen Smart Minimal Installer
# ==============================================================================
set -e

VERSION="0.2.2"
REPO_RAW="https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI"
BINARY_NAME="nooty"
TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t 'nooty_install')"

# Auto cleanup temporary files
trap 'rm -rf "$TMP_DIR"' EXIT

# Formatting & Styling
if [ -t 1 ]; then
    BOLD='\033[1m'; CYAN='\033[0;36m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; DIM='\033[2m'; NC='\033[0m'
else
    BOLD=''; CYAN=''; GREEN=''; YELLOW=''; RED=''; DIM=''; NC=''
fi

# Minimal Smooth Progress Bar
draw_progress() {
    local duration=${1:-0.8}
    local steps=20
    local sleep_val
    sleep_val=$(awk "BEGIN {print $duration/$steps}" 2>/dev/null || echo "0.04")

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

# 1. System Architecture Detection
printf " ${CYAN}🔍 Architecture:${NC} "
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Darwin)  SYS_NAME="macOS (${ARCH})" ;;
    Linux)
        if grep -qi microsoft /proc/version 2>/dev/null; then
            SYS_NAME="WSL (${ARCH})"
        else
            SYS_NAME="Linux (${ARCH})"
        fi
        ;;
    *)
        echo -e "${RED}Unsupported OS: $OS${NC}"
        exit 1
        ;;
esac
echo -e "${GREEN}${SYS_NAME}${NC}"

# 2. Go Environment Validation
printf " ${CYAN}⚡ Engine Check:${NC} "
if command -v go >/dev/null 2>&1; then
    GO_VER="$(go version | awk '{print $3}')"
    echo -e "${GREEN}Go ${GO_VER}${NC}"
else
    echo -e "${YELLOW}Installing Go Toolchain...${NC}"
    if [ "$OS" = "Darwin" ]; then
        if ! command -v brew >/dev/null 2>&1; then
            echo -e "${RED}Homebrew is required to auto-install Go on macOS.${NC}"
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
        else
            echo -e "${RED}Please install Go manually: https://go.dev/dl/${NC}"
            exit 1
        fi
    fi
    echo -e " ${GREEN}✔ Go toolchain installed successfully.${NC}"
fi

# 3. Dynamic Core Source Fetch
printf " ${CYAN}📥 Core Source:${NC}  "
SOURCE_URL="${REPO_RAW}/${VERSION}/nooty.go"
FALLBACK_URL="${REPO_RAW}/nooty.go"

if curl -fsSL "$SOURCE_URL" -o "$TMP_DIR/nooty.go" 2>/dev/null; then
    echo -e "${GREEN}Retrieved v${VERSION}${NC}"
elif curl -fsSL "$FALLBACK_URL" -o "$TMP_DIR/nooty.go" 2>/dev/null; then
    echo -e "${GREEN}Retrieved (Main Fallback)${NC}"
else
    echo -e "${RED}Failed to download source binary.${NC}"
    exit 1
fi

# 4. Binary Compilation
printf " ${CYAN}🛠️  Building:${NC}     "
(
    cd "$TMP_DIR"
    go build -ldflags="-s -w" -o "$BINARY_NAME" nooty.go >/dev/null 2>&1
)
echo -e "${GREEN}Optimized binary generated${NC}"

# 5. Smart Deployment & Permissions
printf " ${CYAN}📦 Deploying:${NC}    "
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

# 6. Smooth Progress Animation
draw_progress 0.6

echo -e "${DIM}──────────────────────────────────────────────────────────────${NC}"
echo -e "${GREEN}${BOLD}✨ NootyCLI v${VERSION} is ready to launch!${NC}"
echo -e "   🚀 Run ${CYAN}${BOLD}nooty${NC} in your terminal."

if [ "${NEED_PATH_WARN:-0}" -eq 1 ]; then
    echo -e "   ${YELLOW}⚠️  Note: Ensure '${INSTALL_DIR}' is added to your \$PATH.${NC}"
fi
echo
