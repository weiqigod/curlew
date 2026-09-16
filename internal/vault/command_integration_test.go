package vault_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/variable"
	"github.com/weiqigod/curlew/internal/vault"
)

const (
	stubSecret   = "  fake-provider-secret-\u00e5  "
	stubToken    = "fake-token ' & %PATH% ! ^ \u96ea"
	stubAddress  = "https://vault.invalid/with space/\u96ea"
	stubRole     = "fake-role ' & %PATH% ! ^ \u96ea"
	stubSecretID = "fake-secret-id ' & %PATH% ! ^ \u96ea"
)

type providerInvocation struct {
	Args []string          `json:"args"`
	Env  map[string]string `json:"env"`
}

type nativeProviderCase struct {
	name            string
	program         string
	newProvider     func() vault.Provider
	fetchArgs       func(string) []string
	validateArgs    []string
	authPattern     string
	notFoundPattern string
}

func nativeProviderCases() []nativeProviderCase {
	return []nativeProviderCase{
		{"aws", "aws", func() vault.Provider { return vault.NewAWSProvider("region ' \u96ea", nil) },
			func(path string) []string {
				return []string{"secretsmanager", "get-secret-value", "--secret-id", path, "--region", "region ' \u96ea", "--query", "SecretString", "--output", "text"}
			},
			[]string{"sts", "get-caller-identity", "--region", "region ' \u96ea"}, "InvalidClientTokenId", "ResourceNotFoundException"},
		{"azure", "az", func() vault.Provider { return vault.NewAzureProvider("vault ' \u96ea", nil) },
			func(path string) []string {
				return []string{"keyvault", "secret", "show", "--name", path, "--vault-name", "vault ' \u96ea", "--query", "value", "-o", "tsv"}
			},
			[]string{"account", "show"}, "AADSTS", "SecretNotFound"},
		{"gcp", "gcloud", func() vault.Provider { return vault.NewGCPProvider("project ' \u96ea", nil) },
			func(path string) []string {
				return []string{"secrets", "versions", "access", "latest", "--secret=" + path, "--project=project ' \u96ea"}
			},
			[]string{"config", "get-value", "project"}, "UNAUTHENTICATED", "NOT_FOUND"},
		{"hashicorp", "vault", func() vault.Provider {
			return vault.NewHashiCorpProvider(stubAddress, vault.AuthConfig{Method: "token", Token: stubToken}, nil)
		},
			func(path string) []string { return []string{"kv", "get", "-format=json", path} },
			[]string{"token", "lookup", "-format=json"}, "permission denied", "No value found"},
		{"op", "op", func() vault.Provider { return vault.NewOnePasswordProvider(nil) },
			func(path string) []string {
				if strings.HasPrefix(path, "op://") {
					return []string{"read", path}
				}
				return []string{"item", "get", path, "--format", "json"}
			}, []string{"whoami"}, "not currently signed in", "no item named"},
	}
}

func providerGo(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Go", "bin", "go.exe")
	}
	return filepath.Join(runtime.GOROOT(), "bin", "go")
}

func buildProviderProgram(t *testing.T, source string) string {
	t.Helper()
	program := filepath.Join(t.TempDir(), "program"+providerExeSuffix())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, providerGo(t), "build", "-o", program, source)
	command.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", source, err, output)
	}
	return program
}

func providerExeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func setupProviderStub(t *testing.T, binary string, batch bool) (string, string) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "provider tools \u96ea")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, program := range []string{"aws", "az", "gcloud", "vault", "op", "provider-stub"} {
		name := program + providerExeSuffix()
		data := contents
		if batch && program != "provider-stub" {
			name = program + ".cmd"
			data = []byte("@echo off\r\n\"%~dp0provider-stub.exe\" %*\r\n")
		}
		if err := os.WriteFile(filepath.Join(directory, name), data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	path := directory
	if runtime.GOOS == "windows" {
		path += string(os.PathListSeparator) + filepath.Join(os.Getenv("SystemRoot"), "System32")
		t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	}
	t.Setenv("PATH", path)
	capture := filepath.Join(directory, "capture.jsonl")
	t.Setenv("CURLEW_STUB_CAPTURE", capture)
	t.Setenv("CURLEW_STUB_SECRET", stubSecret)
	t.Setenv("CURLEW_STUB_TOKEN", stubToken)
	t.Setenv("CURLEW_STUB_INHERITED", "inherited-value")
	t.Setenv("CURLEW_STUB_ERROR", "")
	t.Setenv("CURLEW_STUB_WAIT", "")
	t.Setenv("VAULT_ADDR", "parent-address")
	t.Setenv("VAULT_TOKEN", "parent-token")
	return directory, capture
}

func readProviderInvocations(t *testing.T, capture string) []providerInvocation {
	t.Helper()
	file, err := os.Open(capture)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var invocations []providerInvocation
	decoder := json.NewDecoder(file)
	for {
		var invocation providerInvocation
		err := decoder.Decode(&invocation)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		invocations = append(invocations, invocation)
	}
	return invocations
}

func requireProviderNoPanic(t *testing.T) {
	t.Helper()
	if recovered := recover(); recovered != nil {
		t.Fatalf("nil executor must select structured execution, but provider panicked: %v", recovered)
	}
}

func requireProviderSafeError(t *testing.T, err error, sensitive ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a command error")
	}
	if !errors.Is(err, variable.ErrCommandFailed) {
		t.Errorf("lost ErrCommandFailed: %v", err)
	}
	for _, value := range append(sensitive, stubSecret, stubToken, stubRole, stubSecretID, "private-stderr-marker") {
		if strings.Contains(err.Error(), value) {
			t.Errorf("error leaked %q: %v", value, err)
		}
	}
}

