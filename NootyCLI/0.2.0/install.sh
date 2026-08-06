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
NC='\033[0m' # No Color

progress_bar() {
    local duration=$1
    local steps=20
    for ((i=0; i<=steps; i++)); do
        local percent=$(( i * 100 / steps ))
        local filled=$(( percent / 5 ))
        local empty=$(( 20 - filled ))
        printf "\r[${CYAN}%-${filled}s${NC}%-${empty}s] %d%%" | tr ' ' '#' | tr ' ' '.'
        sleep "$(echo "scale=2; $duration / $steps" | bc)"
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
        else
            echo -e "${RED}Cannot install Go automatically. Please install Go manually.${NC}"
            exit 1
        fi
    fi
    echo -e "${GREEN}✓ Go installed successfully${NC}"
fi

# Download nooty.go
echo -e "Downloading nooty.go..."
curl -fsSL "$REPO_URL" -o /tmp/nooty.go
echo -e "${GREEN}✓ Downloaded${NC}"

# Build
echo -e "Building NootyCLI..."
cd /tmp
go build -o "$BINARY_NAME" nooty.go
echo -e "${GREEN}✓ Build successful${NC}"

# Install binary
echo -e "Installing to ${INSTALL_DIR}/${BINARY_NAME}..."
sudo mv "$BINARY_NAME" "$INSTALL_DIR/"
sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
echo -e "${GREEN}✓ Installed${NC}"

# Clean up
rm /tmp/nooty.go

# Show progress bar for fun
echo -e "\nFinalizing..."
progress_bar 1.5

echo -e "\n${GREEN}NootyCLI v${NOOTY_VERSION} is ready!${NC}"
echo -e "Run it by typing: ${CYAN}nooty${NC}"
