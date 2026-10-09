package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newShowCommand(opts *persistentOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "show SECRET/KEY",
		Short: "Print a decrypted secret value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			secretName, key, found := strings.Cut(args[0], "/")
			secretName = strings.TrimSpace(secretName)
			key = strings.TrimSpace(key)
			if !found || secretName == "" || key == "" {
				return fmt.Errorf("show argument must be secret/key")
			}
			secrets, err := readSecrets(opts.secretFile)
			if err != nil {
				return err
			}
			for _, secret := range secrets {
				if secret.Name != secretName {
					continue
				}
				for _, line := range secret.Lines {
					if line.Key == key {
						_, err := fmt.Fprintln(cmd.OutOrStdout(), line.Value)
						return err
					}
				}
				return fmt.Errorf("key %q not found in secret %q", key, secretName)
			}
			return fmt.Errorf("secret %q not found", secretName)
		},
	}
}
