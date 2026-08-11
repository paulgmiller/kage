package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"filippo.io/age"
	"github.com/paulgmiller/kage/pkg/kage"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	managedByAnnotationKey   = "managed-by"
	managedByAnnotationValue = "github.com/paulgmiller/kage"
	recipientsFilename       = "recipients.txt"
)

// kage is my dumbed down vesion of https://github.com/getsops/sops
func main() {
	path := flag.String("secret-file", "secrets/envtest", "encrypted file to apply to k8s namespace")
	namespace := flag.String("ns", "", "k8s namespace")
	check := flag.Bool("check", false, "dump secret names")
	setSecret := flag.String("set", "", "add or update a secret value as secret/key=value")
	create := flag.Bool("create", false, "create the secret file from all secrets in the namespace")
	reencrypt := flag.Bool("reencrypt", false, "re-encrypt the secret file using its recipients.txt")
	forreal := flag.Bool("apply", false, "actually apply secrets. Don't just print what would be done")
	flag.Parse()
	ctx := context.Background()
	if *create {
		if *namespace == "" {
			log.Fatal("namespace is required")
		}
		recipientsPath := filepath.Join(filepath.Dir(*path), recipientsFilename)
		recipients, err := kage.LoadRecipients(recipientsPath)
		if err != nil {
			log.Fatal(err)
		}
		cfg, err := kubeConfig()
		if err != nil {
			log.Fatal(err)
		}
		clientset, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			log.Fatal(err)
		}
		items, err := clientset.CoreV1().Secrets(*namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Fatalf("list secrets in %s: %s", *namespace, err)
		}
		secrets := fromK8s(items.Items)
		if len(secrets) == 0 {
			log.Fatalf("no secrets found in %s", *namespace)
		}
		if err := secrets.Validate(); err != nil {
			log.Fatalf("secrets in %s cannot be stored: %s", *namespace, err)
		}
		if err := kage.EncryptFile(*path, recipients, secrets); err != nil {
			log.Fatal(err)
		}
		log.Printf("created %s from %d secrets in %s", *path, len(secrets), *namespace)
		return
	}

	if *forreal {
		log.Printf("THIS IS NOT A DRILL")
	}

	identities, err := kage.DefaultSSHIdentities()
	if err != nil {
		log.Fatalf("need an identity %s", err)
	}
	secrets, err := kage.ReadEncryptedFile(*path, identities)
	if err != nil {
		if *setSecret == "" || !errors.Is(err, os.ErrNotExist) {
			log.Fatal(err)
		}
		secrets = kage.File{}
	}

	if *reencrypt || *setSecret != "" {
		// todo let them specify
		recipientsPath := filepath.Join(filepath.Dir(*path), recipientsFilename)

		recipients, err := kage.LoadRecipients(recipientsPath)
		if err != nil {
			log.Fatal(err)
		}
		if *setSecret != "" {
			secretName, key, value, err := parseSetArg(*setSecret)
			if err != nil {
				log.Fatal(err)
			}
			var changed bool
			secrets, changed = secrets.Set(secretName, key, value)
			if !changed {
				log.Printf("%s/%s unchanged", secretName, key)
				return
			}
			log.Printf("updated %s/%s", secretName, key)

			currentRecipient, currentRecipientLine, err := kage.DefaultSSHRecipient()
			if err != nil {
				log.Fatal(err)
			}
			recipients, err = promptForCurrentIdentity(
				os.Stdin,
				os.Stderr,
				recipientsPath,
				recipients,
				currentRecipient,
				currentRecipientLine,
			)
			if err != nil {
				log.Fatal(err)
			}
		}

		if err := secrets.Validate(); err != nil {
			log.Fatalf("updated secrets did not validate: %s", err)
		}
		if err := kage.EncryptFile(*path, recipients, secrets); err != nil {
			log.Fatal(err)
		}
		log.Printf("updated %s", *path)
		return
	}

	if *check {
		for _, secret := range secrets {
			fmt.Println(secret.Name)
			for _, line := range secret.Lines {
				if line.Key == "" {
					continue
				}
				fmt.Printf("  %s=%s\n", line.Key, maskedSecretValue(line.Value))
			}
			fmt.Println()
		}
		return
	}

	if namespace == nil || *namespace == "" {
		log.Fatal("namespace is required")
	}

	secretsK8s := toK8s(secrets)

	cfg, err := kubeConfig()
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	secretapi := clientset.CoreV1().Secrets(*namespace)
	for _, secret := range secretsK8s {
		current, err := secretapi.Get(ctx, secret.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			_, err = secretapi.Create(ctx, secret, metav1.CreateOptions{})
			if err != nil {
				log.Fatalf("failed to update %s: %s", secret.Name, err)
			}
			log.Printf("Created %s/%s", *namespace, secret.Name)
			continue
		}
		if !secretNeedsUpdate(current, secret) {
			continue
		}
		if !*forreal {
			log.Printf("would update %s/%s\n", *namespace, secret.Name)
			continue
		}
		secret.ResourceVersion = current.ResourceVersion
		_, err = secretapi.Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			log.Fatalf("failed to update %s: %s", secret.Name, err)
		}
		log.Printf("Updated %s/%s", *namespace, secret.Name)

	}
}

