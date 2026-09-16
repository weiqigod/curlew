//go:build !windows

package variable

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"reflect"
	"syscall"
	"testing"
	"time"
)

func containedHelperCommand(t *testing.T, ctx context.Context, args ...string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, executable, append([]string{"-test.run=^TestContainedProcessHelper$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "CURLEW_CONTAINED_PROCESS_HELPER=1")
	cmd.WaitDelay = time.Second
	return cmd
}

func TestRunContainedCommand_tree(t *testing.T) {
	for _, finish := range []string{"parent_exit", "cancel", "deadline"} {
		t.Run(finish, func(t *testing.T) {
			listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			if err := listener.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			cmd := containedHelperCommand(t, ctx, "parent", listener.Addr().String())
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			finished := make(chan struct{})
			var runErr error
			go func() {
				runErr = runContainedCommand(cmd)
				close(finished)
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case <-finished:
				case <-time.After(3 * time.Second):
					t.Error("contained command did not finish during cleanup")
				}
			})
			connections := make(map[string]net.Conn)
			groups := make(map[string]int)
			parentPID := 0
			for len(connections) < 3 {
				connection, err := listener.AcceptTCP()
				if err != nil {
					t.Fatalf("tree readiness: %v", err)
				}
				t.Cleanup(func() { _ = connection.Close() })
				if err := connection.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
					t.Fatal(err)
				}
				var ready struct {
					PID  int
					Role string
				}
				if err := json.NewDecoder(connection).Decode(&ready); err != nil {
					t.Fatal(err)
				}
				process, err := os.FindProcess(ready.PID)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					_ = process.Kill()
					_ = process.Release()
				})
				if _, duplicate := connections[ready.Role]; duplicate {
					t.Fatalf("duplicate helper role %q", ready.Role)
				}
				connections[ready.Role] = connection
				group, err := syscall.Getpgid(ready.PID)
				if err != nil {
					t.Fatal(err)
				}
				groups[ready.Role] = group
				if ready.Role == "parent" {
					parentPID = ready.PID
				}
			}
			for _, role := range []string{"parent", "child", "grandchild"} {
				if _, ok := connections[role]; !ok {
					t.Fatalf("missing %s readiness", role)
				}
				if groups[role] != parentPID {
					t.Errorf("%s process group = %d, want %d", role, groups[role], parentPID)
				}
			}
			switch finish {
			case "parent_exit":
				if _, err := connections["parent"].Write([]byte{1}); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				cancel()
			case "deadline":
				<-ctx.Done()
			}
			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				t.Fatal("contained command exceeded bounded completion")
			}
			if finish == "parent_exit" {
				if runErr != nil {
					t.Errorf("normal parent exit: %v; stderr=%q", runErr, stderr.String())
				}
				if stdout.String() != "parent finished\n" {
					t.Errorf("stdout = %q", stdout.String())
				}
			} else if runErr == nil {
				t.Error("cancelled command returned nil")
			}
			if finish == "deadline" && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Errorf("context error = %v", ctx.Err())
			}
			for role, connection := range connections {
				if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				var payload [1]byte
				count, err := connection.Read(payload[:])
				if count != 0 || !errors.Is(err, io.EOF) {
					t.Errorf("%s helper connection still open: count=%d err=%v", role, count, err)
				}
			}
		})
	}
}

func TestRunContainedCommand_argv_and_pipes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := []string{"", "two words", `quote"inside`, `trailing\`, "& | ; $PATH"}
	cmd := containedHelperCommand(t, ctx, append([]string{"echo"}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := runContainedCommand(cmd); err != nil {
		t.Fatal(err)
	}
	var actual []string
	if err := json.Unmarshal(stdout.Bytes(), &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, args) || stderr.String() != "helper stderr\n" {
		t.Fatalf("args=%q stderr=%q", actual, stderr.String())
	}
}

func TestRunContainedCommand_nonzero_exit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := containedHelperCommand(t, ctx, "exit")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := runContainedCommand(cmd)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 42 {
		t.Fatalf("error = %v, want exit code 42", err)
	}
	if stdout.String() != "before exit\n" || stderr.String() != "exit stderr\n" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunContainedCommand_cancelled_before_start(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := containedHelperCommand(t, ctx, "echo", "must not run")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := runContainedCommand(cmd); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if cmd.Process != nil || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatal("cancelled command started")
	}
}

func TestContainedProcessHelper(t *testing.T) {
	if os.Getenv("CURLEW_CONTAINED_PROCESS_HELPER") != "1" {
		return
	}
	args := os.Args[3:]
	switch args[0] {
	case "echo":
		_ = json.NewEncoder(os.Stdout).Encode(args[1:])
		_, _ = fmt.Fprintln(os.Stderr, "helper stderr")
		os.Exit(0)
	case "exit":
		_, _ = fmt.Fprintln(os.Stdout, "before exit")
		_, _ = fmt.Fprintln(os.Stderr, "exit stderr")
		os.Exit(42)
	}
	connection, err := net.DialTimeout("tcp4", args[1], 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if args[0] != "grandchild" {
		next := "child"
		if args[0] == "child" {
			next = "grandchild"
		}
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		child := exec.Command(executable, "-test.run=^TestContainedProcessHelper$", "--", next, args[1])
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		go func() { _ = child.Wait() }()
	}
	ready := struct {
		PID  int
		Role string
	}{os.Getpid(), args[0]}
	if err := json.NewEncoder(connection).Encode(ready); err != nil {
		t.Fatal(err)
	}
	var release [1]byte
	if _, err := connection.Read(release[:]); err != nil {
		os.Exit(98)
	}
	_, _ = fmt.Fprintln(os.Stdout, "parent finished")
	os.Exit(0)
}
