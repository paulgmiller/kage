package kage

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/joho/godotenv"
)

const (
	defaultSecretFile = "secrets/envtest"
	secretFileEnvVar  = "KAGE_SECRET_FILE"
)

// Load adds values from .env and an encrypted secrets file to the process
// environment without overwriting variables that are already set. The secret
// file can be supplied as the optional argument or through KAGE_SECRET_FILE;
// otherwise secrets/envtest is used. Relative secret paths are searched for
// from the working directory up to and including the Git root.
func Load(secretPaths ...string) error {
	if len(secretPaths) > 1 {
		return errors.New("kage.Load accepts at most one secret path")
	}

	entries, err := readEnv(".env")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	setMissingEnv(entries)

	secretPath := os.Getenv(secretFileEnvVar)
	if len(secretPaths) == 1 {
		secretPath = secretPaths[0]
	}
	if secretPath == "" {
		secretPath = defaultSecretFile
	}
	secretPath, err = findSecretFile(secretPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	identities, err := DefaultSSHIdentities()
	if err != nil {
		return fmt.Errorf("load SSH identity: %w", err)
	}
	if len(identities) == 0 {
		return nil
	}

	entries, err = readEncryptedEnv(secretPath, identities)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	setMissingEnv(entries)
	return nil
}

func findSecretFile(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	gitRoot, err := findGitRoot(workingDirectory)
	if err != nil {
		return "", err
	}

	secretFile, err := findInParentDirectories(workingDirectory, gitRoot, path)
	if err != nil {
		return "", fmt.Errorf("find secret file %q: %w", path, err)
	}
	return secretFile, nil
}

func findGitRoot(directory string) (string, error) {
	gitDirectory, err := findInParentDirectories(directory, "", ".git")
	if err == nil {
		return filepath.Dir(gitDirectory), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("find Git root from %q: %w", directory, err)
	}

	// Preserve the old current-directory-only behavior outside a Git worktree
	// instead of loading an unrelated ancestor's secrets.
	return directory, nil
}

func findInParentDirectories(directory, stopDirectory, path string) (string, error) {
	for {
		candidate := filepath.Join(directory, path)
		_, err := os.Stat(candidate)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat %q: %w", candidate, err)
		}
		if directory == stopDirectory {
			return "", os.ErrNotExist
		}

		parent := filepath.Dir(directory)
		if parent == directory {
			return "", os.ErrNotExist
		}
		directory = parent
	}
}

// parseEnv parses a dotenv-style stream. Empty lines and comments are ignored,
// and duplicate keys are rejected.
func parseEnv(r io.Reader) (map[string]string, error) {
	scanner := bufio.NewScanner(r)
	entries := make(map[string]string)
	var lines []string
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		rawLine := scanner.Text()
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			lines = append(lines, rawLine)
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))

		entry, err := parseEnvLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		if _, exists := entries[entry.Key]; exists {
			return nil, fmt.Errorf("line %d: duplicate env key %s", lineNumber, entry.Key)
		}
		entries[entry.Key] = entry.Value
		lines = append(lines, rawLine)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env: %w", err)
	}
	return godotenv.Unmarshal(strings.Join(lines, "\n"))
}

func readEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open env %q: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	entries, err := parseEnv(file)
	if err != nil {
		return nil, fmt.Errorf("parse env %q: %w", path, err)
	}
	return entries, nil
}

func readEncryptedEnv(path string, identities []age.Identity) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open encrypted env %q: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	reader, err := age.Decrypt(file, identities...)
	if err != nil {
		return nil, fmt.Errorf("decrypt env %q: %w", path, err)
	}
	entries, err := parseEnv(reader)
	if err != nil {
		return nil, fmt.Errorf("parse decrypted env %q: %w", path, err)
	}
	return entries, nil
}

func setMissingEnv(entries map[string]string) {
	for key, value := range entries {
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
