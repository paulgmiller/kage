package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"

	"github.com/paulgmiller/kage/pkg/kage"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newCreateCommand(opts *persistentOptions) *cobra.Command {
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
