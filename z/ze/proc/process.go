package proc

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotRunning = errors.New("process not running")
)

type Process interface {
	Pid() int
	Start() error
	Serve() error
	Stop(time.Duration) error
	Wait(time.Duration) error
	State() string
	Restart(time.Duration) error
	String() string
}

const (
	stateRunning  = "running"
	stateStopped  = "stopped"
	stateStopping = "stopping"
)

type process0 struct {
	logger  io.Writer
	command string
	args    []string

	mu    sync.Mutex
	cmd   *exec.Cmd
	done  chan struct{}
	state string
}

// ProcessController 是一个进程控制器，提供启动、停止、重启和状态查询等功能
func NewProcess(logger io.Writer, command string, args ...string) Process {
	if logger == nil {
		logger = os.Stdout
	}
	return &process0{
		logger:  logger,
		command: command,
		args:    args,
		state:   stateStopped,
	}
}

func (p *process0) String() string {
	return p.command + " " + strings.Join(p.args, " ")
}

// Pid 返回当前进程的 PID，如果没有在运行，返回 0
func (p *process0) Pid() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.state != stateStopped {
		return p.cmd.Process.Pid
	}
	return 0
}

// Start 启动进程，异步启动，不等待
func (p *process0) Start() error {
	wait, err := p.enforce()
	if err != nil {
		return err
	}
	go wait()
	return nil
}

// Serve 启动进程， 并等待进程结束
func (p *process0) Serve() error {
	wait, err := p.enforce()
	if err != nil {
		return err
	}
	wait()
	return nil
}

// enforce 启动进程，返回一个函数用于等待进程结束
func (p *process0) enforce() (func(), error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != stateStopped {
		return nil, errors.New("process already running") // 已经在运行
	}
	// 创建启动命令
	cmd := exec.Command(p.command, p.args...)
	cmd.Stdout = p.logger
	cmd.Stderr = p.logger
	// 在非 Windows 系统上，设置 SysProcAttr 以创建新的进程组
	if attr := newSysProcAttr(); attr != nil {
		cmd.SysProcAttr = attr
	}
	// 启动进程
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// 更新状态
	p.cmd = cmd
	p.done = make(chan struct{})
	p.state = stateRunning
	done := p.done
	// 等待进程结束, 并在结束后更新状态
	return func() {
		_ = cmd.Wait()
		p.markStopped(cmd, done)
	}, nil
}

// Stop 停止进程，发送 SIGTERM 信号，等待进程结束，如果超过 5 秒还没有结束，强制杀死进程
func (p *process0) Stop(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	p.mu.Lock()
	// 如果没有在运行，直接返回错误
	if p.cmd == nil || p.state == stateStopped {
		p.mu.Unlock()
		return ErrNotRunning
	} else if p.state == stateStopping {
		p.mu.Unlock()
		return nil
	}
	// 获取当前命令，并更新状态为正在停止
	cmd := p.cmd
	done := p.done
	p.state = stateStopping
	p.mu.Unlock()
	// 在 Windows 上直接杀死进程
	if runtime.GOOS == "windows" {
		if err := cmd.Process.Kill(); err != nil {
			p.restoreRunning(cmd)
			return err
		}
		return nil
	}
	// 在 非 Windows 系统上，发送 SIGTERM 信号到进程组
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		p.restoreRunning(cmd)
		return err
	}
	// 发送 SIGTERM 信号到进程组
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		p.restoreRunning(cmd)
		return err
	}
	// 等待进程结束，如果超过 5 秒还没有结束，强制杀死进程组
	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}()
	return nil
}

// Wait 等待进程停止，超时返回错误
func (p *process0) Wait(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	p.mu.Lock()
	done := p.done
	state := p.state
	p.mu.Unlock()

	if state == stateStopped {
		return nil
	}
	if done == nil {
		return ErrNotRunning
	}
	// 等待进程停止，超时返回错误
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return errors.New("wait stop timeout")
	}
}

// State 返回当前进程状态，可能的值为 "stopped", "running", "stopping"
func (p *process0) State() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// Restart 先停止进程，再启动新进程，更新状态为 running
func (p *process0) Restart(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if err := p.Stop(timeout); err != nil && err != ErrNotRunning {
		return err
	}
	if err := p.Wait(timeout); err != nil {
		return err
	}
	return p.Start()
}

func (p *process0) markStopped(cmd *exec.Cmd, done chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == cmd {
		p.cmd = nil
		p.done = nil
		p.state = stateStopped
	}
	close(done)
}

func (p *process0) restoreRunning(cmd *exec.Cmd) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == cmd && p.state == stateStopping {
		p.state = stateRunning
	}
}
