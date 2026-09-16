package vault_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
	"gopkg.in/yaml.v3"
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
	return filepath.Join(runtime.GOROOT(), "bin", "go"+providerExeSuffix())
}

func buildProviderProgram(t *testing.T, source string) string {
	t.Helper()
	program := filepath.Join(t.TempDir(), filepath.Base(source)+providerExeSuffix())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, providerGo(t), "build", "-buildvcs=false", "-o", program, source)
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
	t.Setenv("CURLEW_STUB_OUTPUT", "")
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
	t.Run("real-binary", func(t *testing.T) {
		testProviderRealBinary(t, binary)
	})
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
					wantValues := make(map[string]string, len(paths))
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
						wantValues[path] = want
					}
					values, err := provider.BulkFetch(context.Background(), paths)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(values, wantValues) {
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
			setupProviderStub(t, binary, false)
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
						var exitErr *exec.ExitError
						if !errors.As(err, &exitErr) || exitErr.ExitCode() != 42 {
							t.Errorf("lost exit code: %v", err)
						}
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
	t.Run("approle-errors", func(t *testing.T) {
		setupProviderStub(t, binary, false)
		for _, operation := range []string{"fetch", "validate"} {
			for _, pattern := range []string{"invalid role", "connection refused", "unclassified failure"} {
				t.Run(operation+"/"+pattern, func(t *testing.T) {
					t.Setenv("CURLEW_STUB_ERROR", pattern+" private-stderr-marker")
					provider := vault.NewHashiCorpProvider(stubAddress, vault.AuthConfig{Method: "approle", RoleID: stubRole, SecretID: stubSecretID}, nil)
					var err error
					if operation == "validate" {
						err = provider.ValidateConfig()
					} else {
						_, err = provider.Fetch(context.Background(), "private-secret-path")
					}
					requireProviderSafeError(t, err, "private-secret-path")
					if pattern == "invalid role" && !errors.Is(err, vault.ErrProviderAuth) {
						t.Errorf("lost auth classification: %v", err)
					}
					if pattern == "connection refused" && !strings.Contains(err.Error(), "Vault server") {
						t.Errorf("missing network hint: %v", err)
					}
				})
			}
		}
	})
	t.Run("hashicorp-invalid-output", func(t *testing.T) {
		setupProviderStub(t, binary, false)
		for _, fixture := range []struct {
			name, output, private, operation string
		}{
			{"response", `98765432123456789`, "98765432123456789", "failed to parse response"},
			{"data", `{"data":98765432123456789}`, "98765432123456789", "failed to parse data field"},
			{"secret", `{"data":{"data":{"private-field":98765432123456789e999}}}`, "98765432123456789e999", "failed to parse secret data"},
			{"approle", `{"auth":{"client_token":98765432123456789}}`, "98765432123456789", "failed to parse approle login response"},
		} {
			t.Run(fixture.name, func(t *testing.T) {
				t.Setenv("CURLEW_STUB_OUTPUT", fixture.output)
				auth := vault.AuthConfig{Method: "token", Token: stubToken}
				if fixture.name == "approle" {
					auth = vault.AuthConfig{Method: "approle", RoleID: stubRole, SecretID: stubSecretID}
				}
				_, err := vault.NewHashiCorpProvider(stubAddress, auth, nil).Fetch(context.Background(), "private-secret-path")
				if err == nil {
					t.Fatal("expected invalid JSON response error")
				}
				if strings.Contains(err.Error(), fixture.private) {
					t.Errorf("JSON error leaked response value: %v", err)
				}
				if !strings.Contains(err.Error(), fixture.operation) {
					t.Errorf("lost parse operation: %v", err)
				}
			})
		}
	})
}

