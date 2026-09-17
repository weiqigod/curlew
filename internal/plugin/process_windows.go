//go:build windows

package plugin

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func spawnPlugin(path string, stderr io.Writer) (io.WriteCloser, io.ReadCloser, func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create plugin job: %w", err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = windows.CloseHandle(job)
		return nil, nil, nil, fmt.Errorf("configure plugin job: %w", err)
	}

	cmd := exec.Command(path) //nolint:gosec // plugin path is explicitly configured
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		_ = windows.CloseHandle(job)
		return nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = windows.CloseHandle(job)
		return nil, nil, nil, fmt.Errorf("start: %w", err)
	}

	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false, uint32(cmd.Process.Pid))
	if err != nil {
		return nil, nil, nil, failPluginSetup(cmd, stdin, stdout, job, fmt.Errorf("open plugin process: %w", err))
	}
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		_ = windows.CloseHandle(process)
		return nil, nil, nil, failPluginSetup(cmd, stdin, stdout, job, fmt.Errorf("assign plugin process to job: %w", err))
	}
	if err := resumePluginProcess(uint32(cmd.Process.Pid)); err != nil {
		_ = windows.CloseHandle(process)
		return nil, nil, nil, failPluginSetup(cmd, stdin, stdout, job, err)
	}
	_ = windows.CloseHandle(process)

	waited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waited)
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = stdin.Close()
			select {
			case <-waited:
			case <-time.After(shutdownTimeout):
				_ = windows.TerminateJobObject(job, 1)
				<-waited
			}
			_ = windows.CloseHandle(job)
		})
	}
	return stdin, stdout, stop, nil
}

func failPluginSetup(cmd *exec.Cmd, stdin io.Closer, stdout io.Closer, job windows.Handle, setupErr error) error {
	_ = stdin.Close()
	_ = stdout.Close()
	killErr := cmd.Process.Kill()
	if errors.Is(killErr, os.ErrProcessDone) {
		killErr = nil
	}
	waitErr := cmd.Wait()
	jobErr := windows.CloseHandle(job)
	return errors.Join(setupErr, killErr, waitErr, jobErr)
}

func resumePluginProcess(pid uint32) (result error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("snapshot plugin threads: %w", err)
	}
	defer func() {
		if err := windows.CloseHandle(snapshot); err != nil {
			result = errors.Join(result, fmt.Errorf("close plugin thread snapshot: %w", err))
		}
	}()

	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return fmt.Errorf("open suspended plugin thread: %w", err)
		}
		previous, resumeErr := windows.ResumeThread(thread)
		closeErr := windows.CloseHandle(thread)
		if resumeErr != nil {
			return errors.Join(fmt.Errorf("resume plugin thread: %w", resumeErr), closeErr)
		}
		if previous != 1 {
			return errors.Join(fmt.Errorf("resume plugin thread: unexpected suspend count %d", previous), closeErr)
		}
		return closeErr
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("enumerate plugin threads: %w", err)
	}
	return fmt.Errorf("suspended plugin thread not found for pid %d", pid)
}