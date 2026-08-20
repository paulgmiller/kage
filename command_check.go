package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

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