func testProviderRealBinary(t *testing.T, stubBinary string) {
	cli := buildProviderProgram(t, "../../cmd/curlew")
	for _, providerName := range []string{"project-hashicorp", "team-aws", "team-azure"} {
		t.Run(providerName, func(t *testing.T) {
			_, capture := setupProviderStub(t, stubBinary, false)
			project := t.TempDir()
			t.Setenv("CURLEW_CONFIG_DIR", filepath.Join(project, "user-config"))
			t.Setenv("CURLEW_TEAM_CONFIG", "")
			t.Setenv("CURLEW_VAULT_STUB", "")
			stdout, stderr, err := runProviderCLI(t, cli, project, "init")
			if err != nil {
				t.Fatalf("init: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
			}
			secretRef := "{{vault_value}}"
			args := []string{"run", "native.yaml", "--format", "json", "-vv", "--events", "events.jsonl"}
			projectConfig := map[string]any{"project_name": "native-provider-test"}
			if providerName == "project-hashicorp" {
				projectConfig["secrets"] = map[string]any{
					"provider": vault.ProviderHashiCorp,
					"address":  stubAddress,
					"auth":     map[string]any{"method": "token", "token": stubToken},
					"keys":     map[string]string{"vault_value": "secret path '\u96ea#secret"},
				}
			} else {
				secretRef = "{{secrets.vault_value}}"
				providerConfig := map[string]any{"keys": map[string]string{"vault_value": "secret path '\u96ea"}}
				if providerName == "team-aws" {
					providerConfig["provider"], providerConfig["region"] = vault.ProviderAWS, "region '\u96ea"
				} else {
					providerConfig["provider"], providerConfig["vault_name"] = vault.ProviderAzure, "vault '\u96ea"
				}
				teamFile := filepath.Join(project, "team.yaml")
				writeProviderYAML(t, teamFile, map[string]any{"team_secrets": map[string]any{"vault_configs": map[string]any{"fixture": providerConfig}}})
				t.Setenv("CURLEW_TEAM_CONFIG", teamFile)
				args = append(args, "--env", "fixture")
			}
			writeProviderYAML(t, filepath.Join(project, "curlew.yaml"), projectConfig)
			commandSecret := "fake-command-value-\u96ea & only-local"
			command := "printf '%s' '" + commandSecret + "'"
			if runtime.GOOS == "windows" {
				command = "Write-Output '" + commandSecret + "'"
			}
			received := make(chan map[string]string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				var body map[string]string
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Errorf("decode loopback body: %v", err)
					response.WriteHeader(http.StatusBadRequest)
					return
				}
				select {
				case received <- body:
				default:
					t.Error("unexpected extra loopback request")
				}
				response.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(response).Encode(body)
			}))
			defer server.Close()
			body, err := json.Marshal(map[string]string{"command": "{{command_value}}", "vault": secretRef})
			if err != nil {
				t.Fatal(err)
			}
			writeProviderYAML(t, filepath.Join(project, "native.yaml"), map[string]any{
				"name":      "native providers",
				"variables": map[string]any{"command_value": map[string]any{"from_command": command, "sensitive": true}},
				"requests": []any{map[string]any{
					"name":       "loopback exact values",
					"request":    map[string]any{"method": "POST", "url": server.URL, "body": string(body)},
					"assertions": map[string]any{"status": 200},
				}},
			})
			stdout, stderr, err = runProviderCLI(t, cli, project, args...)
			if err != nil {
				t.Fatalf("run: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
			}
			invocations := readProviderInvocations(t, capture)
			if len(invocations) != 1 {
				t.Fatalf("expected one provider fetch: %+v", invocations)
			}
			wantArgs := []string{"kv", "get", "-format=json", "secret path '\u96ea"}
			if providerName == "team-aws" {
				wantArgs = []string{"secretsmanager", "get-secret-value", "--secret-id", "secret path '\u96ea", "--region", "region '\u96ea", "--query", "SecretString", "--output", "text"}
			} else if providerName == "team-azure" {
				wantArgs = []string{"keyvault", "secret", "show", "--name", "secret path '\u96ea", "--vault-name", "vault '\u96ea", "--query", "value", "-o", "tsv"}
			}
			if !reflect.DeepEqual(invocations[0].Args, wantArgs) {
				t.Errorf("runner argv = %#v, want %#v", invocations[0].Args, wantArgs)
			}
			select {
			case actual := <-received:
				want := map[string]string{"command": commandSecret, "vault": stubSecret}
				if !reflect.DeepEqual(actual, want) {
					t.Errorf("received %#v, want %#v", actual, want)
				}
			default:
				t.Fatal("CLI did not reach loopback server")
			}
			events, err := os.ReadFile(filepath.Join(project, "events.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			for name, data := range map[string]string{"stdout": stdout, "stderr": stderr, "events": string(events)} {
				checkProviderOutputSecrets(t, name, data, []string{commandSecret, stubSecret, stubToken})
			}
			if !json.Valid([]byte(stdout)) {
				t.Errorf("stdout is not JSON: %s", stdout)
			}
			decoder := json.NewDecoder(bytes.NewReader(events))
			foundRequest := false
			for {
				var event map[string]any
				err := decoder.Decode(&event)
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if event["kind"] == "request.end" {
					foundRequest = true
				}
			}
			if !foundRequest {
				t.Fatal("missing request.end event")
			}
		})
	}
}

func runProviderCLI(t *testing.T, binary, directory string, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = directory
	command.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func writeProviderYAML(t *testing.T, path string, value any) {
	t.Helper()
	data, err := yaml.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func checkProviderOutputSecrets(t *testing.T, name, data string, secrets []string) {
	t.Helper()
	var inspect func(string, any)
	inspect = func(location string, value any) {
		switch value := value.(type) {
		case string:
			for _, secret := range secrets {
				if strings.Contains(value, secret) {
					t.Errorf("%s leaked %q", location, secret)
				}
			}
			decoder := json.NewDecoder(strings.NewReader(value))
			for {
				var nested any
				if err := decoder.Decode(&nested); err != nil {
					break
				}
				inspect(location, nested)
			}
		case map[string]any:
			for key, child := range value {
				inspect(location+"."+key, child)
			}
		case []any:
			for _, child := range value {
				inspect(location, child)
			}
		}
	}
	inspect(name, data)
}