func TestProviderNativeCommands(t *testing.T) {
	binary := buildProviderProgram(t, "../../testdata/providerstub")
	for _, batch := range []bool{false, true} {
		mode := "executable"
		if batch {
			mode = "cmd-wrapper"
		}
		t.Run(mode, func(t *testing.T) {
			if batch && runtime.GOOS != "windows" {
				t.Skip("Windows batch contract")
			}
			for _, providerCase := range nativeProviderCases() {
				t.Run(providerCase.name, func(t *testing.T) {
					defer requireProviderNoPanic(t)
					_, capture := setupProviderStub(t, binary, batch)
					provider := providerCase.newProvider()
					if err := provider.ValidateConfig(); err != nil {
						t.Fatal(err)
					}
					paths := []string{"secret path '\u96ea &|<>^()%PATH%!\ttrailing\\", "second secret"}
					if !batch {
						paths[0] += ` "quoted"`
					}
					if providerCase.name == "op" {
						paths[1] = "op://vault/item \u96ea/field"
					}
					for _, path := range paths {
						value, err := provider.Fetch(context.Background(), path)
						if err != nil {
							t.Fatal(err)
						}
						want := stubSecret
						if providerCase.name == "hashicorp" {
							encoded, err := json.Marshal(map[string]string{"secret": stubSecret})
							if err != nil {
								t.Fatal(err)
							}
							want = string(encoded)
						}
						if value != want {
							t.Errorf("Fetch = %q, want %q", value, want)
						}
					}
					values, err := provider.BulkFetch(context.Background(), paths)
					if err != nil {
						t.Fatal(err)
					}
					if len(values) != len(paths) {
						t.Fatalf("bulk values: %v", values)
					}
					invocations := readProviderInvocations(t, capture)
					wantArgs := [][]string{providerCase.validateArgs, providerCase.fetchArgs(paths[0]), providerCase.fetchArgs(paths[1]), providerCase.fetchArgs(paths[0]), providerCase.fetchArgs(paths[1])}
					if len(invocations) != len(wantArgs) {
						t.Fatalf("invocations: %+v", invocations)
					}
					for index, invocation := range invocations {
						if !reflect.DeepEqual(invocation.Args, wantArgs[index]) {
							t.Errorf("argv[%d] = %#v, want %#v", index, invocation.Args, wantArgs[index])
						}
						wantAddress, wantToken := "parent-address", "parent-token"
						if providerCase.name == "hashicorp" {
							wantAddress, wantToken = stubAddress, stubToken
						}
						if invocation.Env["VAULT_ADDR"] != wantAddress || invocation.Env["VAULT_TOKEN"] != wantToken || invocation.Env["CURLEW_STUB_INHERITED"] != "inherited-value" {
							t.Errorf("child env: %v", invocation.Env)
						}
					}
					if os.Getenv("VAULT_ADDR") != "parent-address" || os.Getenv("VAULT_TOKEN") != "parent-token" {
						t.Fatal("parent environment mutated")
					}
					if batch {
						_, err := provider.Fetch(context.Background(), `private-path-"unsupported"`)
						requireProviderSafeError(t, err, "private-path-")
						if got := len(readProviderInvocations(t, capture)); got != len(wantArgs) {
							t.Errorf("unsupported batch quote launched process: %d", got)
						}
					}
				})
			}
			t.Run("approle", func(t *testing.T) {
				defer requireProviderNoPanic(t)
				_, capture := setupProviderStub(t, binary, batch)
				provider := vault.NewHashiCorpProvider(stubAddress, vault.AuthConfig{Method: "approle", RoleID: stubRole, SecretID: stubSecretID}, nil)
				if err := provider.ValidateConfig(); err != nil {
					t.Fatal(err)
				}
				if _, err := provider.BulkFetch(context.Background(), []string{"first", "second"}); err != nil {
					t.Fatal(err)
				}
				if err := provider.ValidateConfig(); err != nil {
					t.Fatal(err)
				}
				invocations := readProviderInvocations(t, capture)
				if len(invocations) != 5 {
					t.Fatalf("expected one login and four calls: %+v", invocations)
				}
				wantArgs := [][]string{{"write", "-format=json", "auth/approle/login", "role_id=" + stubRole, "secret_id=" + stubSecretID}, {"token", "lookup", "-format=json"}, {"kv", "get", "-format=json", "first"}, {"kv", "get", "-format=json", "second"}, {"token", "lookup", "-format=json"}}
				for index, invocation := range invocations {
					if !reflect.DeepEqual(invocation.Args, wantArgs[index]) {
						t.Errorf("argv[%d]: %#v", index, invocation.Args)
					}
					wantToken := stubToken
					if index == 0 {
						wantToken = "parent-token"
					}
					if invocation.Env["VAULT_TOKEN"] != wantToken || invocation.Env["VAULT_ADDR"] != stubAddress {
						t.Errorf("env[%d]: %v", index, invocation.Env)
					}
				}
				if os.Getenv("VAULT_TOKEN") != "parent-token" || os.Getenv("VAULT_ADDR") != "parent-address" {
					t.Fatal("parent environment mutated")
				}
			})
		})
	}
	for _, providerCase := range nativeProviderCases() {
		t.Run("errors/"+providerCase.name, func(t *testing.T) {
			for _, operation := range []string{"fetch", "validate", "bulk"} {
				for _, failure := range []struct {
					name, pattern string
					sentinel      error
				}{
					{"auth", providerCase.authPattern, vault.ErrProviderAuth},
					{"missing-secret", providerCase.notFoundPattern, vault.ErrSecretNotFound},
					{"generic", "unclassified failure", nil},
				} {
					t.Run(operation+"/"+failure.name, func(t *testing.T) {
						defer requireProviderNoPanic(t)
						setupProviderStub(t, binary, false)
						t.Setenv("CURLEW_STUB_ERROR", failure.pattern+" private-stderr-marker")
						provider := providerCase.newProvider()
						var err error
						switch operation {
						case "fetch":
							_, err = provider.Fetch(context.Background(), "private-secret-path")
						case "validate":
							err = provider.ValidateConfig()
						case "bulk":
							_, err = provider.BulkFetch(context.Background(), []string{"private-secret-path"})
						}
						requireProviderSafeError(t, err, "private-secret-path")
						if failure.sentinel != nil && !errors.Is(err, failure.sentinel) {
							t.Errorf("lost classification: %v", err)
						}
					})
				}
			}
			t.Run("missing-program", func(t *testing.T) {
				defer requireProviderNoPanic(t)
				directory, _ := setupProviderStub(t, binary, false)
				if err := os.Remove(filepath.Join(directory, providerCase.program+providerExeSuffix())); err != nil {
					t.Fatal(err)
				}
				provider := providerCase.newProvider()
				_, fetchErr := provider.Fetch(context.Background(), "private-secret-path")
				for _, err := range []error{fetchErr, provider.ValidateConfig()} {
					requireProviderSafeError(t, err, "private-secret-path")
					if !errors.Is(err, exec.ErrNotFound) || errors.Is(err, vault.ErrSecretNotFound) {
						t.Errorf("incorrect missing-program classification: %v", err)
					}
					if providerCase.name == "gcp" || providerCase.name == "op" {
						if !errors.Is(err, vault.ErrProviderAuth) || !strings.Contains(err.Error(), "Install") {
							t.Errorf("missing install hint: %v", err)
						}
					}
				}
			})
			t.Run("deadline", func(t *testing.T) {
				defer requireProviderNoPanic(t)
				setupProviderStub(t, binary, false)
				t.Setenv("CURLEW_STUB_WAIT", "yes")
				ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
				defer cancel()
				start := time.Now()
				_, err := providerCase.newProvider().Fetch(ctx, "private-secret-path")
				requireProviderSafeError(t, err, "private-secret-path")
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("lost deadline identity: %v", err)
				}
				if elapsed := time.Since(start); elapsed > 4*time.Second {
					t.Errorf("deadline took %v", elapsed)
				}
			})
			t.Run("cancelled", func(t *testing.T) {
				defer requireProviderNoPanic(t)
				setupProviderStub(t, binary, false)
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				_, err := providerCase.newProvider().Fetch(ctx, "private-secret-path")
				requireProviderSafeError(t, err, "private-secret-path")
				if !errors.Is(err, context.Canceled) {
					t.Errorf("lost cancellation identity: %v", err)
				}
			})
		})
	}
}
