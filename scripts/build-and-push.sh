#!/bin/bash

# Build and Push Script for kernelkoala
set -e

# Configuration
IMAGE_NAME="kernelkoala"
REGISTRY="localhost:5001"  # Local registry for k3s testing
TAG="latest"
FULL_IMAGE="${REGISTRY}/${IMAGE_NAME}:${TAG}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}🐨 Building and pushing kernelkoala Docker image...${NC}"

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check prerequisites
if ! command_exists docker; then
    echo -e "${RED}❌ Docker is not installed${NC}"
    exit 1
fi

# Start local registry if it doesn't exist
if ! docker ps | grep -q "registry:2"; then
    echo -e "${YELLOW}🚀 Starting local Docker registry...${NC}"
    docker run -d \
        --name registry \
        --restart=always \
        -p 5001:5000 \
        registry:2 || true
    
    # Wait for registry to be ready
    sleep 3
fi

echo -e "${YELLOW}🔨 Building Docker image...${NC}"
cd ../

docker build \
    --build-arg ARCH=amd64 \
    --build-arg IFACE=lo \
    -t ${IMAGE_NAME}:${TAG} \
    -t ${FULL_IMAGE} \
    .

echo -e "${YELLOW}📤 Pushing to local registry...${NC}"
docker push ${FULL_IMAGE}

echo -e "${GREEN}✅ Successfully built and pushed: ${FULL_IMAGE}${NC}"

# Verify the push
echo -e "${YELLOW}🔍 Verifying image in registry...${NC}"
curl -s http://localhost:5001/v2/_catalog | jq '.'

echo -e "${GREEN}✅ Build and push completed successfully!${NC}"
echo -e "${YELLOW}📝 Image available at: ${FULL_IMAGE}${NC}"