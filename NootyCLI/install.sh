#!/usr/bin/env bash
# ==============================================================================
# 🚀 NootyCLI Auto Installer v0.2.2 "Radin Pro"
# 🌍 Compatible with: macOS / Linux / WSL / Git Bash
# 🔗 Repository: https://github.com/parsaprz429/Nooty
# ==============================================================================

set -e

# ---------- Configuration ----------
VERSION="0.2.2"
SOURCE_URL="https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/nooty.go"
BINARY_NAME="nooty"
TEMP_DIR="/tmp/nooty-install-$$"

# ---------- Colors & Formatting ----------
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    MAGENTA='\033[0;35m'
    CYAN='\033[0;36m'
    WHITE='\033[1;37m'
    BOLD='\033[1m'
    DIM='\033[2m'
    NC='\033[0m'
else
    RED=''; GREEN=''; YELLOW=''; BLUE=''; MAGENTA=''; CYAN=''; WHITE=''; BOLD=''; DIM=''; NC=''
fi

# ---------- UI Helpers ----------
spinner() {
    local pid=$1
    local delay=0.1
    local spinstr='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    while [ "$(ps a | awk '{print $1}' | grep "$pid")" ]; do
        local temp=${spinstr#?}
        printf " ${CYAN}%c${NC}  " "$spinstr"
        local spinstr=$temp${spinstr%"$temp"}
        sleep $delay
        printf "\b\b\b\b"
    done
    printf "    \b\b\b\b"
}

print_banner() {
    clear 2>/dev/null || true
    echo -e "${CYAN}┌──────────────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${MAGENTA}${BOLD}   _  _  ___   ___  _____ __   __  ___  _    ___        ${CYAN}│${NC}"
    echo -e "${CYAN}│${MAGENTA}${BOLD}  | \| |/ _ \ / _ \|_   _|\ \ / / / __|| |  |_ _|       ${CYAN}│${NC}"
    echo -e "${CYAN}│${MAGENTA}${BOLD}  | .` | (_) | (_) | | |   \ V / | (__ | |__ | |        ${CYAN}│${NC}"
    echo -e "${CYAN}│${MAGENTA}${BOLD}  |_|\_|\___/ \___/  |_|    |_|   \___||____|___|       ${CYAN}│${NC}"
    echo -e "${CYAN}├──────────────────────────────────────────────────────────┤${NC}"
    echo -e "${CYAN}│${WHITE}  NootyCLI v${VERSION} — Agentic Terminal Intelligence       ${CYAN}│${NC}"
    echo -e "${CYAN}└──────────────────────────────────────────────────────────┘${NC}"
    echo
}

info() { echo -e " ${BLUE}ℹ${NC} $1"; }
success() { echo -e " ${GREEN}✔${NC} $1"; }
warn() { echo -e " ${YELLOW}⚠${NC} $1"; }
error() { echo -e " ${RED}✖${NC} $1"; exit 1; }

# ---------- Main Installation Steps ----------
print_banner

# 1. OS & Architecture Detection
info "Detecting operating system & architecture..."
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Darwin)
        OS_DISPLAY="macOS"
        ;;
    Linux)
        if grep -q -i microsoft /proc/version 2>/dev/null; then
            OS_DISPLAY="Linux (WSL)"
        else
            OS_DISPLAY="Linux"
        fi
        ;;
    *)
        error "Unsupported operating system: $OS"
        ;;
esac

success "System identified: ${BOLD}${OS_DISPLAY} (${ARCH})${NC}"

# 2. Dependency Check (Go Language Engine)
info "Checking Go toolchain requirement..."
if command -v go &>/dev/null; then
    GO_VER="$(go version | awk '{print $3}')"
    success "Found Go Engine: ${BOLD}${GO_VER}${NC}"
