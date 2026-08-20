package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCheckCommand(opts *persistentOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Show secret names and masked values",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			secrets, err := readSecrets(opts.secretFile)
			if err != nil {
				return err
			}
			output := cmd.OutOrStdout()
			for _, secret := range secrets {
				fmt.Fprintln(output, secret.Name)
				for _, line := range secret.Lines {
					if line.Key != "" {
						fmt.Fprintf(output, "  %s=%s\n", line.Key, maskedSecretValue(line.Value))
					}
				}
				fmt.Fprintln(output)
			}
			return nil
		},
	}
}
