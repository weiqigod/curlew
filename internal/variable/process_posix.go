//go:build !windows

package variable

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

func runContainedCommand(cmd *exec.Cmd) error {
	attributes := syscall.SysProcAttr{}
	if cmd.SysProcAttr != nil {
		attributes = *cmd.SysProcAttr
	}
	attributes.Setpgid = true
	attributes.Pgid = 0
	cmd.SysProcAttr = &attributes
	killGroup := func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		if err != nil {
			return fmt.Errorf("kill command process group: %w", err)
		}
		return nil
	}
	cmd.Cancel = killGroup
	var captures []processCapture
	defer func() {
		for _, capture := range captures {
			_ = capture.reader.Close()
			_ = capture.writer.Close()
		}
	}()
	for _, stream := range []*io.Writer{&cmd.Stdout, &cmd.Stderr} {
		if *stream == nil {
			continue
		}
		if _, directFile := (*stream).(*os.File); directFile {
			continue
		}
		reader, writer, err := os.Pipe()
		if err != nil {
			return fmt.Errorf("create command output pipe: %w", err)
		}
		captures = append(captures, processCapture{reader, writer, *stream})
		*stream = writer
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	drained := make(chan error, len(captures))
	var outputMu sync.Mutex
	for _, capture := range captures {
		_ = capture.writer.Close()
		go func() {
			_, err := io.Copy(processOutputWriter{capture.output, &outputMu}, capture.reader)
			drained <- err
		}()
	}
	result := cmd.Wait()
	if err := killGroup(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		result = errors.Join(result, err)
	}
	delay := cmd.WaitDelay
	if delay <= 0 {
		delay = time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for remaining := len(captures); remaining > 0; remaining-- {
		select {
		case err := <-drained:
			if err != nil {
				result = errors.Join(result, fmt.Errorf("read command output: %w", err))
			}
		case <-timer.C:
			for _, capture := range captures {
				_ = capture.reader.Close()
			}
			for ; remaining > 0; remaining-- {
				<-drained
			}
			return errors.Join(result, exec.ErrWaitDelay)
		}
	}
	return result
}

type processCapture struct {
	reader *os.File
	writer *os.File
	output io.Writer
}

type processOutputWriter struct {
	output io.Writer
	mu     *sync.Mutex
}

func (writer processOutputWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.output.Write(data)
}
