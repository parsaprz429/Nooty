#!/bin/bash

# 🧪 NootyCLI Beta Installer (v0.3.0 Radin Edge)
# Ecosystem: Nooty (nooty.ir) | Author: Parsa
set -e

# رنگ‌های مینیمال و شیک 🎨
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
GRAY='\033[0;90m'
BOLD='\033[1m'
NC='\033[0m'

clear
echo -e "${CYAN}${BOLD}"
echo "  _  _                 _            ___  _    ___ "
echo " | \| | ___  ___  _ |_| _ _     / C \ |  |_ _|"
echo " | .\` |/ _ \/ _ \|  _| || |    |  __/ |__| | "
echo " |_|\_|\___/\___/ \__| \_, |     \___|____|___|"
echo "                       |__/                   "
echo -e "${YELLOW}       ⚡ Radin Edge Engine (v0.3.0 Beta)${NC}\n"

# 1. تشخیص سیستم‌عامل
OS_RAW="$(uname -s)"
ARCH_RAW="$(uname -m)"
OS="Linux"

case "$OS_RAW" in
    Darwin*) OS="macOS" ;;
    CYGWIN*|MINGW*|MSYS*) OS="Windows" ;;
esac

echo -e "${GRAY}▸ Platform:${NC} ${BOLD}$OS ($ARCH_RAW)${NC}"

# مدیریت دسترسی Sudo
SUDO=""
if [ "$OS" != "Windows" ] && [ "$(id -u)" -ne 0 ]; then
    command -v sudo &>/dev/null && SUDO="sudo"
fi

# 2. بررسی و نصب سریع پیش‌نیازها
if ! command -v go &>/dev/null || ! command -v curl &>/dev/null; then
    echo -e "${YELLOW}📦 Installing required dependencies...${NC}"
    case "$OS" in
        Linux)
            if command -v apt-get &>/dev/null; then
                $SUDO apt-get update -qq && $SUDO apt-get install -y -qq curl golang-go
            elif command -v dnf &>/dev/null; then
                $SUDO dnf install -y golang curl
            elif command -v pacman &>/dev/null; then
                $SUDO pacman -Sy --noconfirm go curl
            fi
            ;;
        macOS)
            command -v brew &>/dev/null && brew install go curl
            ;;
    esac
fi

# 3. ایجاد دایرکتوری موقت
BUILD_DIR=$(mktemp -d)
trap 'rm -rf "$BUILD_DIR"' EXIT

echo -e "${CYAN}⚡ Fetching fresh source code (Bypassing GitHub CDN cache)...${NC}"

# 4. دور زدن محدودیت کَش گیت‌هاب با Timestamp نانوثانیه + هدر Cache-Control
NOCACHE_URL="https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/Beta/nooty.go?nocache=$(date +%s%N)"

if ! curl -s -H "Cache-Control: no-cache, no-store, must-revalidate" \
          -H "Pragma: no-cache" \
          -H "Expires: 0" \
          -fsSL "$NOCACHE_URL" -o "$BUILD_DIR/nooty.go"; then
    echo -e "${RED}❌ Failed to fetch latest source code from GitHub!${NC}"
    exit 1
fi

# 5. کامپایل استاندارد و سریع (Optimized Build)
echo -e "${CYAN}🔨 Compiling NootyCLI Binary...${NC}"
cd "$BUILD_DIR"
go mod init nooty-beta >/dev/null 2>&1

# کامپایل مستقیم روی nooty.go با فشرده‌سازی حداکثری (-s -w)
go build -ldflags="-s -w" -o nooty-beta nooty.go

# 6. انتقال به مسیر اجرایی سیستم
BIN_TARGET="/usr/local/bin/nooty-beta"
[ "$OS" = "Windows" ] && BIN_TARGET="/usr/bin/nooty-beta"

$SUDO mv nooty-beta "$BIN_TARGET"
$SUDO chmod +x "$BIN_TARGET"
hash -r 2>/dev/null || true

# پیام موفقیت آمیز
echo -e "\n${GREEN}${BOLD}✨ NootyCLI (v0.3.0 Radin Edge) successfully installed!${NC}"
echo -e "${CYAN}🚀 Run ${YELLOW}${BOLD}nooty-beta${CYAN} to launch the Agentic Terminal.${NC}\n"
