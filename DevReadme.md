### 📘 DevReadme.md — Developer Guide for KernelKoala


Welcome to the development guide for the KernelKoala project! This guide walks you through setting up your environment, running tests, building Go binaries, and compiling eBPF programs.

### 🧱 Project Structure

KernelKoala/
├── api/  
├── config/          # Main Go application entry point
├── cmd/             # Main Go application entry point
├── internal/        # Internal Go packages
├── pkg/             # Shared Go packages
├── bpf/             # All eBPF programs
│   ├── network/
│   │   ├── tc.c
│   │   ├── tc.o
│   │   └── Makefile
│   └── trace/       # (optional, if added later)
├── bin/             # Output binaries after build
├── Makefile         # Root-level Makefile for Go & eBPF
└── DevReadme.md     # This file


### ⚙️ Prerequisites

```bash

Go 1.19+

clang with eBPF target support (LLVM ≥ 11)

make

Linux development environment

Kernel headers (for eBPF compilation)

Optional: bpftool, tc, bcc for eBPF interaction

```

### 🚀 Development Workflow


### 1. 🔨 Build Everything (Go + eBPF)

```bash

make dev

```

This will:


✅ Run all Go tests

✅ Build all BPF modules inside bpf/*

✅ Compile Go binary for your current CPU/OS (e.g., bin/kernelkoala-amd64)

### 2. 👨‍💻 Change eBPF Code

If you modify files like:

```bash

bpf/*

```

You must rebuild the BPF object:

``` bash

make build-bpf

```

This rebuilds .o files using the Makefiles in each bpf/<module>/.

3. 🧪 Run Tests

To only run tests:

```bash

make test

```

### 4. 💻 Run the Built Binary

After running make dev, the binary is at:

```bash

bin/kernelkoala-<arch>   # e.g. bin/kernelkoala-amd64

```

Run it directly:

```bash
./bin/kernelkoala-$(go env GOARCH)
```

### 🧼 Cleaning Up


To remove all builds and compiled eBPF artifacts:

```bash

make clean

```

### 📦 Production Build

To build for all supported architectures:

```bash
make prod
```

Outputs will be in:

```
bin/kernelkoala-amd64
bin/kernelkoala-arm64
bin/kernelkoala-riscv64

```

### 🧩 Adding New eBPF Modules
If you add a new module like bpf/{module_folder_name}/{module_eBPF_file}:

1 . Add .c file (e.g., trace.c)

2. Create a Makefile like:
```bash
BPF_SRC := $(wildcard *.c)
BPF_BIN := $(BPF_SRC:.c=)

ARCHS := x86_64 aarch64 riscv64
BUILD_DIR := build

CLANG := clang
CLANG_FLAGS := -O2 -g -Wall -target bpf

# Generate build/<arch>/<name>.o for each arch and source
BPF_OBJECTS := $(foreach arch,$(ARCHS),$(foreach bin,$(BPF_BIN),$(BUILD_DIR)/$(arch)/$(bin).o))

all: $(BPF_OBJECTS)

$(BUILD_DIR)/%/%.o: %.c
	@mkdir -p $(dir $@)
	$(CLANG) $(CLANG_FLAGS) -mcpu=v3 -c $< -o $@

clean:
	rm -rf $(BUILD_DIR)


```

3.It’s auto-included in make dev, make prod, etc.

```run setup
🟢 Dev with default (lo):

make run

```

```run setup
🔵 Dev with custom interface (e.g., eth0):

make run IFACE=eth0

```
bash
### 🪪 License

This project is licensed under an open-source license. You are free to use, modify, and distribute it.



