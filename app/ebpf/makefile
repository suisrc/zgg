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

monitor: monitor.c ebpf_capture_blob.o
	$(CLANG) -O2 -o monitor monitor.c ebpf_capture_blob.o -lbpf -lpcap -lelf -lz

clean:
	rm -f ebpf_capture.o ebpf_capture_blob.o monitor
