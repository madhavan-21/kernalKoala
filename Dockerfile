# Stage 1: Build Go binary and BPF modules
FROM golang:1.24.3 AS builder

# Install build tools for BPF
RUN apt-get update && apt-get install -y \
    clang \
    llvm \
    make \
    iproute2 \
    gcc \
    libbpf-dev \
    libelf-dev \
    pkg-config \
    iputils-ping \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy everything
COPY . .

# Set default interface if not passed
ARG IFACE=lo

# Build full dev setup
RUN make dev IFACE=${IFACE}

# Stage 2: Minimal runtime image
FROM debian:bookworm-slim

# Tools for running network-related apps
RUN apt-get update && apt-get install -y \
    iproute2 \
    iputils-ping \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy binary and any BPF artifacts
WORKDIR /app
COPY --from=builder /app/bin /app/bin
COPY --from=builder /app/bpf /app/bpf

# Default binary arch = amd64 (can be overridden)
ARG ARCH=amd64
ENV ARCH=${ARCH}
ENV ENV=prod

# Entrypoint to run with interface (override with `docker run -e IFACE=eth0`)
ENV IFACE=lo
ENTRYPOINT ["/app/bin/kernelkoala-amd64"]
CMD ["lo"]
