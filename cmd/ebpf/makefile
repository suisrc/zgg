CLANG ?= clang
KERNEL_HEADERS := /usr/src/linux-headers-$(shell uname -r)
BPF_CFLAGS = -O2 -target bpf -g \
-I$(KERNEL_HEADERS)/arch/x86/include/generated/uapi \
-I/usr/include/x86_64-linux-gnu

all: capture.o monitor

capture.o: capture.c
	$(CLANG) $(BPF_CFLAGS) -c capture.c -o capture.o

monitor: monitor.c
	$(CLANG) -O2 -o monitor monitor.c -lbpf -lelf -lz

clean:
	rm -f capture.o monitor
