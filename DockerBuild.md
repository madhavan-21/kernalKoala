# 🐨 KernelKoala

**KernelKoala** is an eBPF-based packet observer designed to monitor network traffic directly from the Linux kernel. It uses eBPF to capture packets on the specified network interface and gives you real-time visibility into system-level networking events.

> ⚠️ Requires Linux with eBPF support and privileged permissions to run.

---

## 🚀 Features

- ✅ Real-time packet sniffing using eBPF
- 📶 Interface-based filtering (via `IFACE` env)
- 🧪 Colorful Koala ASCII logging
- 🛡️ Runs with `rlimit` and `capabilities` to ensure performance
- 🐳 Easy Dockerized deployment

---

## 🛠️ Requirements

- Docker (with Linux backend)
- Linux kernel 5.x+ with eBPF enabled
- Root privileges or `--privileged` container
- Target network interface (e.g., `eth0`, `wlan0`, `ens33`)

---

## ⚡ Quick Start (Docker)

### 🔁 Run with full privileges:

🧪 Developer Mode (Build & Run)

🧱 Build locally using Docker:

```bash
docker build -t kernelkoala .
```

🏃 Run after build:

```bash
docker run --rm --privileged -e IFACE=eth0 kernelkoala
```

Replace eth0 with your actual interface name.

🧵 Environment Variables

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

🐙 Kubernetes Support (Minikube/Cluster)


🧹 Troubleshooting

```bash
❌ Failed to raise rlimit: operation not permitted
→ Use --privileged or set --cap-add=SYS_RESOURCE --ulimit memlock=-1:-1.

interface not found
→ Check available interfaces using ip link or ifconfig.

```

👨‍💻 Author

Maintained by Madhavan S.
Inspired by kernel-powered observability 🐧🧠


📄 License

MIT License. See LICENSE for details.


