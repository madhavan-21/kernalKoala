#!/bin/bash

set -e

echo "📦 Starting installation of Go and eBPF dependencies..."

# Check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# 1. Check Go dependencies
echo "🔍 Checking Go dependencies..."
if [ ! -f go.mod ]; then
    echo -e "\e[1;31m❌ go.mod not found. Are you in the project root?\e[0m"
    exit 1
fi

echo "📦 Tidying Go modules..."
go mod tidy

# 2. Prepare package list
echo "🔍 Checking and installing eBPF tools..."

PKGS=""
if ! command_exists clang; then PKGS+=" clang"; fi
if ! command_exists llc; then PKGS+=" llvm"; fi
if ! command_exists make; then PKGS+=" make"; fi
if ! command_exists bpftool; then PKGS+=" bpftool"; fi

# Add essential packages
PKGS+=" libelf-dev gcc-multilib libbpf-dev"

# Check kernel headers
KERNEL_HEADERS_PATH="/lib/modules/$(uname -r)/build"
if [ ! -d "$KERNEL_HEADERS_PATH" ]; then
    echo "⚙️ Kernel headers not found. Adding to install list..."
    PKGS+=" linux-headers-$(uname -r)"
fi

# Optional fix: switch to valid Ubuntu mirror if broken
if grep -q "mirror+file:/etc/apt/apt-mirrors.txt" /etc/apt/sources.list 2>/dev/null; then
    echo "⚠️  Detected broken mirror. Replacing with official Ubuntu mirror..."
    sudo sed -i 's|mirror+file:/etc/apt/apt-mirrors.txt|http://archive.ubuntu.com/ubuntu|g' /etc/apt/sources.list
    sudo apt-get update
fi

# 3. Attempt installation with retry logic
echo "📦 Installing packages: $PKGS"
if ! sudo DEBIAN_FRONTEND=noninteractive apt-get install -y $PKGS; then
    echo "⚠️  First install failed. Retrying with --fix-missing..."
    sudo apt-get update
    sudo DEBIAN_FRONTEND=noninteractive apt-get install -y $PKGS --fix-missing
fi

# 4. Fix missing asm/ headers if needed
if [ -d "$KERNEL_HEADERS_PATH/include" ] && [ ! -e "$KERNEL_HEADERS_PATH/include/asm/types.h" ]; then
  echo "🔧 Linking missing asm/ headers..."
  sudo ln -sf ../arch/x86/include/asm "$KERNEL_HEADERS_PATH/include/asm"
fi

# 5. Initialize git submodules (if any)
if [ -f .gitmodules ]; then
    echo "🔁 Initializing git submodules..."
    git submodule update --init --recursive
fi

# 6. Print versions
echo "🔧 Tool versions:"
clang --version || true
llc --version || true
bpftool version || true
go version

# 7. Done
echo -e "\e[1;32m🎉 Installation complete.\e[0m"

# Fancy banner
echo -e "\e[1;35m"
echo "╔═══════════════════════════════════════════════════════════════════════════════╗"
echo "║                                                                               ║"
echo "║   🚀  \e[1;36mkernelKoala: Setup & Build Completed Successfully\e[1;35m   🚀  ║"
echo "║                                                                               ║"
echo "╚═══════════════════════════════════════════════════════════════════════════════╝"
echo -e "\e[0m"
