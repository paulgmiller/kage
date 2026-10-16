package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"github.com/paulgmiller/kage/pkg/kage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestMergeCommand(t *testing.T) {
	// Use a generated SSH identity, never the developer's credentials.
	directory := t.TempDir()
	t.Setenv("HOME", directory)
	require.NoError(t, os.Mkdir(filepath.Join(directory, ".ssh"), 0o700))
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	block, err := ssh.MarshalPrivateKey(privateKey, "merge test")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, ".ssh", "id_ed25519"), pem.EncodeToMemory(block), 0o600))
	identity, err := agessh.NewEd25519Identity(privateKey)
	require.NoError(t, err)
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	require.NoError(t, err)
	recipient, err := agessh.ParseRecipient(string(ssh.MarshalAuthorizedKey(publicKey)))
	require.NoError(t, err)
	recipientsPath := filepath.Join(directory, recipientsFilename)
	require.NoError(t, os.WriteFile(recipientsPath, ssh.MarshalAuthorizedKey(publicKey), 0o600))
	base := filepath.Join(directory, "base")
	ours := filepath.Join(directory, "ours")
	theirs := filepath.Join(directory, "theirs")
	file := func(value string) kage.File {
		return kage.File{{Name: "api", Lines: []kage.Line{{Key: "TOKEN", Value: value}}}}
	}
	write := func(path string, secrets kage.File) {
		t.Helper()
		require.NoError(t, kage.EncryptFile(path, []age.Recipient{recipient}, secrets))
	}
	run := func(args ...string) error {
		cmd := newRootCommand()
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	write(base, file("original"))
	write(ours, file("ours-value"))
	write(theirs, append(file("original"), kage.Secret{Name: "worker", Lines: []kage.Line{{Key: "TOKEN", Value: "worker-value"}}}))
	// Overwrite an input only after all inputs have been read and merged.
	require.NoError(t, run("merge", base, ours, theirs, "--secret-file", ours))
	merged, err := kage.ReadEncryptedFile(ours, []age.Identity{identity})
	require.NoError(t, err)
	require.Len(t, merged, 2)
	assert.Equal(t, "ours-value", merged[0].Lines[0].Value)
	assert.Equal(t, "worker-value", merged[1].Lines[0].Value)
	info, err := os.Stat(ours)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	before, err := os.ReadFile(ours)
	require.NoError(t, err)
	write(theirs, file("theirs-value"))
	err = run("merge", base, ours, theirs, "-f", ours)
	require.EqualError(t, err, "merge conflicts: api/TOKEN")
	after, err := os.ReadFile(ours)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	// Recipient errors and malformed input also leave output untouched.
	write(theirs, file("original"))
	require.NoError(t, os.WriteFile(recipientsPath, nil, 0o600))
	require.ErrorContains(t, run("merge", base, ours, theirs, "-f", ours), "no recipients")
	after, err = os.ReadFile(ours)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	require.NoError(t, os.WriteFile(theirs, []byte("invalid ciphertext"), 0o600))
	require.ErrorContains(t, run("merge", base, ours, theirs, "-f", ours), "cannot decrypt or parse")
	after, err = os.ReadFile(ours)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	temps, err := filepath.Glob(filepath.Join(directory, ".kage-merge-*"))
	require.NoError(t, err)
	assert.Empty(t, temps)
	require.ErrorContains(t, run("merge", base, ours), "accepts 3 arg")
}
