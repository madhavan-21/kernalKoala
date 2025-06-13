# eBPF Packet Monitor

This project contains an eBPF Traffic Control (TC) program that captures ingress and egress TCP/UDP packets and emits their metadata (IP, port, flags, direction) via a perf event map.

---

## 📦 Structure

- `packet_monitor.bpf.c`: Core eBPF program
- `Makefile`: Build system for generating `.o` files for various CPU architectures
- `build/`: Output directory containing compiled object files (created after build)

---

## 🚀 Getting Started

### 🔧 Requirements

- `clang` with eBPF target support
- `make`
- Kernel headers installed
- Optionally: `bpftool` and `iproute2` if you plan to load it

---

## 🛠️ Building the eBPF Program

### 📌 First-time setup or after any `.bpf.c` modification:

```bash
make
```

This will:

Build .o object files for multiple architectures:

```
x86_64

aarch64 (ARM64)

riscv64

```

### 🏗️ Manual Build per Architecture

🏗️ Manual Build per Architecture

```bash

make build/packet_monitor-x86_64.o

```

### 🔄 Updating the eBPF Program


```bash

If you modify packet_monitor.bpf.c, rebuild the object files:

```bash

make

```

### 🧹 Clean Build Artifacts

To remove all compiled outputs:

```bash

make clean

```

### 📤 Loading the Program (Optional)

You can use tools like tc or bpftool to load and attach the program:

```bash

tc qdisc add dev eth0 clsact
tc filter add dev eth0 ingress bpf da obj build/packet_monitor-x86_64.o sec tc_ingress
tc filter add dev eth0 egress bpf da obj build/packet_monitor-x86_64.o sec tc_egress


Replace eth0 with your interface name.

```

### 🪪 License

This project is licensed under the terms of the GNU General Public License v2.0 (GPL-2.0).

You are free to use, modify, and distribute this software under the terms of that license.
See the LICENSE for more details.


### 🙌 Contributing


Feel free to fork, open issues, or submit pull requests. Contributions to improve architecture coverage, output formats, or loader tooling are welcome.


