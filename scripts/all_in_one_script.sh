#!/bin/bash

# All-in-One Demo Script for kernelkoala
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

echo -e "${PURPLE}🐨 kernelkoala k3s Demo Setup${NC}"
echo -e "${PURPLE}================================${NC}"

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to prompt user
ask_continue() {
    read -p "$(echo -e ${YELLOW}$1 [y/N]:${NC} )" -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo -e "${BLUE}👋 Exiting...${NC}"
        exit 0
    fi
}

echo -e "${YELLOW}This script will:${NC}"
echo -e "  1. 🚀 Set up k3s (requires sudo)"
echo -e "  2. 🐳 Start a local Docker registry"
echo -e "  3. 🔨 Build your kernelkoala Docker image"
echo -e "  4. 📤 Push the image to the local registry"
echo -e "  5. 🚀 Deploy kernelkoala to k3s"
echo -e "  6. 📊 Show the running application"
echo ""

ask_continue "Do you want to proceed with the full demo setup?"

# Step 1: Setup k3s
echo -e "${PURPLE}=== Step 1: Setting up k3s ===${NC}"
if ! command_exists k3s; then
    echo -e "${YELLOW}k3s not found. Installing...${NC}"
    ask_continue "This requires sudo privileges. Continue?"
    
    if [[ $EUID -ne 0 ]]; then
        echo -e "${YELLOW}Re-running with sudo...${NC}"
        exec sudo $0 "$@"
    fi
    
    curl -sfL https://get.k3s.io | sh -s - --write-kubeconfig-mode 644
    systemctl enable k3s
    systemctl start k3s
    
    # Set up kubectl for user
    REAL_USER=${SUDO_USER:-$USER}
    REAL_HOME=$(eval echo ~$REAL_USER)
    sudo -u $REAL_USER mkdir -p $REAL_HOME/.kube
    cp /etc/rancher/k3s/k3s.yaml $REAL_HOME/.kube/config
    chown $REAL_USER:$REAL_USER $REAL_HOME/.kube/config
    chmod 600 $REAL_HOME/.kube/config
else
    echo -e "${GREEN}✅ k3s is already installed${NC}"
    if [[ $EUID -ne 0 ]]; then
        sudo systemctl start k3s || true
    else
        systemctl start k3s || true
    fi
fi

# Switch back to regular user context for remaining steps
if [[ $EUID -eq 0 ]] && [[ -n ${SUDO_USER} ]]; then
    echo -e "${YELLOW}Switching back to user context...${NC}"
    exec sudo -u $SUDO_USER $0 --continue-as-user "$@"
fi

# Check if we're continuing as user
if [[ "$1" == "--continue-as-user" ]]; then
    shift
fi

# Set up environment
export KUBECONFIG=${KUBECONFIG:-$HOME/.kube/config}
if [[ ! -f $KUBECONFIG ]] && [[ -f /etc/rancher/k3s/k3s.yaml ]]; then
    export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
fi

sleep 5

# Step 2: Start local registry
echo -e "${PURPLE}=== Step 2: Starting local Docker registry ===${NC}"
if ! docker ps | grep -q "registry:2"; then
    echo -e "${YELLOW}Starting local Docker registry...${NC}"
    docker run -d \
        --name registry \
        --restart=always \
        -p 5001:5000 \
        registry:2 || true
    sleep 3
else
    echo -e "${GREEN}✅ Local registry is already running${NC}"
fi

# Step 3 & 4: Build and push image
echo -e "${PURPLE}=== Step 3-4: Building and pushing image ===${NC}"
IMAGE_NAME="kernelkoala"
REGISTRY="localhost:5001"
TAG="latest"
FULL_IMAGE="${REGISTRY}/${IMAGE_NAME}:${TAG}"

echo -e "${YELLOW}Building Docker image...${NC}"
cd ../
docker build \
    --build-arg ARCH=amd64 \
    --build-arg IFACE=lo \
    -t ${IMAGE_NAME}:${TAG} \
    -t ${FULL_IMAGE} \
    .

echo -e "${YELLOW}Pushing to local registry...${NC}"
docker push ${FULL_IMAGE}

# Configure k3s for local registry
echo -e "${YELLOW}Configuring k3s for local registry...${NC}"
sudo tee /etc/rancher/k3s/registries.yaml > /dev/null <<EOF
mirrors:
  "localhost:5001":
    endpoint:
      - "http://localhost:5001"
configs:
  "localhost:5001":
    insecure_skip_verify: true
EOF

sudo systemctl restart k3s
sleep 10

# Step 5: Deploy to k3s
echo -e "${PURPLE}=== Step 5: Deploying to k3s ===${NC}"
NAMESPACE="kernelkoala"

kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -

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

echo -e "${YELLOW}Waiting for deployment to be ready...${NC}"
kubectl wait --for=condition=available --timeout=300s deployment/kernelkoala -n ${NAMESPACE}

# Step 6: Show results
echo -e "${PURPLE}=== Step 6: Demo Results ===${NC}"
echo -e "${GREEN}🎉 kernelkoala is now running on k3s!${NC}"
echo ""

echo -e "${BLUE}📊 Pod Status:${NC}"
kubectl get pods -n ${NAMESPACE} -o wide
echo ""

echo -e "${BLUE}🔍 Service Status:${NC}"
kubectl get svc -n ${NAMESPACE}
echo ""

echo -e "${BLUE}📋 Recent Logs:${NC}"
kubectl logs -n ${NAMESPACE} -l app=kernelkoala --tail=10
echo ""

echo -e "${GREEN}✅ Demo completed successfully!${NC}"
echo -e "${YELLOW}💡 Useful commands:${NC}"
echo -e "  View logs: ${BLUE}kubectl logs -n ${NAMESPACE} -l app=kernelkoala -f${NC}"
echo -e "  Get pods: ${BLUE}kubectl get pods -n ${NAMESPACE}${NC}"
echo -e "  Shell into pod: ${BLUE}kubectl exec -n ${NAMESPACE} -it \$(kubectl get pods -n ${NAMESPACE} -l app=kernelkoala -o jsonpath='{.items[0].metadata.name}') -- /bin/bash${NC}"
echo -e "  Cleanup: ${BLUE}kubectl delete namespace ${NAMESPACE}${NC}"