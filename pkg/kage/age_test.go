package kage

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestLoadRecipientsFromURLSupportsAgeSSHRecipients(t *testing.T) {
	_, ed25519PrivateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	ed25519PublicKey, err := ssh.NewPublicKey(ed25519PrivateKey.Public())
	require.NoError(t, err)
	ed25519Identity, err := agessh.NewEd25519Identity(ed25519PrivateKey)
	require.NoError(t, err)

	ecdsaPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecdsaPublicKey, err := ssh.NewPublicKey(ecdsaPrivateKey.Public())
	require.NoError(t, err)
	rsaPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rsaPublicKey, err := ssh.NewPublicKey(rsaPrivateKey.Public())
	require.NoError(t, err)
	rsaIdentity, err := agessh.NewRSAIdentity(rsaPrivateKey)
	require.NoError(t, err)

	servedKeys := ssh.MarshalAuthorizedKey(ecdsaPublicKey)
	servedKeys = append(servedKeys, ssh.MarshalAuthorizedKey(ed25519PublicKey)...)
	servedKeys = append(servedKeys, ssh.MarshalAuthorizedKey(rsaPublicKey)...)
	const keysURL = "https://github.com/example.keys"
	previousTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, keysURL, request.URL.String())
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(bytes.NewReader(servedKeys)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = previousTransport
	})

	recipientsPath := filepath.Join(t.TempDir(), "recipients.txt")
	require.NoError(t, os.WriteFile(recipientsPath, []byte(keysURL+"\n"), 0o600))
	var warnings bytes.Buffer
	previousLogOutput := log.Writer()
	log.SetOutput(&warnings)
	t.Cleanup(func() {
		log.SetOutput(previousLogOutput)
	})

	recipients, err := LoadRecipients(recipientsPath)
	require.NoError(t, err)
	require.Len(t, recipients, 2)
	assert.Contains(t, warnings.String(), `warning: ignoring unsupported SSH recipient`)
	assert.Contains(t, warnings.String(), `ecdsa-sha2-nistp256`)
	assert.Contains(t, warnings.String(), keysURL)

	var ciphertext bytes.Buffer
	writer, err := age.Encrypt(&ciphertext, recipients...)
	require.NoError(t, err)
	_, err = writer.Write([]byte("secret"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	for _, identity := range []age.Identity{ed25519Identity, rsaIdentity} {
		reader, err := age.Decrypt(bytes.NewReader(ciphertext.Bytes()), identity)
		require.NoError(t, err)
		plaintext, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, "secret", string(plaintext))
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
