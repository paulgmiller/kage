package main

import (
	"log"
	"path/filepath"

	"github.com/paulgmiller/kage/pkg/kage"
	"github.com/spf13/cobra"
)

func newReencryptCommand(opts *persistentOptions) *cobra.Command {
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
