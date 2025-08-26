#!/bin/bash

# Cleanup Script for kernelkoala k3s deployment
set -e

# Configuration
NAMESPACE="kernelkoala"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${YELLOW}🧹 Cleaning up kernelkoala deployment...${NC}"

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

if ! command_exists kubectl; then
    echo -e "${RED}❌ kubectl is not installed${NC}"
    exit 1
fi

# Set up kubectl config for k3s
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

echo -e "${YELLOW}🗑️  Removing kernelkoala deployment...${NC}"

# Delete the namespace (this will delete everything in it)
if kubectl get namespace ${NAMESPACE} &> /dev/null; then
    kubectl delete namespace ${NAMESPACE}
    echo -e "${GREEN}✅ Namespace '${NAMESPACE}' deleted${NC}"
else
    echo -e "${YELLOW}⚠️  Namespace '${NAMESPACE}' not found${NC}"
fi

echo -e "${YELLOW}🐳 Stopping local Docker registry...${NC}"
if docker ps -q -f name=registry | grep -q .; then
    docker stop registry
    docker rm registry
    echo -e "${GREEN}✅ Local registry stopped and removed${NC}"
else
    echo -e "${YELLOW}⚠️  Local registry not running${NC}"
fi

echo -e "${YELLOW}🧽 Cleaning up Docker images...${NC}"
# Remove local kernelkoala images
docker images | grep kernelkoala | awk '{print $3}' | xargs -r docker rmi -f

echo -e "${GREEN}🎉 Cleanup completed!${NC}"

# Option to completely remove k3s
read -p "$(echo -e ${YELLOW}Do you want to completely uninstall k3s? [y/N]:${NC} )" -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}🔥 Uninstalling k3s completely...${NC}"
    if [[ $EUID -ne 0 ]]; then
        echo -e "${RED}❌ Need sudo privileges to uninstall k3s${NC}"
        echo -e "${YELLOW}💡 Run: sudo /usr/local/bin/k3s-uninstall.sh${NC}"
    else
        if [ -f /usr/local/bin/k3s-uninstall.sh ]; then
            /usr/local/bin/k3s-uninstall.sh
            echo -e "${GREEN}✅ k3s completely uninstalled${NC}"
        else
            echo -e "${YELLOW}⚠️  k3s uninstall script not found${NC}"
        fi
    fi
else
    echo -e "${BLUE}ℹ️  k3s is still installed and running${NC}"
    echo -e "${YELLOW}💡 To manually uninstall k3s later: sudo /usr/local/bin/k3s-uninstall.sh${NC}"
fi