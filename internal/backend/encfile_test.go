package backend

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptedFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	f := newEncryptedFile(filepath.Join(dir, "tok.enc"), []byte("test-salt-1"))
	plaintext := []byte("rt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err := f.Write(plaintext); err != nil {
		t.Fatal(err)
	}
	got, err := f.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestEncryptedFile_FileMode0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tok.enc")
	f := newEncryptedFile(path, []byte("salt"))
	if err := f.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v; want 0600", info.Mode().Perm())
	}
}

func TestEncryptedFile_TamperRejection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tok.enc")
	f := newEncryptedFile(path, []byte("salt-tamper"))
	if err := f.Write([]byte("sensitive-token")); err != nil {
		t.Fatal(err)
	}

	// Corrupt the last byte (in the ciphertext region)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xFF
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = f.Read()
	if !errors.Is(err, ErrEncryptedFileTampered) {
		t.Fatalf("got %v; want ErrEncryptedFileTampered", err)
	}
}

func TestEncryptedFile_KeyChangeRejection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tok.enc")
	fA := newEncryptedFile(path, []byte("salt-A"))
	if err := fA.Write([]byte("token-abc")); err != nil {
		t.Fatal(err)
	}
	fB := newEncryptedFile(path, []byte("salt-B"))
	_, err := fB.Read()
	if !errors.Is(err, ErrEncryptedFileTampered) {
		t.Fatalf("got %v; want ErrEncryptedFileTampered when key changes", err)
	}
}

func TestEncryptedFile_AbsentReturnsErrNotExist(t *testing.T) {
	f := newEncryptedFile("/nonexistent/path/tok.enc", []byte("salt"))
	_, err := f.Read()
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("got %v; want fs.ErrNotExist", err)
	}
}

func TestEncryptedFile_DeleteIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tok.enc")
	f := newEncryptedFile(path, []byte("salt"))
	if err := f.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := f.Delete(); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := f.Delete(); err != nil {
		t.Fatalf("second delete should be no-op: %v", err)
	}
}