else
    warn "Go toolchain not found. Attempting automatic installation..."
    if [ "$OS" = "Darwin" ]; then
        if ! command -v brew &>/dev/null; then
            info "Installing Homebrew..."
            /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
        fi
        brew install go
    elif [ "$OS" = "Linux" ]; then
        if command -v apt-get &>/dev/null; then
            sudo apt-get update -qq && sudo apt-get install -y -qq golang-go
        elif command -v dnf &>/dev/null; then
            sudo dnf install -y golang
        elif command -v yum &>/dev/null; then
            sudo yum install -y golang
        elif command -v pacman &>/dev/null; then
            sudo pacman -S --noconfirm go
        elif command -v zypper &>/dev/null; then
            sudo zypper install -y go
        else
            error "Could not auto-install Go. Please install Go manually: https://go.dev/doc/install"
        fi
    fi
    success "Go toolchain installed successfully!"
fi

# 3. Setup Temporary Directory
mkdir -p "$TEMP_DIR"
trap 'rm -rf "$TEMP_DIR"' EXIT

# 4. Fetch Latest NootyCLI Source Code
echo -n -e " ${BLUE}ℹ${NC} Downloading latest NootyCLI core source code..."
(curl -fsSL "$SOURCE_URL" -o "$TEMP_DIR/nooty.go") &
spinner $!

if [ ! -s "$TEMP_DIR/nooty.go" ]; then
    echo
    error "Failed to download NootyCLI source from GitHub repository."
fi
success "Source code downloaded successfully."

# 5. Compile Binary Engine
echo -n -e " ${BLUE}ℹ${NC} Compiling binary executable with optimizations..."
(cd "$TEMP_DIR" && go build -ldflags="-s -w" -o "$BINARY_NAME" nooty.go) &
spinner $!

if [ ! -f "$TEMP_DIR/$BINARY_NAME" ]; then
    echo
    error "Compilation failed. Please check your Go environment."
fi
success "Build completed successfully!"

# 6. Install to System PATH
TARGET_DIR="/usr/local/bin"
USE_SUDO=false

if [ ! -w "$TARGET_DIR" ]; then
    if command -v sudo &>/dev/null && [ -w "/tmp" ]; then
        USE_SUDO=true
    else
        TARGET_DIR="$HOME/.local/bin"
        mkdir -p "$TARGET_DIR"
    fi
fi

info "Installing executable binary to ${BOLD}${TARGET_DIR}/${BINARY_NAME}${NC}..."

if [ "$USE_SUDO" = true ]; then
    sudo mv "$TEMP_DIR/$BINARY_NAME" "$TARGET_DIR/"
    sudo chmod +x "$TARGET_DIR/$BINARY_NAME"
else
    mv "$TEMP_DIR/$BINARY_NAME" "$TARGET_DIR/"
    chmod +x "$TARGET_DIR/$BINARY_NAME"
fi

success "Binary installed successfully to ${TARGET_DIR}/${BINARY_NAME}"

# 7. Initialize User Workspace & Directory
NOOTY_HOME="$HOME/.nooty"
mkdir -p "$NOOTY_HOME/chats"

# 8. Check PATH for $HOME/.local/bin
if [ "$TARGET_DIR" = "$HOME/.local/bin" ]; then
    case ":$PATH:" in
        *":$HOME/.local/bin:"*) ;;
        *)
            warn "Add ${BOLD}~/.local/bin${NC} to your PATH environment variable:"
            echo -e "   ${DIM}export PATH=\"\$HOME/.local/bin:\$PATH\"${NC}"
            ;;
    esac
fi

# ---------- Completion Header ----------
echo
echo -e "${GREEN}┌──────────────────────────────────────────────────────────┐${NC}"
echo -e "${GREEN}│ ${BOLD}🎉 NootyCLI v${VERSION} has been successfully installed!     ${NC}${GREEN}│${NC}"
echo -e "${GREEN}└──────────────────────────────────────────────────────────┘${NC}"
echo
echo -e " 🚀 ${BOLD}To launch NootyCLI instantly, run:${NC}"
echo -e "    ${CYAN}${BOLD}nooty${NC}"
echo
echo -e " ⚙️  ${BOLD}First steps inside NootyCLI:${NC}"
echo -e "    • ${YELLOW}/config${NC}      → Configure your API key and AI models"
echo -e "    • ${YELLOW}/mode cli${NC}    → Enable Agentic CLI / Autonomous Mode"
echo -e "    • ${YELLOW}/help${NC}        → Display full command reference"
echo
echo -e " 💙 ${DIM}Thank you for using Nooty Ecosystem! (https://nooty.ir)${NC}"
echo
