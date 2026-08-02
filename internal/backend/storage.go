package backend

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

// keyring service + user constants. Service is the apitool-cli identity;
// user is the secret slot ("refresh_token", "device_id", ...).
const (
	keyringService           = "apitool-cli"
	keyringUserRefreshToken  = "refresh_token"
	keyringUserAccessToken   = "access_token"
	refreshTokenFileBaseName = "refresh_token.enc"
	accessTokenFileBaseName  = "access_token.enc"
	refreshLockBase          = "refresh.lock"
)

// ErrTokenNotFound is returned when a requested backend token has not been stored yet.
var ErrTokenNotFound = errors.New("backend: token not found")

// Storage is the CLI-internal interface for backend-token at-rest persistence.
// Two implementations: keychain-backed and encrypted-file
// fallback, selected at construction time per SPECIFICATION.md:7946-7951.
type Storage interface {
	GetRefreshToken() (string, error)
	SetRefreshToken(token string) error
	DeleteRefreshToken() error
	GetAccessToken() (string, error)
	SetAccessToken(token string) error
	DeleteAccessToken() error
	KeychainAvailable() bool
}

// StorageOptions configure Storage construction.
type StorageOptions struct {
	ConfigDir string // ~/.config/apitesttool by default; override via APITEST_CONFIG_DIR
	DeviceID  string // empty when called pre-login; HKDF-salted with machine-id
	// ForceFile, when true, skips the keychain probe and always uses the
	// encrypted-file backend. Tests use this to exercise the fallback path
	// independent of the host OS.
	ForceFile bool
}

// NewStorage probes the OS keychain and returns a Storage. The probe
// distinguishes "keychain present but slot empty" (keyring.ErrNotFound) —
// which is success for our purposes — from "keychain unreachable" (any
// other error, e.g. no D-Bus on headless Linux), which triggers fallback.
func NewStorage(opts StorageOptions) (Storage, error) {
	if opts.ConfigDir == "" {
		return nil, errors.New("backend: ConfigDir is required")
	}
	// Build HKDF salt: deviceID || 0x00 separator || machineID
	salt := make([]byte, 0, len(opts.DeviceID)+1+32)
	salt = append(salt, []byte(opts.DeviceID)...)
	salt = append(salt, 0)
	mid, err := machineID()
	if err != nil {
		return nil, fmt.Errorf("machine-id: %w", err)
	}
	salt = append(salt, mid...)

	refreshFile := newEncryptedFile(filepath.Join(opts.ConfigDir, refreshTokenFileBaseName), salt)
	accessFile := newEncryptedFile(filepath.Join(opts.ConfigDir, accessTokenFileBaseName), salt)

	if opts.ForceFile || os.Getenv("APITEST_FORCE_FILE_STORAGE") == "1" {
		return &fileStorage{refreshFile: refreshFile, accessFile: accessFile}, nil
	}
	if probeKeychainAvailable() {
		return &keychainStorage{}, nil
	}
	return &fileStorage{refreshFile: refreshFile, accessFile: accessFile}, nil
}

// probeKeychainAvailable probes whether the OS keychain is reachable.
// Returns true when the keychain service is up (even if the slot is empty).
func probeKeychainAvailable() bool {
	_, err := keyring.Get(keyringService, "_apitool_probe_unused_user_")
	if err == nil {
		return true
	}
	if errors.Is(err, keyring.ErrNotFound) {
		return true
	}
	return false
}

type keychainStorage struct{}

// KeychainAvailable reports that the OS keychain backend is active.
func (s *keychainStorage) KeychainAvailable() bool { return true }

// GetRefreshToken reads the refresh token from the OS keychain.
func (s *keychainStorage) GetRefreshToken() (string, error) {
	v, err := keyring.Get(keyringService, keyringUserRefreshToken)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrTokenNotFound
	}
	if err != nil {
		return "", fmt.Errorf("keyring get: %w", err)
	}
	return v, nil
}

// SetRefreshToken writes the refresh token to the OS keychain.
func (s *keychainStorage) SetRefreshToken(t string) error {
	if t == "" {
		return errors.New("backend: refresh token is empty")
	}
	if err := keyring.Set(keyringService, keyringUserRefreshToken, t); err != nil {
		return fmt.Errorf("keyring set: %w", err)
	}
	return nil
}

// DeleteRefreshToken removes the refresh token from the OS keychain.
func (s *keychainStorage) DeleteRefreshToken() error {
	err := keyring.Delete(keyringService, keyringUserRefreshToken)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("keyring delete: %w", err)
	}
	return nil
}

// GetAccessToken reads the API access token from the OS keychain.
func (s *keychainStorage) GetAccessToken() (string, error) {
	v, err := keyring.Get(keyringService, keyringUserAccessToken)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrTokenNotFound
	}
	if err != nil {
		return "", fmt.Errorf("keyring get access token: %w", err)
	}
	return v, nil
}

// SetAccessToken writes the API access token to the OS keychain.
func (s *keychainStorage) SetAccessToken(t string) error {
	if t == "" {
		return errors.New("backend: access token is empty")
	}
	if err := keyring.Set(keyringService, keyringUserAccessToken, t); err != nil {
		return fmt.Errorf("keyring set access token: %w", err)
	}
	return nil
}

// DeleteAccessToken removes the API access token from the OS keychain.
func (s *keychainStorage) DeleteAccessToken() error {
	err := keyring.Delete(keyringService, keyringUserAccessToken)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("keyring delete access token: %w", err)
	}
	return nil
}

type fileStorage struct {
	refreshFile *encryptedFile
	accessFile  *encryptedFile
}

// KeychainAvailable reports false for the encrypted-file fallback.
func (s *fileStorage) KeychainAvailable() bool { return false }

// GetRefreshToken reads the refresh token from the encrypted file.
func (s *fileStorage) GetRefreshToken() (string, error) {
	data, err := s.refreshFile.Read()
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrTokenNotFound
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// SetRefreshToken writes the refresh token to the encrypted file.
func (s *fileStorage) SetRefreshToken(t string) error {
	if t == "" {
		return errors.New("backend: refresh token is empty")
	}
	return s.refreshFile.Write([]byte(t))
}

// DeleteRefreshToken removes the encrypted token file.
func (s *fileStorage) DeleteRefreshToken() error {
	return s.refreshFile.Delete()
}

// GetAccessToken reads the API access token from the encrypted file.
func (s *fileStorage) GetAccessToken() (string, error) {
	data, err := s.accessFile.Read()
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrTokenNotFound
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// SetAccessToken writes the API access token to the encrypted file.
func (s *fileStorage) SetAccessToken(t string) error {
	if t == "" {
		return errors.New("backend: access token is empty")
	}
	return s.accessFile.Write([]byte(t))
}

// DeleteAccessToken removes the encrypted API access-token file.
func (s *fileStorage) DeleteAccessToken() error {
	return s.accessFile.Delete()
}
