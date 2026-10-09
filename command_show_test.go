package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"github.com/paulgmiller/kage/pkg/kage"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestShowCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	privateBlock, err := ssh.MarshalPrivateKey(privateKey, "test")
	require.NoError(t, err)
	sshDir := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(sshDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(sshDir, "id_ed25519"), pem.EncodeToMemory(privateBlock), 0o600))
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	require.NoError(t, err)
	recipient, err := agessh.ParseRecipient(string(ssh.MarshalAuthorizedKey(publicKey)))
	require.NoError(t, err)

	secretFile := filepath.Join(t.TempDir(), "secrets.age")
	require.NoError(t, kage.EncryptFile(secretFile, []age.Recipient{recipient}, kage.File{
		{Name: "worker", Lines: []kage.Line{{Key: "TOKEN", Value: "other-value"}}},
		{Name: "api", Lines: []kage.Line{
			{Comment: "a comment"},
			{Key: "TOKEN", Value: "value with # quotes \" and ="},
		}},
	}))
	before, err := os.ReadFile(secretFile)
	require.NoError(t, err)

	for _, tt := range []struct {
		name    string
		arg     string
		output  string
		message string
	}{
		{name: "raw value", arg: "api/TOKEN", output: "value with # quotes \" and =\n"},
		{name: "missing secret", arg: "missing/TOKEN", message: `secret "missing" not found`},
		{name: "missing key", arg: "api/MISSING", message: `key "MISSING" not found in secret "api"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCommand()
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetArgs([]string{"show", "-f", secretFile, tt.arg})
			err := cmd.Execute()
			if tt.message != "" {
				require.ErrorContains(t, err, tt.message)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.output, output.String())
		})
	}
	after, err := os.ReadFile(secretFile)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
