package proc_test

import (
	"os"
	"testing"
	"time"

	"github.com/suisrc/zgg/z/ze/proc"
)

// go test -v z/ze/proc/process_test.go -run TestProcess1

func TestProcess1(t *testing.T) {
	pp := proc.NewProcess(nil, "bash", "-c", "while true; do head -c 16 /dev/urandom | base64; sleep 1; done")
	if err := pp.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}
	t.Logf("process started with PID %d", pp.Pid())

	time.Sleep(5 * time.Second)

	if err := pp.Stop(0); err != nil {
		t.Fatalf("failed to stop process: %v", err)
	}
	t.Log("process stopped")

	if err := pp.Wait(10 * time.Second); err != nil {
		t.Fatalf("failed to wait for process to stop: %v", err)
	}
	t.Log("process exited")
}

// 测试主线程关闭， 子线程是否能正确退出

// go test -v z/ze/proc/process_test.go -run TestProcess2
func TestProcess2(t *testing.T) {
	pp := proc.NewProcess(nil, "bash", "-c", "while true; do head -c 16 /dev/urandom | base64; sleep 1; done")
	if err := pp.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}
	t.Logf("process started with PID %d", pp.Pid())

	time.Sleep(2 * time.Second)

	os.Exit(0)
}
