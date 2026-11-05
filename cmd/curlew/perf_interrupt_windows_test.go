//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type perfConsoleReady struct {
	Role      string
	Token     string
	PID       uint32
	ParentPID uint32
}

type perfConsoleResult struct {
	ConsolePIDs []uint32
	SignalSent  bool
	ExitCode    int
	Stdout      string
	Stderr      string
	Error       string
}

func TestPerfCmd_WindowsCtrlC(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	tools := t.TempDir()
	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, "go", "build", "-buildvcs=false", "-o", tools+string(os.PathSeparator),
		"../../testdata/perfconsolehelper", ".")
	build.WaitDelay = time.Second
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build native console fixtures: %v\n%s", err, output)
	}
	helper := filepath.Join(tools, "perfconsolehelper.exe")
	cli := filepath.Join(tools, "curlew.exe")

	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	requestFile := writePerfRequestFile(t, server.URL)

	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close() //nolint:errcheck
	deadline := time.Now().Add(45 * time.Second)
	if err := listener.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	token := hex.EncodeToString(nonce[:])
	t.Setenv("CURLEW_CONSOLE_ADDRESS", listener.Addr().String())
	t.Setenv("CURLEW_CONSOLE_TOKEN", token)

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		t.Fatal(err)
	}
	verified := false
	defer func() {
		if !verified {
			_ = windows.TerminateJobObject(job, 1)
		}
		if err := windows.CloseHandle(job); err != nil {
			t.Errorf("close test job: %v", err)
		}
	}()

	var stdout, stderr bytes.Buffer
	broker := exec.Command(helper, cli, requestFile)
	broker.Stdout = &stdout
	broker.Stderr = &stderr
	broker.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_CONSOLE | windows.CREATE_SUSPENDED,
		HideWindow:    true,
	}
	if err := broker.Start(); err != nil {
		t.Fatal(err)
	}
	brokerPID := uint32(broker.Process.Pid)
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE,
		false, brokerPID)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle) //nolint:errcheck
	if err := windows.AssignProcessToJobObject(job, handle); err != nil {
		t.Fatal(err)
	}
	resumePerfConsoleProcess(t, brokerPID)

	connection, err := listener.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close() //nolint:errcheck
	if err := connection.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(connection)
	readReady := func(role string) perfConsoleReady {
		var ready perfConsoleReady
		if err := decoder.Decode(&ready); err != nil {
			t.Fatalf("read %s readiness: %v", role, err)
		}
		if ready.Role != role || ready.Token != token {
			t.Fatalf("unexpected %s readiness: %+v", role, ready)
		}
		return ready
	}
	brokerReady := readReady("broker")
	cliReady := readReady("cli")
	if brokerReady.PID != brokerPID || brokerReady.ParentPID != uint32(os.Getpid()) || cliReady.ParentPID != brokerPID {
		t.Fatalf("unexpected process ancestry: broker=%+v cli=%+v", brokerReady, cliReady)
	}

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("perf request did not reach loopback server")
	}
	pids := []uint32{brokerPID, cliReady.PID}
	sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })
	unsafePIDs := append([]uint32(nil), pids...)
	unsafePIDs[0] = uint32(os.Getpid())
	sort.Slice(unsafePIDs, func(i, j int) bool { return unsafePIDs[i] < unsafePIDs[j] })
	if err := json.NewEncoder(connection).Encode(unsafePIDs); err != nil {
		t.Fatal(err)
	}
	var refusal perfConsoleResult
	if err := decoder.Decode(&refusal); err != nil {
		t.Fatal(err)
	}
	if refusal.Error == "" || refusal.SignalSent || !reflect.DeepEqual(refusal.ConsolePIDs, pids) {
		t.Fatalf("broker did not refuse foreign allowlist safely: %+v", refusal)
	}
	if err := json.NewEncoder(connection).Encode(pids); err != nil {
		t.Fatal(err)
	}
	if err := broker.Wait(); err != nil {
		t.Fatalf("console broker: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	var result perfConsoleResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode broker result: %v\nstdout: %s", err, stdout.String())
	}
	if result.Error != "" || !result.SignalSent || result.ExitCode != 130 || !reflect.DeepEqual(result.ConsolePIDs, pids) {
		t.Fatalf("Ctrl+C contract failed: %+v\nstderr: %s", result, stderr.String())
	}
	verified = true
}

func resumePerfConsoleProcess(t *testing.T, pid uint32) {
	t.Helper()
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(snapshot) //nolint:errcheck
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			t.Fatal(err)
		}
		previous, resumeErr := windows.ResumeThread(thread)
		closeErr := windows.CloseHandle(thread)
		if resumeErr != nil || closeErr != nil || previous != 1 {
			t.Fatal(fmt.Errorf("resume broker: previous=%d resume=%v close=%v", previous, resumeErr, closeErr))
		}
		return
	}
	t.Fatalf("no suspended broker thread for PID %d: %v", pid, err)
}
