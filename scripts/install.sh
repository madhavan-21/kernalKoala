#!/bin/bash

set -e

echo "📦 Starting installation of Go and eBPF dependencies..."

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# 1. Install Go dependencies
echo "🔍 Checking Go dependencies..."
if [ ! -f go.mod ]; then
    echo "❌ go.mod not found. Are you in the project root?"
    exit 1
fi

echo "📦 Tidying Go modules..."
go mod tidy

# 2. Install eBPF build tools
echo "🔍 Checking and installing eBPF tools..."

PKGS=""
if ! command_exists clang; then PKGS="$PKGS clang"; fi
if ! command_exists llc; then PKGS="$PKGS llvm"; fi
if ! command_exists make; then PKGS="$PKGS make"; fi
if ! command_exists bpftool; then PKGS="$PKGS bpftool"; fi

# Install kernel headers
KERNEL_HEADERS_PATH="/lib/modules/$(uname -r)/build"
if [ ! -d "$KERNEL_HEADERS_PATH" ]; then
    echo "⚙️ Kernel headers not found. Adding to install list..."
    PKGS="$PKGS linux-headers-$(uname -r)"
fi

if [ ! -e "/lib/modules/$(uname -r)/build/include/asm/types.h" ]; then
  echo "🔧 Linking missing asm/ headers..."
  sudo ln -s ../arch/x86/include/asm /lib/modules/$(uname -r)/build/include/asm
fi

if [ -n "$PKGS" ]; then
    echo "📦 Installing packages: $PKGS"
    sudo apt update
    sudo apt install -y $PKGS
else
    echo "✅ All eBPF dependencies already installed."
fi

echo "🎉 Installation complete."
