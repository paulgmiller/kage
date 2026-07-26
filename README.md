# kage

`kage` stores Kubernetes Secret values in an
[age](https://github.com/FiloSottile/age)-encrypted file and synchronizes them
to a Kubernetes namespace.

It is intentionally small and opinionated. For a more general and powerful
secret-management system—with correspondingly more configuration and
complexity—see [SOPS](https://github.com/getsops/sops).

## Installation

Install the latest version with Go:

```sh
go install github.com/paulgmiller/kage@latest
```

Make sure Go's binary directory (normally `$(go env GOPATH)/bin`) is on your
`PATH`.

Kage currently uses `~/.ssh/id_ed25519` to decrypt secret files. Applying
secrets also requires a working Kubernetes configuration at
`~/.kube/config`.

## Secret file format

The decrypted file is dotenv-like. Start each Kubernetes Secret with a
`#secret:<name>` header:

```dotenv
#secret:api
API_TOKEN=replace-with-a-secret
DATABASE_URL="postgres://user:password@example/db"

#secret:worker
QUEUE_TOKEN=replace-with-another-secret
```

Secret names must be valid Kubernetes DNS subdomains. Keys may not be
duplicated within a secret, and values must contain at least five characters.

The file passed to `kage` must be encrypted with age. To allow `kage` to update
or re-encrypt it, place a `recipients.txt` file in the same directory:

```text
# One age or SSH recipient per line
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA...
```

For example, with the `age` command installed:

```sh
age -R secrets/recipients.txt -o secrets/envtest secrets/envtest.plaintext
```

Remove the plaintext copy securely after confirming that the encrypted file
can be read.

## Usage

The default encrypted file is `secrets/envtest`. Select another file with
`-secret-file`.

Inspect secret names and masked values:

```sh
kage -secret-file secrets/envtest -check
```

Preview changes to existing secrets in a namespace:

```sh
kage -secret-file secrets/envtest -ns my-namespace
```

Apply changes:

```sh
kage -secret-file secrets/envtest -ns my-namespace -apply
```

Add or update a value in the encrypted file:

```sh
kage -secret-file secrets/envtest -set 'api/API_TOKEN=new-secret-value'
```

If the encrypted file does not exist, `-set` creates it using the adjacent
`recipients.txt`. This is the simplest way to start a new file:

```sh
mkdir -p secrets
cp ~/.ssh/id_ed25519.pub secrets/recipients.txt
kage -secret-file secrets/envtest -set 'api/API_TOKEN=new-secret-value'
```

Re-encrypt the file using its adjacent `recipients.txt`, for example after
changing the recipient list:

```sh
kage -secret-file secrets/envtest -reencrypt
```

Show all command-line options:

```sh
kage -h
```

Kage creates opaque Kubernetes Secrets and marks them with the
`managed-by: github.com/paulgmiller/kage` annotation.

## Loading secrets locally

Applications written in Go can use `kage.Load()` similarly to
[`godotenv.Load()`](https://github.com/joho/godotenv). It first loads `.env`,
then decrypts and loads `secrets/envtest` when `~/.ssh/id_ed25519` matches a
recipient used to encrypt the file:

```go
package main

import (
	"log"
	"os"

	"github.com/paulgmiller/kage/pkg/kage"
)

func main() {
	if err := kage.Load(); err != nil {
		log.Fatal(err)
	}

	apiToken := os.Getenv("API_TOKEN")
	_ = apiToken
}
```

Like godotenv, `kage.Load()` does not overwrite environment variables that are
already set. This makes it possible to use ordinary values from `.env` and
encrypted local values from `secrets/envtest`, while allowing the shell or CI
environment to take precedence.

## End-to-end test

The end-to-end test requires Docker on a Linux host. It builds an isolated
test image, creates a kind cluster, and verifies two Secrets:

```sh
./test/e2e.sh
```

> **Note:** Preview mode suppresses updates to existing Secrets, but the
> current version creates a Secret immediately when it does not already exist.
> Check the target namespace before running the preview command.
