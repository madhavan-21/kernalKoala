#!/bin/bash

# k3s Setup Script for local testing
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${YELLOW}🚀 Setting up k3s for local development...${NC}"

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check if running as root or with sudo
if [[ $EUID -ne 0 ]]; then
    echo -e "${RED}❌ This script must be run as root or with sudo${NC}"
    exit 1
fi

# Install k3s if not present
if ! command_exists k3s; then
    echo -e "${YELLOW}📦 Installing k3s...${NC}"
    curl -sfL https://get.k3s.io | sh -s - --write-kubeconfig-mode 644
else
    echo -e "${GREEN}✅ k3s is already installed${NC}"
fi

# Start k3s service
echo -e "${YELLOW}🔄 Starting k3s service...${NC}"
systemctl enable k3s
systemctl start k3s

# Wait for k3s to be ready
echo -e "${YELLOW}⏳ Waiting for k3s to be ready...${NC}"
sleep 10

# Set up kubectl access for current user
REAL_USER=${SUDO_USER:-$USER}
REAL_HOME=$(eval echo ~$REAL_USER)

echo -e "${YELLOW}🔧 Setting up kubectl access for user: ${REAL_USER}${NC}"

# Create .kube directory and copy config
sudo -u $REAL_USER mkdir -p $REAL_HOME/.kube
cp /etc/rancher/k3s/k3s.yaml $REAL_HOME/.kube/config
chown $REAL_USER:$REAL_USER $REAL_HOME/.kube/config
chmod 600 $REAL_HOME/.kube/config

# Verify k3s is working
echo -e "${YELLOW}🧪 Testing k3s installation...${NC}"
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

if kubectl cluster-info &> /dev/null; then
    echo -e "${GREEN}✅ k3s is running successfully!${NC}"
else
    echo -e "${RED}❌ k3s cluster is not responding${NC}"
    exit 1
fi

# Show cluster info
echo -e "${BLUE}📊 Cluster Information:${NC}"
kubectl cluster-info
echo ""
kubectl get nodes

# Install useful tools if not present
echo -e "${YELLOW}🛠️  Installing additional tools...${NC}"

# Install jq for JSON parsing (useful for registry verification)
if ! command_exists jq; then
    apt-get update
    apt-get install -y jq
fi

# Install curl if not present
if ! command_exists curl; then
    apt-get install -y curl
fi

echo -e "${GREEN}🎉 k3s setup completed successfully!${NC}"
echo -e "${YELLOW}💡 Next steps:${NC}"
echo -e "  1. Run ${BLUE}./build-and-push.sh${NC} to build and push your image"
echo -e "  2. Run ${BLUE}./deploy-to-k3s.sh${NC} to deploy to k3s"
echo -e "  3. Use ${BLUE}kubectl${NC} commands to manage your deployment"
echo ""
echo -e "${YELLOW}📝 Important notes:${NC}"
echo -e "  - k3s config is at: ${BLUE}/etc/rancher/k3s/k3s.yaml${NC}"
echo -e "  - User kubectl config is at: ${BLUE}$REAL_HOME/.kube/config${NC}"
echo -e "  - Local registry will run on: ${BLUE}localhost:5001${NC}"