package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os/exec"
	"syscall"
	"time"

	"github.com/d--j/go-milter/integration"
)

func Build(goDir string, output string) error {
	cmd := exec.Command("go", "build", "-gcflags=all=-l", "-o", output, goDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("%s", out)
	}
	return err
}

func WaitForPort(ctx context.Context, port uint16) error {
	for i := 0; i < 1200; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		time.Sleep(250 * time.Millisecond)
		conn, err := net.Dial("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			_ = conn.Close()
			return nil
		}
	}
	return fmt.Errorf("timeout waiting for port %d to get ready", port)
}

func IsExpectedExitErr(err error) bool {
	if err == nil {
		return true
	}
	var e *exec.ExitError
	if errors.As(err, &e) {
		if e.Success() || e.ExitCode() == integration.ExitSkip {
			return true
		}
		status := e.Sys().(syscall.WaitStatus)
		if status.Signaled() && (status.Signal() == syscall.SIGTERM || status.Signal() == syscall.SIGQUIT) {
			return true
		}
	}
	return false
}
