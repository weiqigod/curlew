package backend_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/weiqigod/curlew/internal/backend"
	"github.com/zalando/go-keyring"
)

func TestStorage_KeychainAvailable_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin keychain check only")
	}
	keyring.MockInit()
	t.Cleanup(func() { keyring.MockInit() })

	cfgDir := t.TempDir()
	s, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, DeviceID: "dev-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !s.KeychainAvailable() {
		t.Fatal("expected keychain available on Darwin with MockInit")
	}
	if err := s.SetRefreshToken("rt-x"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccessToken("at-x"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRefreshToken()
	if err != nil || got != "rt-x" {
		t.Fatalf("got %q, %v; want rt-x, nil", got, err)
	}
	gotAccess, err := s.GetAccessToken()
	if err != nil || gotAccess != "at-x" {
		t.Fatalf("access token = %q, %v; want at-x, nil", gotAccess, err)
	}
	// Assert the encrypted file was NOT created — keychain was used.
	encPath := filepath.Join(cfgDir, "refresh_token.enc")
	if _, statErr := os.Stat(encPath); !os.IsNotExist(statErr) {
		t.Fatalf("encrypted file should not exist when keychain is in use; stat err = %v", statErr)
	}
	// Exercise DeleteRefreshToken on the keychain path (covers keychainStorage.DeleteRefreshToken).
	if err := s.DeleteRefreshToken(); err != nil {
		t.Fatalf("first keychain delete: %v", err)
	}
	if err := s.DeleteAccessToken(); err != nil {
		t.Fatalf("first access-token keychain delete: %v", err)
	}
	// Idempotent second delete — token already gone, should be a no-op.
	if err := s.DeleteRefreshToken(); err != nil {
		t.Fatalf("second keychain delete should be no-op: %v", err)
	}
	if err := s.DeleteAccessToken(); err != nil {
		t.Fatalf("second access-token keychain delete should be no-op: %v", err)
	}
}

func TestStorage_NoKeychainFallsBackToFile(t *testing.T) {
	keyring.MockInitWithError(errors.New("no D-Bus"))
	t.Cleanup(func() { keyring.MockInit() })

	cfgDir := t.TempDir()
	s, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, DeviceID: "dev-1"})
	if err != nil {
		t.Fatal(err)
	}
	if s.KeychainAvailable() {
		t.Fatal("expected fallback to file storage when keychain errors")
	}
	if err := s.SetRefreshToken("rt-y"); err != nil {
		t.Fatal(err)
	}
	encPath := filepath.Join(cfgDir, "refresh_token.enc")
	if _, statErr := os.Stat(encPath); statErr != nil {
		t.Fatalf("encrypted file should exist in fallback mode: %v", statErr)
	}
	got, err := s.GetRefreshToken()
	if err != nil || got != "rt-y" {
		t.Fatalf("got %q, %v; want rt-y, nil", got, err)
	}
	if err := s.SetAccessToken("at-y"); err != nil {
		t.Fatal(err)
	}
	accessPath := filepath.Join(cfgDir, "access_token.enc")
	if _, statErr := os.Stat(accessPath); statErr != nil {
		t.Fatalf("encrypted access-token file should exist in fallback mode: %v", statErr)
	}
	accessToken, err := s.GetAccessToken()
	if err != nil || accessToken != "at-y" {
		t.Fatalf("access token = %q, %v; want at-y, nil", accessToken, err)
	}
}

func TestStorage_ForceFileEnvironmentOverridesKeychain(t *testing.T) {
	keyring.MockInit()
	t.Cleanup(func() { keyring.MockInit() })
	t.Setenv("CURLEW_FORCE_FILE_STORAGE", "1")

	cfgDir := t.TempDir()
	s, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, DeviceID: "dev-ci"})
	if err != nil {
		t.Fatal(err)
	}
	if s.KeychainAvailable() {
		t.Fatal("expected CURLEW_FORCE_FILE_STORAGE=1 to select encrypted file storage")
	}
	if err := s.SetRefreshToken("rt-ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfgDir, "refresh_token.enc")); err != nil {
		t.Fatalf("encrypted refresh-token file was not created: %v", err)
	}
}

func TestStorage_GetMissingReturnsErrTokenNotFound(t *testing.T) {
	keyring.MockInitWithError(errors.New("no D-Bus"))
	t.Cleanup(func() { keyring.MockInit() })

	cfgDir := t.TempDir()
	s, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, ForceFile: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.GetRefreshToken()
	if !errors.Is(err, backend.ErrTokenNotFound) {
		t.Fatalf("got %v; want ErrTokenNotFound", err)
	}
	_, err = s.GetAccessToken()
	if !errors.Is(err, backend.ErrTokenNotFound) {
		t.Fatalf("access token: got %v; want ErrTokenNotFound", err)
	}
}

func TestStorage_DeleteIdempotent(t *testing.T) {
	keyring.MockInitWithError(errors.New("no D-Bus"))
	t.Cleanup(func() { keyring.MockInit() })

	cfgDir := t.TempDir()
	s, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, ForceFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRefreshToken("rt-z"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccessToken("at-z"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRefreshToken(); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := s.DeleteAccessToken(); err != nil {
		t.Fatalf("first access-token delete: %v", err)
	}
	if err := s.DeleteAccessToken(); err != nil {
		t.Fatalf("second access-token delete should be no-op: %v", err)
	}
	if err := s.DeleteRefreshToken(); err != nil {
		t.Fatalf("second delete should be no-op: %v", err)
	}
}

func TestStorage_ConfigDirRequired(t *testing.T) {
	_, err := backend.NewStorage(backend.StorageOptions{})
	if err == nil {
		t.Fatal("expected error when ConfigDir is empty")
	}
}

func TestStorage_SetEmptyTokenRejected(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) (backend.Storage, func())
	}{
		{
			name: "file-backend rejects empty token",
			setup: func(t *testing.T) (backend.Storage, func()) {
				t.Helper()
				keyring.MockInitWithError(errors.New("no D-Bus"))
				cfgDir := t.TempDir()
				s, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, ForceFile: true})
				if err != nil {
					t.Fatalf("NewStorage: %v", err)
				}
				return s, func() { keyring.MockInit() }
			},
		},
		{
			name: "keychain-backend rejects empty token",
			setup: func(t *testing.T) (backend.Storage, func()) {
				t.Helper()
				if runtime.GOOS != "darwin" {
					t.Skip("keychain mock only reliable on Darwin")
				}
				keyring.MockInit()
				cfgDir := t.TempDir()
				s, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, DeviceID: "dev-1"})
				if err != nil {
					t.Fatalf("NewStorage: %v", err)
				}
				return s, func() { keyring.MockInit() }
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, cleanup := tc.setup(t)
			defer cleanup()
			if err := s.SetRefreshToken(""); err == nil {
				t.Fatal("expected error when setting empty refresh token; got nil")
			}
			if err := s.SetAccessToken(""); err == nil {
				t.Fatal("expected error when setting empty access token; got nil")
			}
		})
	}
}
