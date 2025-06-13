ARCHS = x86_64 aarch64 riscv64
BPF_SRC = tc.c
BUILD_DIR = build

CLANG ?= clang
KERNEL_HEADERS := /lib/modules/$(shell uname -r)/build

# Use arch-specific mapping
ARCH_DIR_MAP_x86_64 = x86
ARCH_DIR_MAP_aarch64 = arm64
ARCH_DIR_MAP_riscv64 = riscv

define get_arch_dir
$(ARCH_DIR_MAP_$(1))
endef

COMMON_FLAGS = -g -O2 -target bpf

.PHONY: all clean

all: $(ARCHS:%=$(BUILD_DIR)/tc-%.o)

$(BUILD_DIR)/tc-%.o: $(BPF_SRC)
	@mkdir -p $(BUILD_DIR)
	@echo "🔨 Building for arch: $*"
	ARCH_DIR=$(call get_arch_dir,$*) && \
	$(CLANG) $(COMMON_FLAGS) \
		-I$(KERNEL_HEADERS)/include \
		-I$(KERNEL_HEADERS)/include/uapi \
		-I$(KERNEL_HEADERS)/arch/$$ARCH_DIR/include \
		-I$(KERNEL_HEADERS)/arch/$$ARCH_DIR/include/uapi \
		-I$(KERNEL_HEADERS)/arch/$$ARCH_DIR/include/generated \
		-D__TARGET_ARCH_$* -c $(BPF_SRC) -o $@
	
clean:
	rm -rf $(BUILD_DIR)
