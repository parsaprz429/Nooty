#!/usr/bin/env bash
# install.sh – Smart installer for NootyCLI v0.2.2 (macOS / Linux / WSL)
set -e

NOOTY_VERSION="0.2.2"
REPO_URL="https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/${NOOTY_VERSION}/nooty.go"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="nooty"

# Colors
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

progress_bar() {
    local duration=${1:-1}; local steps=20
    for ((i=0;i<=steps;i++)); do
        local pct=$((i*100/steps)); local filled=$((pct/5)); local empty=$((20-filled))
        printf "\r["
        for ((j=0;j<filled;j++)); do printf "#"; done
        for ((j=0;j<empty;j++)); do printf "."; done
        printf "] %%d%%%%" "$pct"
        sleep 0.05
    done; echo
}

echo -e "${GREEN}╔══════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║   NootyCLI v${NOOTY_VERSION} – Smart Installer       ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════╝${NC}"
echo

OS="$(uname -s)"
if [[ "$OS" == "Darwin" ]]; then echo -e "${CYAN}✓ macOS${NC}"
elif [[ "$OS" == "Linux" ]]; then echo -e "${CYAN}✓ Linux${NC}"
else echo -e "${RED}Unsupported OS: $OS${NC}"; exit 1; fi

if command -v go &>/dev/null; then
    echo -e "${GREEN}✓ Go $(go version | awk '{print $3}')${NC}"
else
    echo -e "${YELLOW}Installing Go...${NC}"
    if [[ "$OS" == "Darwin" ]]; then
        if ! command -v brew &>/dev/null; then /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"; fi
        brew install go
    else
        if command -v apt-get &>/dev/null; then sudo apt-get update -qq && sudo apt-get install -y -qq golang-go
        elif command -v yum &>/dev/null; then sudo yum install -y golang
        elif command -v dnf &>/dev/null; then sudo dnf install -y golang
        else echo -e "${RED}Install Go manually: https://go.dev/dl/${NC}"; exit 1; fi
    fi
    echo -e "${GREEN}✓ Go installed${NC}"
fi

echo -e "\n${CYAN}📥 Downloading nooty.go (v${NOOTY_VERSION})...${NC}"
curl -fsSL "$REPO_URL" -o /tmp/nooty.go
echo -e "${GREEN}✓ Downloaded${NC}"

echo -e "\n${CYAN}🔨 Building optimized binary...${NC}"
cd /tmp
go build -ldflags="-s -w" -o "$BINARY_NAME" nooty.go
echo -e "${GREEN}✓ Built${NC}"

echo -e "\n${CYAN}📦 Installing to ${INSTALL_DIR}/${BINARY_NAME}...${NC}"
if [ -w "$INSTALL_DIR" ]; then
    mv "$BINARY_NAME" "$INSTALL_DIR/"
    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
else
    sudo mv "$BINARY_NAME" "$INSTALL_DIR/"
    sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
fi
rm -f /tmp/nooty.go
echo -e "${GREEN}✓ Installed${NC}"

echo -e "\n${CYAN}⚡ Finalizing...${NC}"
progress_bar 1

echo -e "\n${GREEN}${BOLD}✅ NootyCLI v${NOOTY_VERSION} ready!${NC}"
echo -e "   Run: ${CYAN}nooty${NC}"
echo -e "   Then: ${CYAN}/config${NC} , ${CYAN}/mode cli${NC} , ${CYAN}/help${NC}\n"
