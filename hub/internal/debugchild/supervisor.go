// Package debugchild 负责把调试服务作为独立子进程拉起并守护。
// 日志通过 stdin 单向发送；子进程崩溃不会影响 hub。
package debugchild

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// InternalFlag 是子进程模式的命令行开关。
	InternalFlag = "--internal-debug"
	queueSize    = 1024
	maxFailures  = 5
)

// Supervisor 管理调试子进程的生命周期。
type Supervisor struct {
	exe    string
	addr   string
	logger *log.Logger

	lines chan string
	ctx   context.Context
	stop  context.CancelFunc
	done  chan struct{}

	mu    sync.Mutex
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

// New 创建守护器。exe 是当前 hub 可执行文件的路径。
func New(exe, addr string, logger *log.Logger) *Supervisor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Supervisor{
		exe:    exe,
		addr:   addr,
		logger: logger,
		lines:  make(chan string, queueSize),
		ctx:    ctx,
		stop:   cancel,
		done:   make(chan struct{}),
	}
}

// Write 实现 io.Writer。队列满时丢弃，绝不让日志拖慢 hub。
func (s *Supervisor) Write(p []byte) (int, error) {
	text := strings.TrimRight(string(p), "\r\n")
	for _, line := range strings.Split(text, "\n") {
		select {
		case s.lines <- line:
		default:
			// 调试进程处理不过来时丢弃，主进程继续工作。
		}
	}
	return len(p), nil
}

// Run 启动并守护子进程，直到 Stop 被调用。应当放在独立 goroutine 中。
func (s *Supervisor) Run() {
	defer close(s.done)

	backoff := time.Second
	failures := 0

	for s.ctx.Err() == nil {
		cmd, stdin, err := s.start()
		if err != nil {
			s.logger.Printf("调试服务启动失败: %v", err)
			failures++
			if failures >= maxFailures {
				s.logger.Printf("调试服务连续失败 %d 次，已放弃启动", failures)
				return
			}
			if !sleep(s.ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		failures = 0
		backoff = time.Second
		s.logger.Printf("调试服务已启动: http://%s/logs (pid %d)", s.addr, cmd.Process.Pid)

		waitCh := make(chan error, 1)
		go func() {
			waitCh <- cmd.Wait()
			close(waitCh)
		}()

		exited := s.pump(stdin, waitCh)

		_ = stdin.Close()
		select {
		case <-waitCh:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-waitCh
		}

		if s.ctx.Err() != nil {
			return
		}
		s.logger.Printf("调试服务退出: %v，准备重启", exited)
		if !sleep(s.ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

func (s *Supervisor) start() (*exec.Cmd, io.WriteCloser, error) {
	cmd := exec.Command(s.exe, InternalFlag, "--addr", s.addr)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("创建 stdin 失败: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("启动失败: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.stdin = stdin
	s.mu.Unlock()

	return cmd, stdin, nil
}

// pump 转发日志直到子进程退出或收到停止信号。
func (s *Supervisor) pump(stdin io.WriteCloser, waitCh <-chan error) error {
	for {
		select {
		case <-s.ctx.Done():
			return nil
		case err := <-waitCh:
			return err
		case line := <-s.lines:
			if _, err := io.WriteString(stdin, line+"\n"); err != nil {
				return fmt.Errorf("写入日志失败: %w", err)
			}
		}
	}
}

// Stop 停止子进程并等待清理完成。
func (s *Supervisor) Stop() {
	s.stop()
	<-s.done

	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}
