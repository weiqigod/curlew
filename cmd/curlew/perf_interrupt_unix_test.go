//go:build !windows

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"
)

func TestPerfCmd_ContextCancelExitCode130(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	file := writePerfRequestFile(t, srv.URL)
	done := make(chan int, 2)
	go func() {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Errorf("test server never received a request; SIGINT not sent")
			done <- -1
			return
		}
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()

	var stdout, stderr bytes.Buffer
	code := perfCmdOut([]string{file, "--vus", "2", "--duration", "30s"}, &stdout, &stderr)
	done <- code

	if got := <-done; got != 130 {
		t.Errorf("perfCmd exit code = %d, want 130 (SIGINT)", got)
	}
}