package backend

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrEncryptedFileTampered is returned when the GCM auth tag does not
// verify — the file has been corrupted, replaced, or the key has changed
// (e.g. machine-id rotated).
var ErrEncryptedFileTampered = errors.New("backend: encrypted file tampered or wrong key")

// encryptedFile implements the AES-256-GCM at-rest storage primitive used
// by the encrypted-file fallback. Wire format (file contents):
//
//	magic[4] | version[1] | nonce[12] | ciphertext+tag[N]
//
// magic = "ATOK" (apitest opaque), version = 0x01.
type encryptedFile struct {
	path    string
	keySalt []byte // (deviceID || machineID) bytes
}

const (
	encFileMagic   = "ATOK"
	encFileVersion = byte(0x01)
	encFilePerm    = fs.FileMode(0o600)
	nonceSize      = 12 // AES-GCM standard nonce
)

// newEncryptedFile constructs an encryptedFile bound to path. keySalt is
// the HKDF salt — typically (deviceID || machineID). When deviceID is
// empty (M14-004 pre-login state), machineID alone is sufficient.
func newEncryptedFile(path string, keySalt []byte) *encryptedFile {
	return &encryptedFile{path: path, keySalt: keySalt}
}

// Write encrypts plaintext and writes it to f.path with mode 0600.
// Atomic via write-temp + rename.
func (f *encryptedFile) Write(plaintext []byte) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return fmt.Errorf("mkdir parent: %w", err)
	}
	key, err := deriveKey(f.keySalt)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("gcm: %w", err)
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("nonce: %w", err)
	}
	ct := aead.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(encFileMagic)+1+nonceSize+len(ct))
	out = append(out, []byte(encFileMagic)...)
	out = append(out, encFileVersion)
	out = append(out, nonce...)
	out = append(out, ct...)
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, out, encFilePerm); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := os.Rename(tmp, f.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// Read decrypts and returns the file contents. Returns os.ErrNotExist if
// the file is absent, ErrEncryptedFileTampered if the GCM tag does not
// verify or the magic/version is wrong.
func (f *encryptedFile) Read() ([]byte, error) {
	raw, err := os.ReadFile(f.path)
	if err != nil {
		return nil, err
	}
	headerLen := len(encFileMagic) + 1
	if len(raw) < headerLen+nonceSize {
		return nil, fmt.Errorf("%w: file too short", ErrEncryptedFileTampered)
	}
	if string(raw[:len(encFileMagic)]) != encFileMagic {
		return nil, fmt.Errorf("%w: bad magic", ErrEncryptedFileTampered)
	}
	if raw[len(encFileMagic)] != encFileVersion {
		return nil, fmt.Errorf("%w: bad version", ErrEncryptedFileTampered)
	}
	body := raw[headerLen:]
	nonce := body[:nonceSize]
	ct := body[nonceSize:]
	key, err := deriveKey(f.keySalt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncryptedFileTampered, err)
	}
	return pt, nil
}

// Delete removes the file; absent files are a no-op.
func (f *encryptedFile) Delete() error {
	err := os.Remove(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// deriveKey runs HKDF-SHA256 over keySalt with a project-specific info
// label, producing a 32-byte AES-256 key.
func deriveKey(salt []byte) ([]byte, error) {
	if len(salt) == 0 {
		return nil, errors.New("backend: empty key salt — machine-id resolution failed")
	}
	// ikm is a fixed product identifier; the entropy comes from salt.
	ikm := []byte("apitool-cli/refresh-token/v1")
	return hkdf.Key(sha256.New, ikm, salt, "apitool-encrypted-file", 32)
}

// machineID returns a stable per-machine identifier as a byte string.
// Best-effort: returns a runtime fallback (hostname + GOOS) when the
// platform-specific source is unavailable. The encrypted-file path is only
// used when the OS keychain is also unavailable; at-rest secrecy is
// bounded by file mode 0600 in that scenario.
func machineID() ([]byte, error) {
	switch runtime.GOOS {
	case "linux":
		for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if data, err := os.ReadFile(p); err == nil {
				return []byte(strings.TrimSpace(string(data))), nil
			}
		}
	}
	// Fallback: hostname + GOOS + product salt. NOT a security boundary —
	// the encrypted-file path is only used when keychain is unavailable,
	// and at-rest secrecy is bounded by file mode 0600 anyway.
	host, _ := os.Hostname()
	return []byte("apitool/fallback/" + runtime.GOOS + "/" + host), nil
}
