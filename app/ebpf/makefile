CLANG ?= clang
LD ?= ld
KERNEL_HEADERS := /usr/src/linux-headers-$(shell uname -r)
BPF_CFLAGS = -O2 -target bpf -g \
-D__TARGET_ARCH_x86 \
-I$(KERNEL_HEADERS)/arch/x86/include/generated/uapi \
-I/usr/include/x86_64-linux-gnu

all: ebpf_capture.o ebpf_capture_blob.o monitor

ebpf_capture.o: ebpf_capture.c
	$(CLANG) $(BPF_CFLAGS) -c ebpf_capture.c -o ebpf_capture.o

ebpf_capture_blob.o: ebpf_capture.o
	$(LD) -r -z noexecstack -b binary ebpf_capture.o -o ebpf_capture_blob.o

# monitor 默认不启用 -static：
# 1. 当前系统的 libpcap.a 会继续依赖 dbus 静态符号，直接静态链接会出现 undefined reference；
# 2. glibc 名字解析路径在静态链接下还会额外带来运行时兼容警告。
# 如果后续必须做全静态，需要单独准备裁剪过的静态 libpcap 及完整静态依赖链。
monitor: monitor.c ebpf_capture_blob.o
	$(CLANG) -O2 -o monitor monitor.c ebpf_capture_blob.o -lbpf -lelf -lz

clean:
	rm -f ebpf_capture.o ebpf_capture_blob.o monitor
