#!/bin/bash

# 🧪 NootyCLI Beta Cross-Platform Smart Installer
# Ecosystem: Nooty (nooty.ir) | Author: Parsa
set -e

# رنگ‌بندی جذاب و استاندارد 🎨
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # Reset

clear
echo -e "${CYAN}${BOLD}"
echo " _  _             _        ___ _    ___ "
echo "| \| |___  ___ |_| |_ _  / C \ |  |_ _|"
echo "| .\` / _ \/ _ \  |  _| || |  __/ |__| | "
echo "|_|\_\___/\___/  |_|  \_, |\___|____|___|"
echo "                      |__/"
echo -e "${YELLOW}            🚀 NootyCLI Beta Installer (v0.3.0 Radin Edge)${NC}\n"

echo -e "${BLUE}🔎 [1/5] Analyzing OS & Environment...${NC}"

# ۱. تشخیص دقیق سیستم‌عامل
OS_RAW="$(uname -s)"
ARCH_RAW="$(uname -m)"

OS="Unknown"
case "$OS_RAW" in
    Linux*)     OS="Linux" ;;
    Darwin*)    OS="macOS" ;;
    CYGWIN*|MINGW*|MSYS*) OS="Windows" ;;
    *)          OS="$OS_RAW" ;;
esac

echo -e "${GREEN}✨ Detected OS:${NC} ${BOLD}$OS${NC} (${ARCH_RAW})"

SUDO=""
if [ "$OS" != "Windows" ] && [ "$(id -u)" -ne 0 ]; then
    if command -v sudo &> /dev/null; then
        SUDO="sudo"
    fi
fi

# ۲. نصب خودکار پیش‌نیازها
install_dependencies() {
    echo -e "${BLUE}📦 [2/5] Auto-installing dependencies (Go, Curl, Git)...${NC}"
    
    case "$OS" in
        Linux)
            if command -v apt-get &> /dev/null; then
                $SUDO apt-get update -qq
                $SUDO apt-get install -y -qq curl git golang-go
            elif command -v dnf &> /dev/null; then
                $SUDO dnf install -y golang curl git
            elif command -v pacman &> /dev/null; then
                $SUDO pacman -Sy --noconfirm go curl git
            elif command -v zypper &> /dev/null; then
                $SUDO zypper install -y go curl git
            else
                echo -e "${RED}❌ Unsupported Package Manager. Please install 'go' manually.${NC}"
                exit 1
            fi
            ;;
        macOS)
            if ! command -v brew &> /dev/null; then
                echo -e "${YELLOW}⚠️ Homebrew not found. Installing Homebrew...${NC}"
                /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
            fi
            brew install go curl git
            ;;
        *)
            echo -e "${RED}❌ Unsupported Operating System.${NC}"
            exit 1
            ;;
    esac
}

if ! command -v go &> /dev/null || ! command -v curl &> /dev/null || ! command -v git &> /dev/null; then
    install_dependencies
else
    echo -e "${GREEN}✅ All core tools (Go, Curl, Git) are already installed.${NC}"
fi

# ۳. پاکسازی نسخه قبلی
BIN_TARGET="/usr/local/bin/nooty-beta"
if [ "$OS" = "Windows" ]; then
    BIN_TARGET="/usr/bin/nooty-beta"
fi

if [ -f "$BIN_TARGET" ]; then
    echo -e "${YELLOW}🧹 [3/5] Cleaning up previous Beta binary...${NC}"
    $SUDO rm -f "$BIN_TARGET"
fi

# ۴. دانلود دقیق سورس‌کد اصلی (اصلاح لینک به Beta با B بزرگ)
BUILD_DIR=$(mktemp -d)
echo -e "${BLUE}📥 [4/5] Downloading latest NootyCLI source code...${NC}"

SOURCE_URL="https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/Beta/nooty.go?v=$(date +%s)"

curl -fsSL "$SOURCE_URL" -o "$BUILD_DIR/nooty.go"

if [ ! -s "$BUILD_DIR/nooty.go" ]; then
    echo -e "${RED}❌ Download Failed! File is empty or link is incorrect.${NC}"
    rm -rf "$BUILD_DIR"
    exit 1
fi

# ۵. کامپایل و نصب نهایی
echo -e "${BLUE}🔨 [5/5] Compiling NootyCLI Beta...${NC}"
cd "$BUILD_DIR"
go mod init nooty-beta > /dev/null 2>&1

go build -ldflags="-s -w" -o nooty-beta nooty.go

if [ $? -eq 0 ]; then
    $SUDO mv nooty-beta "$BIN_TARGET"
    $SUDO chmod +x "$BIN_TARGET"
    
    rm -rf "$BUILD_DIR"
    hash -r 2>/dev/null || true

    echo -e "\n${GREEN}${BOLD}🎉 NootyCLI Beta (v0.3.0 Radin Edge) Installed Successfully!${NC}"
    echo -e "${CYAN}⚡ Start exploring now by running: ${YELLOW}${BOLD}nooty-beta${NC}\n"
else
    echo -e "${RED}❌ Build Compilation Error! Check Go code for errors.${NC}"
    rm -rf "$BUILD_DIR"
    exit 1
fi
