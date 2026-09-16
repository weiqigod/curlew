package variable

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func runContainedCommand(cmd *exec.Cmd) (result error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create command job: %w", err)
	}
	var jobMu sync.Mutex
	closeJobLocked := func() error {
		if job == 0 {
			return nil
		}
		if err := windows.CloseHandle(job); err != nil {
			return fmt.Errorf("close command job: %w", err)
		}
		job = 0
		return nil
	}
	defer func() {
		jobMu.Lock()
		defer jobMu.Unlock()
		if err := closeJobLocked(); err != nil {
			result = errors.Join(result, err)
		}
	}()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return fmt.Errorf("configure command job: %w", err)
	}
	attributes := syscall.SysProcAttr{}
	if cmd.SysProcAttr != nil {
		attributes = *cmd.SysProcAttr
	}
	attributes.CreationFlags |= windows.CREATE_SUSPENDED
	cmd.SysProcAttr = &attributes
	terminateJobLocked := func() error {
		if job == 0 {
			return nil
		}
		if err := windows.TerminateJobObject(job, 1); err != nil {
			return errors.Join(fmt.Errorf("terminate command job: %w", err), closeJobLocked())
		}
		return nil
	}
	var process windows.Handle
	cmd.Cancel = func() error {
		jobMu.Lock()
		defer jobMu.Unlock()
		if job == 0 {
			return os.ErrProcessDone
		}
		status, waitErr := windows.WaitForSingleObject(process, 0)
		if err := terminateJobLocked(); err != nil {
			return err
		}
		if waitErr != nil {
			return fmt.Errorf("check command process during cancellation: %w", waitErr)
		}
		if status == windows.WAIT_OBJECT_0 {
			return os.ErrProcessDone
		}
		return nil
	}
	jobMu.Lock()
	if err := cmd.Start(); err != nil {
		jobMu.Unlock()
		return err
	}
	failSetup := func(setupErr error) error {
		killErr := cmd.Process.Kill()
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
		jobErr := terminateJobLocked()
		jobMu.Unlock()
		return errors.Join(setupErr, killErr, jobErr, cmd.Wait())
	}
	process, err = windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE,
		false, uint32(cmd.Process.Pid))
	if err != nil {
		return failSetup(fmt.Errorf("open command process: %w", err))
	}
	defer func() {
		if err := windows.CloseHandle(process); err != nil {
			result = errors.Join(result, fmt.Errorf("close command process: %w", err))
		}
	}()
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return failSetup(fmt.Errorf("assign command process to job: %w", err))
	}
	if err := resumeContainedProcess(uint32(cmd.Process.Pid)); err != nil {
		return failSetup(err)
	}
	jobMu.Unlock()
	monitored := make(chan error, 1)
	go func() {
		status, waitErr := windows.WaitForSingleObject(process, windows.INFINITE)
		if waitErr != nil {
			waitErr = fmt.Errorf("monitor command process: %w", waitErr)
		} else if status != windows.WAIT_OBJECT_0 {
			waitErr = fmt.Errorf("monitor command process: unexpected wait status %d", status)
		}
		jobMu.Lock()
		jobErr := terminateJobLocked()
		jobMu.Unlock()
		monitored <- errors.Join(waitErr, jobErr)
	}()
	result = cmd.Wait()
	if err := <-monitored; err != nil {
		result = errors.Join(result, err)
	}
	return result
}

func resumeContainedProcess(pid uint32) (result error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("snapshot command threads: %w", err)
	}
	defer func() {
		if err := windows.CloseHandle(snapshot); err != nil {
			result = errors.Join(result, fmt.Errorf("close command thread snapshot: %w", err))
		}
	}()
	entry := windows.ThreadEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return fmt.Errorf("open suspended command thread: %w", err)
		}
		previous, resumeErr := windows.ResumeThread(thread)
		closeErr := windows.CloseHandle(thread)
		if resumeErr != nil {
			return errors.Join(fmt.Errorf("resume command thread: %w", resumeErr), closeErr)
		}
		if previous != 1 {
			return errors.Join(fmt.Errorf("resume command thread: unexpected suspend count %d", previous), closeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close command thread: %w", closeErr)
		}
		return nil
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("enumerate command threads: %w", err)
	}
	return fmt.Errorf("suspended command thread not found for pid %d", pid)
}
