#!/bin/bash

# Deploy to k3s Script for kernelkoala
set -e

# Configuration
IMAGE_NAME="kernelkoala"
REGISTRY="localhost:5001"
TAG="latest"
FULL_IMAGE="${REGISTRY}/${IMAGE_NAME}:${TAG}"
NAMESPACE="kernelkoala"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${YELLOW}🐨 Deploying kernelkoala to k3s...${NC}"

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check prerequisites
if ! command_exists kubectl; then
    echo -e "${RED}❌ kubectl is not installed${NC}"
    exit 1
fi

if ! command_exists k3s; then
    echo -e "${RED}❌ k3s is not installed${NC}"
    echo -e "${YELLOW}💡 Install k3s with: curl -sfL https://get.k3s.io | sh -${NC}"
    exit 1
fi

# Check if k3s is running
if ! systemctl is-active --quiet k3s; then
    echo -e "${RED}❌ k3s is not running${NC}"
    echo -e "${YELLOW}💡 Start k3s with: sudo systemctl start k3s${NC}"
    exit 1
fi

# Set up kubectl config for k3s
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

echo -e "${YELLOW}🔧 Setting up k3s for local registry...${NC}"

# Configure k3s to use insecure local registry
sudo tee /etc/rancher/k3s/registries.yaml > /dev/null <<EOF
mirrors:
  "localhost:5001":
    endpoint:
      - "http://localhost:5001"
configs:
  "localhost:5001":
    insecure_skip_verify: true
EOF

echo -e "${YELLOW}🔄 Restarting k3s to apply registry config...${NC}"
sudo systemctl restart k3s
sleep 10

echo -e "${YELLOW}📦 Creating namespace...${NC}"
kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -

echo -e "${YELLOW}🚀 Deploying kernelkoala...${NC}"

# Create deployment manifest
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kernelkoala
  namespace: ${NAMESPACE}
  labels:
    app: kernelkoala
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kernelkoala
  template:
    metadata:
      labels:
        app: kernelkoala
    spec:
      containers:
      - name: kernelkoala
        image: ${FULL_IMAGE}
        imagePullPolicy: Always
        env:
        - name: IFACE
          value: "lo"
        - name: ENV
          value: "prod"
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "500m"
        securityContext:
          capabilities:
            add:
            - NET_ADMIN
            - SYS_ADMIN
          privileged: true
      hostNetwork: true
      tolerations:
      - operator: Exists
---
apiVersion: v1
kind: Service
metadata:
  name: kernelkoala-service
  namespace: ${NAMESPACE}
spec:
  selector:
    app: kernelkoala
  ports:
  - port: 8080
    targetPort: 8080
    name: http
  type: ClusterIP
EOF

echo -e "${YELLOW}⏳ Waiting for deployment to be ready...${NC}"
kubectl wait --for=condition=available --timeout=300s deployment/kernelkoala -n ${NAMESPACE}

echo -e "${GREEN}✅ Deployment successful!${NC}"

# Show status
echo -e "${BLUE}📊 Deployment Status:${NC}"
kubectl get pods -n ${NAMESPACE} -o wide

echo -e "${BLUE}🔍 Service Status:${NC}"
kubectl get svc -n ${NAMESPACE}

echo -e "${BLUE}📋 Recent logs:${NC}"
kubectl logs -n ${NAMESPACE} -l app=kernelkoala --tail=20

echo -e "${GREEN}🎉 kernelkoala is now running on k3s!${NC}"
echo -e "${YELLOW}💡 Useful commands:${NC}"
echo -e "  View logs: ${BLUE}kubectl logs -n ${NAMESPACE} -l app=kernelkoala -f${NC}"
echo -e "  Get pods: ${BLUE}kubectl get pods -n ${NAMESPACE}${NC}"
echo -e "  Describe pod: ${BLUE}kubectl describe pod -n ${NAMESPACE} -l app=kernelkoala${NC}"
echo -e "  Delete deployment: ${BLUE}kubectl delete namespace ${NAMESPACE}${NC}"