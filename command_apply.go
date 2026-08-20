package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newApplyCommand(opts *persistentOptions) *cobra.Command {
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
