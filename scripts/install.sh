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

# Essential libraries
PKGS="$PKGS libelf-dev gcc-multilib libbpf-dev"

# Install kernel headers
KERNEL_HEADERS_PATH="/lib/modules/$(uname -r)/build"
if [ ! -d "$KERNEL_HEADERS_PATH" ]; then
    echo "⚙️ Kernel headers not found. Adding to install list..."
    PKGS="$PKGS linux-headers-$(uname -r)"
fi

# Fix missing asm/ headers for some distros
if [ ! -e "$KERNEL_HEADERS_PATH/include/asm/types.h" ]; then
  echo "🔧 Linking missing asm/ headers..."
  sudo ln -s ../arch/x86/include/asm "$KERNEL_HEADERS_PATH/include/asm"
fi

# Install all required packages
echo "📦 Installing packages: $PKGS"
sudo apt-get update
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y $PKGS

# Optional: clone libbpf submodule if using
if [ -f .gitmodules ]; then
    echo "🔁 Initializing submodules..."
    git submodule update --init --recursive
fi

# Debugging: Show versions
echo "🔧 Tool versions:"
clang --version || true
llc --version || true
bpftool version || true
go version

echo "🎉 Installation complete."
