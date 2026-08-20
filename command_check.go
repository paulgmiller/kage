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
				if _, err := fmt.Fprintln(output, secret.Name); err != nil {
					return err
				}
				for _, line := range secret.Lines {
					if line.Key != "" {
						if _, err := fmt.Fprintf(output, "  %s=%s\n", line.Key, maskedSecretValue(line.Value)); err != nil {
							return err
						}
					}
				}
				if _, err := fmt.Fprintln(output); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