func kubeConfig() (*rest.Config, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(
		"",
		filepath.Join(os.Getenv("HOME"), ".kube", "config"),
	)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func fromK8s(items []corev1.Secret) kage.File {
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	secrets := make(kage.File, 0, len(items))
	for _, item := range items {
		keys := make([]string, 0, len(item.Data))
		for key := range item.Data {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		secret := kage.Secret{Name: item.Name, Lines: make([]kage.Line, 0, len(keys))}
		for _, key := range keys {
			secret.Lines = append(secret.Lines, kage.Line{Key: key, Value: string(item.Data[key])})
		}
		secrets = append(secrets, secret)
	}
	return secrets
}

func promptForCurrentIdentity(
	input io.Reader,
	output io.Writer,
	recipientsPath string,
	recipients []age.Recipient,
	current age.Recipient,
	currentLine string,
) ([]age.Recipient, error) {
	if current == nil || currentLine == "" {
		return recipients, nil
	}
	contents, err := os.ReadFile(recipientsPath)
	if err != nil {
		return nil, fmt.Errorf("read recipients file %q: %w", recipientsPath, err)
	}
	currentKey := strings.Join(strings.Fields(currentLine)[:2], " ")
	for line := range strings.Lines(string(contents)) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.Join(fields[:2], " ") == currentKey {
			return recipients, nil
		}
	}

	fmt.Fprintf(output, "Current SSH identity %s is not in %s. Add it? [y/N] ", currentKey, recipientsPath)
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read identity confirmation: %w", err)
	}
	answer = strings.TrimSpace(answer)
	if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
		return recipients, nil
	}

	if err := appendRecipient(recipientsPath, currentLine); err != nil {
		return nil, err
	}
	return append(recipients, current), nil
}

func appendRecipient(path, recipient string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read recipients file %q: %w", path, err)
	}
	prefix := ""
	if len(contents) > 0 && contents[len(contents)-1] != '\n' {
		prefix = "\n"
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return fmt.Errorf("open recipients file %q: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()
	if _, err := fmt.Fprintf(file, "%s%s\n", prefix, recipient); err != nil {
		return fmt.Errorf("append current identity to %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close recipients file %q: %w", path, err)
	}
	return nil
}

func secretNeedsUpdate(current, desired *corev1.Secret) bool {
	if current.Annotations[managedByAnnotationKey] != desired.Annotations[managedByAnnotationKey] {
		log.Printf("secret %s unmanged", desired.Name)
		return true
	}
	if len(current.Data) != len(desired.StringData) {
		log.Printf("secret %s key count mismatch", desired.Name)
		return true
	}
	for key, value := range desired.StringData {
		if !bytes.Equal(current.Data[key], []byte(value)) {
			log.Printf("secret %s key %s needs update", desired.Name, key)
			return true
		}
	}
	return false
}

func toK8s(secretVals kage.File) []*corev1.Secret {
	var secrets []*corev1.Secret
	for _, vals := range secretVals {
		stringData := map[string]string{}
		for _, line := range vals.Lines {
			if line.Key == "" {
				continue
			}
			stringData[line.Key] = line.Value
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: vals.Name,
				Annotations: map[string]string{
					managedByAnnotationKey: managedByAnnotationValue,
				},
			},
			Type:       corev1.SecretTypeOpaque,
			StringData: stringData,
		}
		secrets = append(secrets, secret)
	}
	return secrets
}

func parseSetArg(arg string) (string, string, string, error) {
	secretAndKey, value, found := strings.Cut(arg, "=")
	if !found {
		return "", "", "", fmt.Errorf("set value must be secret/key=value")
	}
	secretName, key, found := strings.Cut(secretAndKey, "/")
	if !found {
		return "", "", "", fmt.Errorf("set value must be secret/key=value")
	}
	secretName = strings.TrimSpace(secretName)
	key = strings.TrimSpace(key)
	if secretName == "" || key == "" {
		return "", "", "", fmt.Errorf("set value must be secret/key=value")
	}
	if len(value) < kage.MinSecretValueLength {
		return "", "", "", fmt.Errorf(
			"secret %s/%s must be at least %d characters",
			secretName,
			key,
			kage.MinSecretValueLength,
		)
	}
	return secretName, key, value, nil
}

func maskedSecretValue(value string) string {
	// invariant is value must be 5 or more characters, so this is safe
	return fmt.Sprintf("%s[%d]%s", value[:1], len(value), value[len(value)-1:])
}
