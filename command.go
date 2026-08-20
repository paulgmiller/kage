package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/paulgmiller/kage/pkg/kage"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	managedByAnnotationKey   = "managed-by"
	managedByAnnotationValue = "github.com/paulgmiller/kage"
	recipientsFilename       = "recipients.txt"
	defaultSecretFile        = "secrets/envtest"
)

type commandOptions struct {
	secretFile string
	input      io.Reader
	output     io.Writer
	errOutput  io.Writer
}

func newRootCommand() *cobra.Command {
	opts := &commandOptions{input: os.Stdin, output: os.Stdout, errOutput: os.Stderr}
	cmd := &cobra.Command{
		Use:           "kage",
		Short:         "Manage age-encrypted Kubernetes secrets",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.PersistentFlags().StringVarP(&opts.secretFile, "secret-file", "f", defaultSecretFile, "encrypted secret file")
	cmd.AddCommand(
		newCheckCommand(opts),
		newApplyCommand(opts),
		newSetCommand(opts),
		newCreateCommand(opts),
		newReencryptCommand(opts),
	)
	return cmd
}

func newCheckCommand(opts *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Show secret names and masked values",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			secrets, err := readSecrets(opts.secretFile)
			if err != nil {
				return err
			}
			for _, secret := range secrets {
				fmt.Fprintln(opts.output, secret.Name)
				for _, line := range secret.Lines {
					if line.Key != "" {
						fmt.Fprintf(opts.output, "  %s=%s\n", line.Key, maskedSecretValue(line.Value))
					}
				}
				fmt.Fprintln(opts.output)
			}
			return nil
		},
	}
}

func newSetCommand(opts *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "set SECRET/KEY=VALUE",
		Short: "Add or update an encrypted secret value",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			secretName, key, value, err := parseSetArg(args[0])
			if err != nil {
				return err
			}
			secrets, err := readSecrets(opts.secretFile)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if errors.Is(err, os.ErrNotExist) {
				secrets = kage.File{}
			}
			secrets, changed := secrets.Set(secretName, key, value)
			if !changed {
				log.Printf("%s/%s unchanged", secretName, key)
				return nil
			}
			recipientsPath := filepath.Join(filepath.Dir(opts.secretFile), recipientsFilename)
			recipients, err := kage.LoadRecipients(recipientsPath)
			if err != nil {
				return err
			}
			current, currentLine, err := kage.DefaultSSHRecipient()
			if err != nil {
				return err
			}
			recipients, err = promptForCurrentIdentity(opts.input, opts.errOutput, recipientsPath, recipients, current, currentLine)
			if err != nil {
				return err
			}
			if err := secrets.Validate(); err != nil {
				return fmt.Errorf("updated secrets did not validate: %w", err)
			}
			if err := kage.EncryptFile(opts.secretFile, recipients, secrets); err != nil {
				return err
			}
			log.Printf("updated %s/%s in %s", secretName, key, opts.secretFile)
			return nil
		},
	}
}

func newReencryptCommand(opts *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use: "reencrypt", Short: "Re-encrypt the file using its recipients.txt", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			secrets, err := readSecrets(opts.secretFile)
			if err != nil {
				return err
			}
			recipients, err := kage.LoadRecipients(filepath.Join(filepath.Dir(opts.secretFile), recipientsFilename))
			if err != nil {
				return err
			}
			if err := kage.EncryptFile(opts.secretFile, recipients, secrets); err != nil {
				return err
			}
			log.Printf("updated %s", opts.secretFile)
			return nil
		},
	}
}

func newCreateCommand(opts *commandOptions) *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use: "create", Short: "Create an encrypted file from a Kubernetes namespace", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if namespace == "" {
				return errors.New("namespace is required")
			}
			recipients, err := kage.LoadRecipients(filepath.Join(filepath.Dir(opts.secretFile), recipientsFilename))
			if err != nil {
				return err
			}
			clientset, err := kubernetesClient()
			if err != nil {
				return err
			}
			items, err := clientset.CoreV1().Secrets(namespace).List(context.Background(), metav1.ListOptions{})
			if err != nil {
				return fmt.Errorf("list secrets in %s: %w", namespace, err)
			}
			secrets := fromK8s(items.Items)
			if len(secrets) == 0 {
				return fmt.Errorf("no secrets found in %s", namespace)
			}
			if err := secrets.Validate(); err != nil {
				return fmt.Errorf("secrets in %s cannot be stored: %w", namespace, err)
			}
			if err := kage.EncryptFile(opts.secretFile, recipients, secrets); err != nil {
				return err
			}
			log.Printf("created %s from %d secrets in %s", opts.secretFile, len(secrets), namespace)
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Kubernetes namespace (required)")
	_ = cmd.MarkFlagRequired("namespace")
	return cmd
}

func newApplyCommand(opts *commandOptions) *cobra.Command {
	var namespace string
	var confirm bool
	cmd := &cobra.Command{
		Use: "apply", Short: "Synchronize the encrypted secrets to Kubernetes", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			secrets, err := readSecrets(opts.secretFile)
			if err != nil {
				return err
			}
			clientset, err := kubernetesClient()
			if err != nil {
				return err
			}
			secretAPI := clientset.CoreV1().Secrets(namespace)
			for _, secret := range toK8s(secrets) {
				current, err := secretAPI.Get(context.Background(), secret.Name, metav1.GetOptions{})
				if err != nil && !apierrors.IsNotFound(err) {
					return fmt.Errorf("get %s: %w", secret.Name, err)
				}
				if apierrors.IsNotFound(err) {
					if !confirm {
						log.Printf("would create %s/%s", namespace, secret.Name)
						continue
					}
					if _, err = secretAPI.Create(context.Background(), secret, metav1.CreateOptions{}); err != nil {
						return fmt.Errorf("create %s: %w", secret.Name, err)
					}
					log.Printf("Created %s/%s", namespace, secret.Name)
					continue
				}
				if !secretNeedsUpdate(current, secret) {
					continue
				}
				if !confirm {
					log.Printf("would update %s/%s", namespace, secret.Name)
					continue
				}
				secret.ResourceVersion = current.ResourceVersion
				if _, err = secretAPI.Update(context.Background(), secret, metav1.UpdateOptions{}); err != nil {
					return fmt.Errorf("update %s: %w", secret.Name, err)
				}
				log.Printf("Updated %s/%s", namespace, secret.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Kubernetes namespace (required)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "perform changes instead of previewing them")
	_ = cmd.MarkFlagRequired("namespace")
	return cmd
}

func readSecrets(path string) (kage.File, error) {
	identities, err := kage.DefaultSSHIdentities()
	if err != nil {
		return nil, fmt.Errorf("need an identity: %w", err)
	}
	return kage.ReadEncryptedFile(path, identities)
}

func kubernetesClient() (*kubernetes.Clientset, error) {
	cfg, err := kubeConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}
