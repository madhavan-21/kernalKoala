# kernelkoala k3s Deployment Scripts

This collection of scripts helps you build, push, and deploy your kernelkoala Docker image to a local k3s cluster for testing.

## 🚀 Quick Start (Recommended)

For a complete end-to-end demo, just run:

```bash
chmod +x demo.sh
./demo.sh
```

This will automatically:
- Install and configure k3s
- Set up a local Docker registry
- Build and push your image
- Deploy to k3s
- Show the running application

## 📁 Script Overview

### 1. `setup-k3s.sh`
Sets up k3s for local development (requires sudo)

```bash
sudo ./setup-k3s.sh
```

**What it does:**
- Installs k3s if not present
- Configures kubectl access for current user
- Installs useful tools (jq, curl)
- Verifies the installation

### 2. `build-and-push.sh`
Builds and pushes your Docker image to a local registry

```bash
./build-and-push.sh
```

**What it does:**
- Starts a local Docker registry on port 5001
- Builds the kernelkoala image
- Pushes to localhost:5001/kernelkoala:latest
- Verifies the push

### 3. `deploy-to-k3s.sh`
Deploys kernelkoala to your k3s cluster

```bash
./deploy-to-k3s.sh
```

**What it does:**
- Configures k3s to use the local registry
- Creates the kernelkoala namespace
- Deploys the application with proper security context
- Sets up a service
- Shows logs and status

### 4. `cleanup.sh`
Cleans up the deployment and optionally removes k3s

```bash
./cleanup.sh
```

**What it does:**
- Removes the kernelkoala namespace and deployment
- Stops the local Docker registry
- Cleans up Docker images
- Optionally uninstalls k3s completely

### 5. `demo.sh`
All-in-one script that runs the complete workflow

```bash
./demo.sh
```

## 📋 Prerequisites

- Docker installed and running
- Linux system (tested on Ubuntu/Debian)
- Sudo access (for k3s installation)
- Internet connection (for downloading k3s)

## 🔧 Step-by-Step Usage

If you prefer to run each step manually:

### Step 1: Set up k3s
```bash
sudo ./setup-k3s.sh
```

### Step 2: Build and push image
```bash
./build-and-push.sh
```

### Step 3: Deploy to k3s
```bash
./deploy-to-k3s.sh
```

### Step 4: Monitor your application
```bash
# View pods
kubectl get pods -n kernelkoala

# View logs
kubectl logs -n kernelkoala -l app=kernelkoala -f

# Get shell access
kubectl exec -n kernelkoala -it $(kubectl get pods -n kernelkoala -l app=kernelkoala -o jsonpath='{.items[0].metadata.name}') -- /bin/bash
```

### Step 5: Clean up when done
```bash
./cleanup.sh
```

## ⚙️ Configuration

### Docker Build Arguments
You can customize the build by setting environment variables:

```bash
# For different architecture
ARCH=arm64 ./build-and-push.sh

# For different interface
IFACE=eth0 ./deploy-to-k3s.sh
```

### Registry Configuration
The scripts use `localhost:5001` as the local registry. You can modify this in each script if needed.

### k3s Configuration
The deployment uses:
- Privileged containers (required for BPF operations)
- Host network access
- NET_ADMIN and SYS_ADMIN capabilities

## 🔍 Troubleshooting

### Common Issues

1. **k3s won't start**
   ```bash
   sudo systemctl status k3s
   sudo journalctl -u k3s -f
   ```

2. **Image pull failures**
   - Ensure the local registry is running: `docker ps | grep registry`
   - Check k3s registry config: `cat /etc/rancher/k3s/registries.yaml`

3. **Pod fails to start**
   ```bash
   kubectl describe pod -n kernelkoala -l app=kernelkoala
   kubectl logs -n kernelkoala -l app=kernelkoala
   ```

4. **Permission issues**
   - Make scripts executable: `chmod +x *.sh`
   - k3s setup requires sudo privileges

### Verification Commands

```bash
# Check k3s cluster
kubectl cluster-info
kubectl get nodes

# Check local registry
curl -s http://localhost:5001/v2/_catalog

# Check deployment
kubectl get all -n kernelkoala
```

## 🎯 What's Deployed

The deployment creates:

- **Namespace**: `kernelkoala`
- **Deployment**: Single replica with privileged containers
- **Service**: ClusterIP service on port 8080
- **Security Context**: Full privileges for BPF operations

The container runs with:
- Environment variables: `IFACE=lo`, `ENV=prod`
- Host network access
- Required capabilities for network operations

## 🧹 Clean Up

To completely remove everything:

```bash
./cleanup.sh
# Answer 'y' when asked about removing k3s
```

Or manually:

```bash
kubectl delete namespace kernelkoala
docker stop registry && docker rm registry
sudo /usr/local/bin/k3s-uninstall.sh  # Complete k3s removal
```