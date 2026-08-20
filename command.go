package main

import (
	"fmt"

	"github.com/paulgmiller/kage/pkg/kage"
	"github.com/spf13/cobra"
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
}

func newRootCommand() *cobra.Command {
	opts := &commandOptions{}
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
