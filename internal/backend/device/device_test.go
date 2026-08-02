package device_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peterlindqvist/apitest/internal/backend/device"
)

func TestDeviceRoundTrip(t *testing.T) {
	t.Run("happy round-trip", func(t *testing.T) {
		dir := t.TempDir()
		want := device.Record{
			DeviceID: "dev_abc",
			IssuedAt: time.Now().UTC().Truncate(time.Second),
		}
		if err := device.Write(dir, want); err != nil {
			t.Fatalf("Write: %v", err)
		}
		got, err := device.Read(dir)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if got.DeviceID != want.DeviceID {
			t.Errorf("DeviceID = %q, want %q", got.DeviceID, want.DeviceID)
		}
		if !got.IssuedAt.Equal(want.IssuedAt) {
			t.Errorf("IssuedAt = %v, want %v", got.IssuedAt, want.IssuedAt)
		}
	})

	t.Run("absent file returns ErrNotFound", func(t *testing.T) {
		dir := t.TempDir()
		_, err := device.Read(dir)
		if !errors.Is(err, device.ErrNotFound) {
			t.Errorf("Read on absent file: got %v, want ErrNotFound", err)
		}
	})

	t.Run("file mode is 0600", func(t *testing.T) {
		dir := t.TempDir()
		rec := device.Record{DeviceID: "dev_x", IssuedAt: time.Now().UTC()}
		if err := device.Write(dir, rec); err != nil {
			t.Fatalf("Write: %v", err)
		}
		info, err := os.Stat(filepath.Join(dir, "device.json"))
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("file mode = %04o, want 0600", perm)
		}
	})

	t.Run("write creates directory if absent", func(t *testing.T) {
		base := t.TempDir()
		dir := filepath.Join(base, "nested", "subdir")
		rec := device.Record{DeviceID: "dev_nested", IssuedAt: time.Now().UTC()}
		if err := device.Write(dir, rec); err != nil {
			t.Fatalf("Write: %v", err)
		}
		got, err := device.Read(dir)
		if err != nil {
			t.Fatalf("Read after mkdir: %v", err)
		}
		if got.DeviceID != rec.DeviceID {
			t.Errorf("DeviceID = %q, want %q", got.DeviceID, rec.DeviceID)
		}
	})

	t.Run("corrupt JSON yields parse error", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "device.json"), []byte("{not-json"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		_, err := device.Read(dir)
		if err == nil {
			t.Fatal("Read on corrupt JSON: want error, got nil")
		}
		if errors.Is(err, device.ErrNotFound) {
			t.Errorf("Read on corrupt JSON: got ErrNotFound, want a parse error")
		}
	})

	t.Run("write fails on read-only directory", func(t *testing.T) {
		if os.Getenv("CI") != "" || os.Getuid() == 0 {
			t.Skip("skipping: root or CI may bypass permission checks")
		}
		base := t.TempDir()
		// Make the target directory read-only so WriteFile fails.
		if err := os.Chmod(base, 0o500); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(base, 0o700) }) // restore so TempDir cleanup works
		rec := device.Record{DeviceID: "dev_fail", IssuedAt: time.Now().UTC()}
		err := device.Write(base, rec)
		if err == nil {
			t.Fatal("Write on read-only dir: want error, got nil")
		}
	})
}
