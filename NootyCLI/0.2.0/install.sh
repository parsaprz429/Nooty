#!/usr/bin/env bash
# install.sh – Smart installer for NootyCLI v0.2.0
# Supports macOS and Linux. Installs Go if missing, downloads and builds Nooty.

set -e

NOOTY_VERSION="0.2.0"
REPO_URL="https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/${NOOTY_VERSION}/nooty.go"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="nooty"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

progress_bar() {
    local duration=${1:-1}
    local steps=20
    for ((i=0; i<=steps; i++)); do
        local percent=$(( i * 100 / steps ))
        local filled=$(( percent / 5 ))
        local empty=$(( 20 - filled ))
        printf "\r["
        for ((j=0; j<filled; j++)); do printf "#"; done
        for ((j=0; j<empty; j++)); do printf "."; done
        printf "] %d%%" $percent
        sleep $(echo "scale=3; $duration / $steps" | bc 2>/dev/null || echo "0.05")
    done
    echo
}

echo -e "${GREEN}╔══════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║   NootyCLI v${NOOTY_VERSION} – Smart Installer       ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════╝${NC}"
echo

# Detect OS
OS="$(uname -s)"
if [[ "$OS" == "Darwin" ]]; then
    echo -e "${CYAN}✓ macOS detected${NC}"
elif [[ "$OS" == "Linux" ]]; then
    echo -e "${CYAN}✓ Linux detected${NC}"
else
    echo -e "${RED}Unsupported OS: $OS${NC}"
    exit 1
fi

# Check Go
if command -v go &> /dev/null; then
    echo -e "${GREEN}✓ Go is already installed ($(go version | awk '{print $3}'))${NC}"
else
    echo -e "${YELLOW}Go not found. Installing...${NC}"
    if [[ "$OS" == "Darwin" ]]; then
        if ! command -v brew &> /dev/null; then
            echo "Installing Homebrew..."
            /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
        fi
        echo "Installing Go via Homebrew..."
        brew install go
    elif [[ "$OS" == "Linux" ]]; then
        if command -v apt-get &> /dev/null; then
            sudo apt-get update -qq
            sudo apt-get install -y -qq golang-go
        elif command -v yum &> /dev/null; then
            sudo yum install -y golang
        elif command -v dnf &> /dev/null; then
            sudo dnf install -y golang
        else
            echo -e "${RED}Cannot install Go automatically. Please install Go manually from https://go.dev/dl/${NC}"
            exit 1
        fi
    fi
    echo -e "${GREEN}✓ Go installed successfully${NC}"
fi

# Download nooty.go
echo -e "\n${CYAN}📥 Downloading nooty.go...${NC}"
curl -fsSL "$REPO_URL" -o /tmp/nooty.go
echo -e "${GREEN}✓ Downloaded${NC}"

# Build
echo -e "\n${CYAN}🔨 Building NootyCLI...${NC}"
cd /tmp
if go build -o "$BINARY_NAME" nooty.go 2>&1 | tee /tmp/build.log; then
    echo -e "${GREEN}✓ Build successful${NC}"
else
    echo -e "${RED}Build failed. Trying with flags...${NC}"
    go build -gcflags="-e" -o "$BINARY_NAME" nooty.go
fi

# Install binary
echo -e "\n${CYAN}📦 Installing to ${INSTALL_DIR}/${BINARY_NAME}...${NC}"
sudo mv "$BINARY_NAME" "$INSTALL_DIR/"
sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
echo -e "${GREEN}✓ Installed${NC}"

# Clean up
rm -f /tmp/nooty.go /tmp/build.log

# Show progress bar
echo -e "\n${CYAN}⚡ Finalizing...${NC}"
progress_bar 1.5

# Success message with clear instructions
echo -e "\n${GREEN}${BOLD}✅ NootyCLI v${NOOTY_VERSION} installed successfully!${NC}\n"
echo -e "${BOLD}🚀 How to run:${NC}"
echo -e "   Simply type: ${CYAN}nooty${NC}"
echo
echo -e "${BOLD}📝 First steps:${NC}"
echo -e "   1. Run: ${CYAN}nooty${NC}"
echo -e "   2. Configure: ${CYAN}/config${NC}"
echo -e "   3. Get help: ${CYAN}/help${NC}"
echo
echo -e "${BOLD}💡 Tips:${NC}"
echo -e "   • Set API key via environment: ${CYAN}export OPENAI_API_KEY=\"sk-...\"${NC}"
echo -e "   • Or set interactively: ${CYAN}/config${NC}"
echo -e "   • Browse models: ${CYAN}/model list${NC}"
echo -e "   • Switch to agent: ${CYAN}/mode cli${NC}"
echo
