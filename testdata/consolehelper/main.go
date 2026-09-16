//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type readiness struct {
	Role      string
	Token     string
	PID       uint32
	ParentPID uint32
}

type result struct {
	ConsolePIDs []uint32
	SignalSent  bool
	ExitCode    int
	Stdout      string
	Stderr      string
	Error       string
}

var (
	kernel         = windows.NewLazySystemDLL("kernel32.dll")
	consoleHandler = kernel.NewProc("SetConsoleCtrlHandler")
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "child" {
		if err := runChild(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) != 3 || os.Args[1] != "broker" || !filepath.IsAbs(os.Args[2]) {
		fmt.Fprintln(os.Stderr, "expected broker <absolute-cli-path> or child")
		os.Exit(1)
	}
	report, err := runBroker(os.Args[2])
	if err != nil {
		report.Error = err.Error()
	}
	if encodeErr := json.NewEncoder(os.Stdout).Encode(report); encodeErr != nil {
		fmt.Fprintln(os.Stderr, encodeErr)
		os.Exit(1)
	}
	if err != nil {
		os.Exit(1)
	}
}

func connect() (net.Conn, error) {
	address := os.Getenv("CURLEW_CONSOLE_ADDRESS")
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" || len(os.Getenv("CURLEW_CONSOLE_TOKEN")) != 32 {
		return nil, fmt.Errorf("expected authenticated loopback test connection")
	}
	connection, err := net.DialTimeout("tcp4", address, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to console test: %w", err)
	}
	if err := connection.SetDeadline(time.Now().Add(45 * time.Second)); err != nil {
		_ = connection.Close()
		return nil, err
	}
	return connection, nil
}

func announce(connection net.Conn, role string, pid, parentPID uint32) error {
	return json.NewEncoder(connection).Encode(readiness{
		Role: role, Token: os.Getenv("CURLEW_CONSOLE_TOKEN"), PID: pid, ParentPID: parentPID,
	})
}

func runChild() error {
	if success, _, err := consoleHandler.Call(0, 1); success == 0 {
		return fmt.Errorf("make native child Ctrl+C-immune: %w", err)
	}
	connection, err := connect()
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	if err := announce(connection, "child", uint32(os.Getpid()), uint32(os.Getppid())); err != nil {
		return err
	}
	time.Sleep(60 * time.Second)
	return fmt.Errorf("native child survived its expected job termination")
}

func consolePIDs() ([]uint32, error) {
	var pids [32]uint32
	count, _, err := kernel.NewProc("GetConsoleProcessList").Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if count == 0 || count > uintptr(len(pids)) {
		return nil, fmt.Errorf("cannot establish bounded console membership: count=%d error=%v", count, err)
	}
	members := pids[:count]
	sort.Slice(members, func(first, second int) bool { return members[first] < members[second] })
	return members, nil
}

func runBroker(cli string) (report result, returnedErr error) {
	initial, err := consolePIDs()
	if err != nil {
		return report, err
	}
	if !reflect.DeepEqual(initial, []uint32{uint32(os.Getpid())}) {
		return report, fmt.Errorf("refusing shared console: initial members=%v, broker=%d", initial, os.Getpid())
	}
	if success, _, err := consoleHandler.Call(0, 0); success == 0 {
		return report, fmt.Errorf("clear inherited Ctrl+C ignore flag before CLI launch: %w", err)
	}
	callback := syscall.NewCallback(func(event uint32) uintptr {
		if event == windows.CTRL_C_EVENT {
			return 1
		}
		return 0
	})
	if success, _, err := consoleHandler.Call(callback, 1); success == 0 {
		return report, fmt.Errorf("install non-inherited broker control handler: %w", err)
	}
	connection, err := connect()
	if err != nil {
		return report, err
	}
	defer func() { _ = connection.Close() }()
	if err := announce(connection, "broker", uint32(os.Getpid()), uint32(os.Getppid())); err != nil {
		return report, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, cli, "run", "interrupt.yaml")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	command.WaitDelay = time.Second
	if err := command.Start(); err != nil {
		return report, fmt.Errorf("start real CLI: %w", err)
	}
	waited := false
	defer func() {
		if !waited {
			cancel()
			_ = command.Wait()
		}
		report.Stdout, report.Stderr = stdout.String(), stderr.String()
	}()
	if err := announce(connection, "cli", uint32(command.Process.Pid), uint32(os.Getpid())); err != nil {
		return report, err
	}
	decoder := json.NewDecoder(connection)
	for attempt := 0; attempt < 2; attempt++ {
		var allowed []uint32
		if err := decoder.Decode(&allowed); err != nil {
			return report, fmt.Errorf("wait for parent to retain child handles: %w", err)
		}
		members, err := consolePIDs()
		if err != nil {
			return report, err
		}
		if len(allowed) != 4 || !reflect.DeepEqual(members, allowed) {
			err := fmt.Errorf("refusing CTRL_C_EVENT: console membership %v does not match allowlist %v", members, allowed)
			if encodeErr := json.NewEncoder(connection).Encode(result{ConsolePIDs: members, Error: err.Error()}); encodeErr != nil {
				return report, encodeErr
			}
			continue
		}
		report.ConsolePIDs = members
		if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_C_EVENT, 0); err != nil {
			return report, fmt.Errorf("generate isolated console Ctrl+C: %w", err)
		}
		report.SignalSent = true
		waitErr := command.Wait()
		waited = true
		if ctx.Err() != nil {
			return report, fmt.Errorf("CLI exceeded console test deadline: %w", ctx.Err())
		}
		var exitErr *exec.ExitError
		if waitErr != nil && !errors.As(waitErr, &exitErr) {
			return report, fmt.Errorf("wait for CLI: %w", waitErr)
		}
		report.ExitCode = command.ProcessState.ExitCode()
		return report, nil
	}
	return report, fmt.Errorf("refused both console allowlists; no signal sent")
}
