package kage

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"filippo.io/age/agessh"
)

// DefaultSSHIdentities loads the default SSH identity used by kage.
func DefaultSSHIdentities() ([]age.Identity, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return []age.Identity{}, nil
	}

	path := filepath.Join(home, ".ssh", "id_ed25519")
	key, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []age.Identity{}, nil
		}
		return nil, fmt.Errorf("read ssh identity %q: %w", path, err)
	}
	identity, err := agessh.ParseIdentity(key)
	if err != nil {
		return nil, fmt.Errorf("parse ssh identity %q: %w", path, err)
	}
	return []age.Identity{identity}, nil
}

// DefaultSSHRecipient loads the public key for the default SSH identity.
func DefaultSSHRecipient() (age.Recipient, string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, "", nil
	}

	path := filepath.Join(home, ".ssh", "id_ed25519.pub")
	key, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("read ssh recipient %q: %w", path, err)
	}
	line := strings.TrimSpace(string(key))
	recipient, err := agessh.ParseRecipient(line)
	if err != nil {
		return nil, "", fmt.Errorf("parse ssh recipient %q: %w", path, err)
	}
	return recipient, line, nil
}

// LoadRecipients parses age and SSH recipients from a text file. HTTP(S)
// entries are fetched as authorized_keys files, and entries age can't parse
// as SSH recipients are skipped with a logged warning.
func LoadRecipients(path string) ([]age.Recipient, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open recipients file %q: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	var recipients []age.Recipient
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var parsed []age.Recipient
		if strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "http://") {
			parsed, err = loadRecipientsURL(line)
		} else if strings.HasPrefix(line, "ssh-") {
			var r age.Recipient
			r, err = agessh.ParseRecipient(line)
			parsed = []age.Recipient{r}
		} else {
			parsed, err = age.ParseRecipients(strings.NewReader(line))
		}
		if err != nil {
			return nil, fmt.Errorf("parse recipient %q in %q: %w", line, path, err)
		}

		recipients = append(recipients, parsed...)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read recipients file %q: %w", path, err)
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("no recipients in %q", path)
	}
	return recipients, nil
}

func loadRecipientsURL(rawURL string) ([]age.Recipient, error) {
	response, err := http.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("fetch %q: %w", rawURL, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("fetch %q: unexpected HTTP status %s", rawURL, response.Status)
	}
	var recipients []age.Recipient
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		recipient, err := agessh.ParseRecipient(line)
		if err != nil {
			log.Printf(
				"warning: ignoring unsupported SSH recipient from %q: %v",
				rawURL,
				err,
			)
			continue
		}
		recipients = append(recipients, recipient)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read keys from %q: %w", rawURL, err)
	}
	return recipients, nil
}

// ReadEncryptedFile decrypts and parses a kage secrets file.
func ReadEncryptedFile(path string, identities []age.Identity) (File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open encrypted file %q: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	reader, err := age.Decrypt(file, identities...)
	if err != nil {
		return nil, fmt.Errorf("decrypt file %q: %w", path, err)
	}
	secrets, err := parse(reader)
	if err != nil {
		return nil, fmt.Errorf("parse decrypted file %q: %w", path, err)
	}
	return secrets, nil
}

// EncryptFile writes a kage secrets file encrypted for the supplied recipients.
func EncryptFile(path string, recipients []age.Recipient, secrets File) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("open encrypted file %q: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	writer, err := age.Encrypt(file, recipients...)
	if err != nil {
		return fmt.Errorf("start encryption: %w", err)
	}
	if err := secrets.write(writer); err != nil {
		return fmt.Errorf("write encrypted file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish encryption: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close encrypted file: %w", err)
	}
	return nil
}
