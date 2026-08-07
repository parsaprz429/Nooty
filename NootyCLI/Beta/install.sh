#!/bin/bash

# 🧪 NootyCLI Beta Smart One-Command Installer
set -e

# رنگ‌بندی ترمینال 🎨
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}"
echo "   _  _                 _            ___  _    ___ "
echo "  | \| |___  ___ |_ _ _ _   / C \ | |  |_ _|"
echo "  | .\` / _ \/ _ \  | || '_| |  __/ | |__ | | "
echo "  |_|\_\___/\___/  |_||_|    \___| |____|___|"
echo "            🚀 Beta Installer v0.3.0"
echo -e "${NC}"

echo -e "${BLUE}🔎 Detecting System Environment...${NC}"

# ۱. تشخیص سیستم‌عامل و مدیریت برنامه‌ساز (Package Manager)
OS="$(uname -s)"
ARCH="$(uname -m)"

SUDO=""
if [ "$(id -u)" -ne 0 ]; then
    if command -v sudo &> /dev/null; then
        SUDO="sudo"
    fi
fi

echo -e "${GREEN}🖥️  OS: $OS | Architecture: $ARCH${NC}"

# ۲. نصب هوشمند پیش‌نیازها (Go & Curl & Git) بر اساس سیستم‌عامل
install_packages() {
    echo -e "${BLUE}📦 Checking & Installing dependencies (Go, Curl, Git)...${NC}"
    
    case "$OS" in
        Linux*)
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
                echo -e "${RED}❌ Unsupported Linux package manager. Please install Go manually.${NC}"
                exit 1
            fi
            ;;
        Darwin*)
            if ! command -v brew &> /dev/null; then
                echo -e "${YELLOW}⚠️ Homebrew not found. Installing Homebrew...${NC}"
                /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
            fi
            brew install go curl git
            ;;
        *)
            echo -e "${RED}❌ Unsupported Operating System: $OS${NC}"
            exit 1
            ;;
    esac
}

# اگر Go یا curl یا git نصب نبودند، اتوماتیک همه رو نصب کن
if ! command -v go &> /dev/null || ! command -v curl &> /dev/null || ! command -v git &> /dev/null; then
    install_packages
fi

# ۳. پاکسازی نسخه قدیمی nooty-beta
if [ -f "/usr/local/bin/nooty-beta" ]; then
    echo -e "${YELLOW}🧹 Removing older nooty-beta binary...${NC}"
    $SUDO rm -f /usr/local/bin/nooty-beta
fi

# ۴. دانلود سورس‌کد و کامپایل هوشمند
BUILD_DIR=$(mktemp -d)
echo -e "${BLUE}📥 Fetching latest Beta code from GitHub...${NC}"

curl -fsSL "https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/beta/nooty.go?v=$(date +%s)" -o "$BUILD_DIR/nooty.go"

if [ ! -s "$BUILD_DIR/nooty.go" ]; then
    echo -e "${RED}❌ Download failed! File is empty or 404 URL.${NC}"
    rm -rf "$BUILD_DIR"
    exit 1
fi

echo -e "${BLUE}🔨 Compiling NootyCLI Beta...${NC}"
cd "$BUILD_DIR"
go mod init nooty-beta > /dev/null 2>&1
go build -ldflags="-s -w" -o nooty-beta nooty.go

# ۵. استقرار باینری در مسیر عمومی سیستم
echo -e "${BLUE}🚀 Installing binary to /usr/local/bin/nooty-beta...${NC}"
$SUDO mv nooty-beta /usr/local/bin/nooty-beta
$SUDO chmod +x /usr/local/bin/nooty-beta

# پاکسازی و اتمام
rm -rf "$BUILD_DIR"
hash -r 2>/dev/null || true

echo -e "\n${GREEN}🎉 NootyCLI Beta installed successfully!${NC}"
echo -e "${CYAN}⚡ Just type ${YELLOW}nooty-beta${CYAN} in your terminal to start!${NC}\n"
