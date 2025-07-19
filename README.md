**KernelKoala - eBPF Network Traffic Capture Tool**

![Custom Image](/config/assest/kernalkoala.png)


A high-performance network traffic analyzer built with eBPF in Go. It supports TCP/UDP/ICMP packet monitoring with optional DNS resolution, multi-interface support, and per-interface statistics via tc (Traffic Control) hooks.

📦 Features

 **🧠 eBPF programs for ingress and egress traffic**

 **🚀 High-performance packet capture via perf ring buffer**

 **🧵 Worker pool architecture with batching**

 **🌐 Optional DNS resolution with caching**

 **🔄 Supports multiple interfaces via config**

 **📊 Live statistics (processed/dropped/queue full)**

 **🛑 Graceful shutdown handling via signals**

 **🧩 Extensible and modular design**

🔧 Requirements

 => Linux Kernel ≥ 5.4 (with eBPF and tc support)
 => Go ≥ 1.20
 => Dependencies managed in go.mod:
```
    github.com/cilium/ebpf v0.12.3
    github.com/vishvananda/netlink v1.1.0
    github.com/miekg/dns v1.1.55
    golang.org/x/sys v0.13.0
```

🚀 Quick Run

Follow these steps to quickly set up, build, and run kernelkoala:

***1. 📥 Install Dependencies***
Install Go and eBPF build tools (via scripts/install.sh):

```bash
make install
```

***2. 🧪 Run Tests and Build Everything (Go + eBPF)***

```bash
make
```

***3. 🚀 Run the App***
Run the app on the loopback interface (lo) by default:

```bash
make run
```

Or specify a different interface (e.g., eth0):

```bash
IFACE=eth0 make run
```

***4. 🏗️ Build Production Binaries (All Architectures)***
```bash
make prod
```

***5. 🧹 Clean All Builds***
```bash
make clean
```

***Note: All binaries will be placed under the bin/ directory.***



🛠️ Build Instructions

***1. Build the eBPF Object***

```bash
cd bpf/network
make
# This should output tc-x86_64.o (or aarch64/riscv64 depending on arch)
```
Note: Ensure your kernel headers and LLVM/Clang are installed for eBPF compilation.

***2. Build the Go Application***
```bash
go build -o kernelKoala ./cmd/kernelKoala
```

🚀 Run the Application

***Basic Usage***

```
sudo ./kernelKoala --iface eth0 --dns=true
```

***Environment-based***

```
export IFACE=eth0
export LOOPBACK=false
sudo ./kernelKoala
```

***⚙️ CLI Flags & Environment Variables***

```table
| Flag/Env                  | Description                            | Default                 |
| ------------------------- | -------------------------------------- | ----------------------- |
| `--iface` / `IFACE`       | Network interface to monitor           | `lo`                    |
| `--loopback` / `LOOPBACK` | Drop loopback traffic (`true`/`false`) | `true`                  |
| `--workers`               | Number of worker goroutines            | `NumCPU`                |
| `--buffer`                | Event channel buffer size              | `100000`                |
| `--batch`                 | Batch size per worker                  | `100`                   |
| `--dns`                   | Enable DNS resolution                  | `false`                 |
| `--dns-timeout`           | Timeout per DNS query                  | `500ms`                 |
| `--dns-cache-size`        | Max DNS cache entries                  | `10000`                 |
| `--dns-cache-ttl`         | TTL per DNS cache entry                | `5m`                    |
| `--dns-servers`           | DNS servers to use (comma-separated)   | `8.8.8.8:53,1.1.1.1:53` |
```

📦 Output Example

```bash
Ingress TCP: src=192.168.1.10(myhost.com):443 -> dst=192.168.1.5(:-):53820 | flags=0x10([ACK]) | iface=eth0
Egress UDP: src=192.168.1.5(:-):56000 -> dst=8.8.8.8(dns.google):53 | flags=NONE | iface=eth0
```

📊 Stats

```bash
Every 10 seconds, logs:

```bash
Stats - Processed: 15000, Dropped: 0, Queue Full: 0
```

🧼 Graceful Shutdown

```bash
Handles SIGINT and SIGTERM, stops all goroutines, cleans up tc filters, and closes perf readers.

```

📂 Project Structure

```bash
cmd/kernelKoala/main.go         # Entry point
internal/network/               # Capture logic, DNS resolver, workers
bpf/network/                    # eBPF program (.c and .o files)
internal/logger/                # Logger wrapper (assumed custom)
```

🧪 Testing

Use test environments like minikube, Docker, or local interfaces:

```bash
curl google.com
ping 1.1.1.1
dig google.com
```

To trigger traffic and see real-time capture.

📌 Notes

***Requires root privileges to attach eBPF programs to interfaces.***
***Ensure ulimit -l is sufficient for memlock (auto raised in code).***
***Use ethtool, ip a, or ifconfig to find valid interfaces.***

🧠 Inspired by

```bash
Cilium

BCC Tools (tcplife, tcptop)

Netshoot

```

🧑‍💻 Author
 
***Maintained by Madhavan S.***
💬 For questions, feel free to ask here.

