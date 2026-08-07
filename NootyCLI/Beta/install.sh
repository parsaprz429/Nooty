#!/bin/bash

# 🧪 NootyCLI Beta Smart Installer & Auto-Updater
# Author: Parsa (Nooty Ecosystem)

echo "🚀 Starting NootyCLI Beta Installation / Update..."

# ۱. پاکسازی نسخه قبلی Beta برای جلوگیری از تداخل
echo "🧹 Checking and removing old Beta installation..."
if [ -f "/usr/local/bin/nooty-beta" ]; then
    sudo rm -f /usr/local/bin/nooty-beta
    echo "✅ Old nooty-beta binary removed successfully."
fi

# ۲. بررسی و نصب هوشمند Go در صورت عدم وجود
if ! command -v go &> /dev/null; then
    echo "⚠️ Go language compiler is not installed."
    echo "📦 Attempting to install Go automatically..."
    
    if command -v apt &> /dev/null; then
        sudo apt update && sudo apt install -y golang-go
    elif command -v brew &> /dev/null; then
        brew install go
    elif command -v dnf &> /dev/null; then
        sudo dnf install -y golang
    elif command -v pacman &> /dev/null; then
        sudo pacman -S --noconfirm go
    else
        echo "❌ Unable to install Go automatically. Please install Go manually and re-run."
        exit 1
    fi
fi

# تایید مجدد وجود Go
if ! command -v go &> /dev/null; then
    echo "❌ Go installation failed or PATH not updated. Please restart terminal and try again."
    exit 1
fi

# ۳. ایجاد پوشه موقت برای کامپایل
BUILD_DIR=$(mktemp -d)
echo "📥 Fetching latest Beta source code from GitHub..."

# دانلود مستقیم کد بتا از پوشه NootyCLI/beta/nooty.go
curl -fsSL "https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/beta/nooty.go?v=$(date +%s)" -o "$BUILD_DIR/nooty.go"

if [ ! -s "$BUILD_DIR/nooty.go" ]; then
    echo "❌ Download failed! Please check your network connection."
    rm -rf "$BUILD_DIR"
    exit 1
fi

# ۴. کامپایل ایجنت نسخه بتا
echo "🔨 Compiling NootyCLI Beta..."
cd "$BUILD_DIR" || exit
go mod init nooty-beta > /dev/null 2>&1
go get github.com/fatih/color > /dev/null 2>&1
go build -ldflags="-s -w" -o nooty-beta nooty.go

if [ $? -eq 0 ]; then
    # ۵. انتقال فایل اجرایی جدید به مسیر سراسری
    echo "📦 Installing new binary to /usr/local/bin/nooty-beta..."
    sudo mv nooty-beta /usr/local/bin/nooty-beta
    sudo chmod +x /usr/local/bin/nooty-beta
    
    # پاکسازی کش ترمینال (Hash Reset)
    hash -r 2>/dev/null || true
    rm -rf "$BUILD_DIR"

    echo ""
    echo "🎉 NootyCLI Beta updated successfully!"
    echo "⚡ Test it now by running: nooty-beta"
else
    echo "❌ Compilation failed. Check your Go code for syntax errors."
    rm -rf "$BUILD_DIR"
    exit 1
fi
