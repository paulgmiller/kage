package kage_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/paulgmiller/kage/pkg/kage"

	"filippo.io/age"
	agessh "filippo.io/age/agessh"
)

const testSSHPrivateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACB7qx7CGF0+RlAe2W0yhkiKlf71UMVcDaxCDfkSqtRO1QAAAJhNAJ9JTQCf
SQAAAAtzc2gtZWQyNTUxOQAAACB7qx7CGF0+RlAe2W0yhkiKlf71UMVcDaxCDfkSqtRO1Q
AAAEAIOeRpdKSm4SAwH+TzGtR01RQoGiR/PSEns26+wH1GXXurHsIYXT5GUB7ZbTKGSIqV
/vVQxVwNrEIN+RKq1E7VAAAAEXJvb3RAZTIzYzBmOTM1ZGFmAQIDBA==
-----END OPENSSH PRIVATE KEY-----`

const testSSHPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHurHsIYXT5GUB7ZbTKGSIqV/vVQxVwNrEIN+RKq1E7V test@careme"

func TestLoadDotAndEncryptedWithoutOverride(t *testing.T) {
	t.Setenv("KEEP", "already")
	t.Setenv("KAGE_SECRET_FILE", "")

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir(%q) error = %v", tmp, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	if err := os.WriteFile(".env", []byte("DOTENV_KEY=plain\nKEEP=from-dotenv\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	sshDir := filepath.Join(tmp, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(.ssh) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), []byte(testSSHPrivateKey), 0o600); err != nil {
		t.Fatalf("WriteFile(id_ed25519) error = %v", err)
	}

	if err := os.MkdirAll(filepath.Join("secrets"), 0o700); err != nil {
		t.Fatalf("MkdirAll(secrets) error = %v", err)
	}

	ciphertext, err := encryptWithRecipient("SECRET_KEY=encrypted\n", testSSHPublicKey)
	if err != nil {
		t.Fatalf("encryptWithRecipient() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join("secrets", "envtest"), ciphertext, 0o600); err != nil {
		t.Fatalf("WriteFile(secrets/envtest) error = %v", err)
	}

	if err := kage.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := os.Getenv("DOTENV_KEY"); got != "plain" {
		t.Fatalf("DOTENV_KEY = %q, want %q", got, "plain")
	}
	if got := os.Getenv("SECRET_KEY"); got != "encrypted" {
		t.Fatalf("SECRET_KEY = %q, want %q", got, "encrypted")
	}
	if got := os.Getenv("KEEP"); got != "already" {
		t.Fatalf("KEEP = %q, want %q", got, "already")
	}
}

func TestLoadFindsDefaultSecretFileUpToGitRoot(t *testing.T) {
	t.Setenv("KAGE_SECRET_FILE", "")
	unsetEnv(t, "UPWARD_DEFAULT_KEY")

	tmp := t.TempDir()
	repository := filepath.Join(tmp, "repository")
	workingDirectory := filepath.Join(repository, "one", "two")
	mustMkdirAll(t, filepath.Join(repository, ".git"))
	mustMkdirAll(t, workingDirectory)
	installTestIdentity(t, filepath.Join(tmp, "home"))
	writeEncryptedEnv(t, filepath.Join(repository, "secrets", "envtest"), "UPWARD_DEFAULT_KEY=found\n")
	changeWorkingDirectory(t, workingDirectory)

	if err := kage.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := os.Getenv("UPWARD_DEFAULT_KEY"); got != "found" {
		t.Fatalf("UPWARD_DEFAULT_KEY = %q, want %q", got, "found")
	}
}

func TestLoadUsesSecretFileEnvironmentVariable(t *testing.T) {
	t.Setenv("KAGE_SECRET_FILE", filepath.Join("config", "local.age"))
	unsetEnv(t, "CONFIGURED_SECRET_KEY")

	tmp := t.TempDir()
	repository := filepath.Join(tmp, "repository")
	workingDirectory := filepath.Join(repository, "nested")
	mustMkdirAll(t, filepath.Join(repository, ".git"))
	mustMkdirAll(t, workingDirectory)
	installTestIdentity(t, filepath.Join(tmp, "home"))
	writeEncryptedEnv(t, filepath.Join(repository, "config", "local.age"), "CONFIGURED_SECRET_KEY=environment\n")
	changeWorkingDirectory(t, workingDirectory)

	if err := kage.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := os.Getenv("CONFIGURED_SECRET_KEY"); got != "environment" {
		t.Fatalf("CONFIGURED_SECRET_KEY = %q, want %q", got, "environment")
	}
}

func TestLoadArgumentOverridesSecretFileEnvironmentVariable(t *testing.T) {
	t.Setenv("KAGE_SECRET_FILE", "from-environment.age")
	unsetEnv(t, "ARGUMENT_SECRET_KEY")
	unsetEnv(t, "ENVIRONMENT_SECRET_KEY")

	tmp := t.TempDir()
	installTestIdentity(t, filepath.Join(tmp, "home"))
	writeEncryptedEnv(t, filepath.Join(tmp, "from-environment.age"), "ENVIRONMENT_SECRET_KEY=wrong\n")
	writeEncryptedEnv(t, filepath.Join(tmp, "from-argument.age"), "ARGUMENT_SECRET_KEY=right\n")
	changeWorkingDirectory(t, tmp)

	if err := kage.Load("from-argument.age"); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := os.Getenv("ARGUMENT_SECRET_KEY"); got != "right" {
		t.Fatalf("ARGUMENT_SECRET_KEY = %q, want %q", got, "right")
	}
	if _, exists := os.LookupEnv("ENVIRONMENT_SECRET_KEY"); exists {
		t.Fatal("ENVIRONMENT_SECRET_KEY was loaded from the overridden environment path")
	}
}

func TestLoadDoesNotSearchAboveGitRoot(t *testing.T) {
	t.Setenv("KAGE_SECRET_FILE", "")
	unsetEnv(t, "ABOVE_GIT_ROOT_KEY")

	tmp := t.TempDir()
	repository := filepath.Join(tmp, "repository")
	workingDirectory := filepath.Join(repository, "nested")
	mustMkdirAll(t, filepath.Join(repository, ".git"))
	mustMkdirAll(t, workingDirectory)
	installTestIdentity(t, filepath.Join(tmp, "home"))
	writeEncryptedEnv(t, filepath.Join(tmp, "secrets", "envtest"), "ABOVE_GIT_ROOT_KEY=wrong\n")
	changeWorkingDirectory(t, workingDirectory)

	if err := kage.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, exists := os.LookupEnv("ABOVE_GIT_ROOT_KEY"); exists {
		t.Fatal("ABOVE_GIT_ROOT_KEY was loaded from above the Git root")
	}
}

func TestLoadRejectsMultipleSecretPaths(t *testing.T) {
	err := kage.Load("first", "second")
	if err == nil {
		t.Fatal("Load() error = nil, want an error")
	}
}

func changeWorkingDirectory(t *testing.T, directory string) {
	t.Helper()
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatalf("Chdir(%q) error = %v", directory, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWorkingDirectory)
	})
}

func installTestIdentity(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	sshDirectory := filepath.Join(home, ".ssh")
	mustMkdirAll(t, sshDirectory)
	if err := os.WriteFile(filepath.Join(sshDirectory, "id_ed25519"), []byte(testSSHPrivateKey), 0o600); err != nil {
		t.Fatalf("WriteFile(id_ed25519) error = %v", err)
	}
}

func writeEncryptedEnv(t *testing.T, path, plaintext string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	ciphertext, err := encryptWithRecipient(plaintext, testSSHPublicKey)
	if err != nil {
		t.Fatalf("encryptWithRecipient() error = %v", err)
	}
	if err := os.WriteFile(path, ciphertext, 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	oldValue, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%q) error = %v", key, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, oldValue)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func encryptWithRecipient(plaintext, publicKey string) ([]byte, error) {
	recipient, err := agessh.ParseRecipient(publicKey)
	if err != nil {
		return nil, err
	}

	var ciphertext bytes.Buffer
	writer, err := age.Encrypt(&ciphertext, recipient)
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(writer, plaintext); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return ciphertext.Bytes(), nil
}
