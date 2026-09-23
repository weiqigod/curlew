package vault_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var consoleFixturesFlag = flag.String("console-fixtures", "", "Directory containing native curlew.exe and consolehelper.exe built from this checkout; empty builds them offline within the test deadline")

type consoleReady struct {
	Role      string
	Token     string
	PID       uint32
	ParentPID uint32
}

type consoleResult struct {
	ConsolePIDs []uint32
	SignalSent  bool
	ExitCode    int
	Stdout      string
	Stderr      string
	Error       string
}

func TestCommandCLIInterruptCleanup(t *testing.T) {
	started := time.Now()
	tools := *consoleFixturesFlag
	if tools == "" {
		tools = t.TempDir()
		buildCtx, cancelBuild := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancelBuild()
		build := exec.CommandContext(buildCtx, providerGo(t), "build", "-buildvcs=false", "-ldflags=-s -w", "-o", tools+string(os.PathSeparator), "../../testdata/consolehelper", "../../cmd/curlew")
		build.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off")
		build.WaitDelay = time.Second
		if output, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build native console fixtures within test budget (no signal sent): %v\n%s\nOn slow hosts, build fixtures before testing and pass -args -console-fixtures=<absolute-directory>", err, output)
		}
		t.Logf("offline fixture build -buildvcs=false -ldflags='-s -w' took %s", time.Since(started).Round(time.Millisecond))
	} else if !filepath.IsAbs(tools) {
		t.Fatal("-console-fixtures must be an absolute directory")
	}
	for name, source := range map[string]string{
		"consolehelper.exe": "github.com/weiqigod/curlew/testdata/consolehelper",
		"curlew.exe":        "github.com/weiqigod/curlew/cmd/curlew",
	} {
		info, err := buildinfo.ReadFile(filepath.Join(tools, name))
		if err != nil {
			t.Fatalf("read %s native build identity: %v", name, err)
		}
		var nativeOS, nativeArch bool
		for _, setting := range info.Settings {
			nativeOS = nativeOS || (setting.Key == "GOOS" && setting.Value == "windows")
			nativeArch = nativeArch || (setting.Key == "GOARCH" && setting.Value == runtime.GOARCH)
		}
		if info.Path != source || !nativeOS || !nativeArch {
			t.Fatalf("%s must be a native windows/%s build of %s; got %+v", name, runtime.GOARCH, source, info)
		}
	}
	helper := filepath.Join(tools, "consolehelper.exe")
	cli := filepath.Join(tools, "curlew.exe")
	deadline := time.Now().Add(45 * time.Second)
	if testDeadline, ok := t.Deadline(); ok && testDeadline.Add(-3*time.Second).Before(deadline) {
		deadline = testDeadline.Add(-3 * time.Second)
	}
	t.Logf("native %s/%s %s; compiler=%s; verified helper=%s; verified CLI=%s", runtime.GOOS, runtime.GOARCH, runtime.Version(), providerGo(t), helper, cli)
	project := t.TempDir()
	t.Setenv("CURLEW_CONFIG_DIR", filepath.Join(project, "user-config"))
	for _, name := range []string{"CURLEW_TEAM_CONFIG", "CURLEW_PLUGINS", "CURLEW_VAULT_STUB"} {
		t.Setenv(name, "")
	}
	t.Setenv("PATH", filepath.Join(os.Getenv("SystemRoot"), "System32"))
	t.Setenv("CURLEW_CONSOLE_HELPER", helper)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		response.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	writeProviderYAML(t, filepath.Join(project, "curlew.yaml"), map[string]any{
		"project_name": "console-interrupt-cleanup",
	})
	writeProviderYAML(t, filepath.Join(project, "interrupt.yaml"), map[string]any{
		"name": "console interrupt cleanup",
		"variables": map[string]any{
			"command_value": map[string]any{"from_command": "Start-Process -FilePath $env:CURLEW_CONSOLE_HELPER -ArgumentList 'child' -NoNewWindow -PassThru | Out-Null; Start-Sleep -Seconds 60"},
		},
		"requests": []any{map[string]any{
			"name": "must not run", "request": map[string]any{"method": "GET", "url": server.URL},
		}},
	})
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
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
	var command *exec.Cmd
	var finished chan error
	var stdout, stderr bytes.Buffer
	handles := make(map[uint32]windows.Handle)
	verified := false
	t.Cleanup(func() {
		if !verified {
			t.Log("fallback cleanup: closing the test-owned kill-on-close job; no console event")
		}
		if err := windows.CloseHandle(job); err != nil {
			t.Errorf("close console test job: %v", err)
		}
		if finished != nil {
			select {
			case waitErr := <-finished:
				if !verified {
					t.Logf("broker cleanup: wait=%v\nstdout: %s\nstderr: %s", waitErr, stdout.String(), stderr.String())
				}
			case <-time.After(2 * time.Second):
				t.Error("console broker did not exit during job cleanup")
			}
		}
		for pid, handle := range handles {
			status, err := windows.WaitForSingleObject(handle, 500)
			if err != nil || status != windows.WAIT_OBJECT_0 {
				t.Errorf("cleanup PID %d: wait=%d error=%v", pid, status, err)
			}
			if err := windows.CloseHandle(handle); err != nil {
				t.Errorf("close PID %d handle: %v", pid, err)
			}
		}
		if verified {
			t.Log("cleanup: broker, CLI, PowerShell and native child exited before test job closure; all process handles closed")
		}
	})
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		t.Fatal(err)
	}
	command = exec.Command(helper, "broker", cli)
	command.Dir = project
	command.Stdout, command.Stderr = &stdout, &stderr
	command.WaitDelay = time.Second
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_CONSOLE | windows.CREATE_SUSPENDED, HideWindow: true,
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	finished = make(chan error, 1)
	go func() {
		finished <- command.Wait()
		close(finished)
	}()
	brokerPID := uint32(command.Process.Pid)
	brokerHandle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, brokerPID)
	if err != nil {
		_ = command.Process.Kill()
		t.Fatal(err)
	}
	handles[brokerPID] = brokerHandle
	if err := windows.AssignProcessToJobObject(job, brokerHandle); err != nil {
		_ = command.Process.Kill()
		t.Fatal(err)
	}
	resumeConsoleBroker(t, brokerPID)
	accept := func() net.Conn {
		connection, err := listener.AcceptTCP()
		if err != nil {
			t.Fatalf("console readiness (no signal sent): %v", err)
		}
		t.Cleanup(func() { _ = connection.Close() })
		if err := connection.SetDeadline(deadline); err != nil {
			t.Fatal(err)
		}
		return connection
	}
	readReady := func(decoder *json.Decoder, role string) consoleReady {
		var ready consoleReady
		if err := decoder.Decode(&ready); err != nil {
			t.Fatalf("%s readiness (no signal sent): %v", role, err)
		}
		if ready.Token != token || ready.Role != role || ready.PID == 0 {
			t.Fatalf("unexpected %s readiness: role=%q PID=%d", role, ready.Role, ready.PID)
		}
		return ready
	}
	broker := accept()
	decoder := json.NewDecoder(broker)
	ready := readReady(decoder, "broker")
	if ready.PID != brokerPID || ready.ParentPID != uint32(os.Getpid()) {
		t.Fatalf("wrong broker identity: %+v", ready)
	}
	cliReady := readReady(decoder, "cli")
	childReady := readReady(json.NewDecoder(accept()), "child")
	parents := map[uint32]uint32{cliReady.PID: brokerPID, childReady.ParentPID: cliReady.PID, childReady.PID: childReady.ParentPID}
	if len(parents) != 3 || cliReady.ParentPID != brokerPID {
		t.Fatal("invalid CLI/PowerShell/native-child ancestry")
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := windows.CloseHandle(snapshot); err != nil {
			t.Errorf("close process snapshot: %v", err)
		}
	}()
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		parent, expected := parents[entry.ProcessID]
		if !expected {
			continue
		}
		if entry.ParentProcessID != parent || (entry.ProcessID == childReady.ParentPID && windows.UTF16ToString(entry.ExeFile[:]) != "powershell.exe") {
			t.Fatalf("unexpected process ancestry: PID=%d parent=%d image=%s", entry.ProcessID, entry.ParentProcessID, windows.UTF16ToString(entry.ExeFile[:]))
		}
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_INFORMATION, false, entry.ProcessID)
		if err != nil {
			t.Fatal(err)
		}
		handles[entry.ProcessID] = handle
		var inJob int32
		result, _, callErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(handle), uintptr(job), uintptr(unsafe.Pointer(&inJob)))
		if result == 0 || inJob == 0 {
			t.Fatalf("PID %d not in test-owned job: %v", entry.ProcessID, callErr)
		}
	}
	if len(handles) != 4 {
		t.Fatalf("retained %d process handles, want broker + CLI + PowerShell + native child", len(handles))
	}
	var pids []uint32
	for pid, handle := range handles {
		if status, err := windows.WaitForSingleObject(handle, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
			t.Fatalf("PID %d must be alive before Ctrl+C: wait=%d error=%v", pid, status, err)
		}
		pids = append(pids, pid)
	}
	sort.Slice(pids, func(first, second int) bool { return pids[first] < pids[second] })
	t.Logf("live broker=%d CLI=%d PowerShell=%d Ctrl+C-immune native child=%d; handles retained", brokerPID, cliReady.PID, childReady.ParentPID, childReady.PID)
	unsafePIDs := append([]uint32(nil), pids...)
	unsafePIDs[0] = uint32(os.Getpid())
	sort.Slice(unsafePIDs, func(first, second int) bool { return unsafePIDs[first] < unsafePIDs[second] })
	if err := json.NewEncoder(broker).Encode(unsafePIDs); err != nil {
		t.Fatal(err)
	}
	var refusal consoleResult
	if err := decoder.Decode(&refusal); err != nil {
		t.Fatalf("console membership refusal: %v", err)
	}
	if refusal.Error == "" || refusal.SignalSent || !reflect.DeepEqual(refusal.ConsolePIDs, pids) {
		t.Fatalf("broker did not safely refuse a foreign-console PID: %+v", refusal)
	}
	t.Logf("safety guard exercised without signalling: %s", refusal.Error)
	for pid, handle := range handles {
		if status, err := windows.WaitForSingleObject(handle, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
			t.Fatalf("PID %d exited during refusal: wait=%d error=%v", pid, status, err)
		}
	}
	if err := json.NewEncoder(broker).Encode(pids); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("isolated console broker: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
		}
	case <-time.After(time.Until(deadline)):
		t.Fatal("CLI did not exit within the bounded console test")
	}
	var result consoleResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("broker result: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if result.Error != "" || !result.SignalSent || !reflect.DeepEqual(result.ConsolePIDs, pids) || result.ExitCode != 5 || !strings.Contains(result.Stderr, "context canceled") {
		t.Fatalf("actual CTRL_C_EVENT/exit-5 contract failed: %+v", result)
	}
	for pid, handle := range handles {
		if status, err := windows.WaitForSingleObject(handle, 500); err != nil || status != windows.WAIT_OBJECT_0 {
			t.Errorf("PID %d survived actual console Ctrl+C: wait=%d error=%v", pid, status, err)
		}
	}
	if count := requests.Load(); count != 0 {
		t.Errorf("sent %d HTTP requests before command resolution completed", count)
	}
	verified = !t.Failed()
	t.Logf("GenerateConsoleCtrlEvent(CTRL_C_EVENT, 0), isolated console PIDs=%v, CLI exit=%d\nstdout: %s\nstderr: %s", result.ConsolePIDs, result.ExitCode, result.Stdout, result.Stderr)
}

func resumeConsoleBroker(t *testing.T, pid uint32) {
	t.Helper()
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := windows.CloseHandle(snapshot); err != nil {
			t.Errorf("close thread snapshot: %v", err)
		}
	}()
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
