package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/paulgmiller/kage/pkg/kage"
	"github.com/spf13/cobra"
)

func newSetCommand(opts *persistentOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "set SECRET/KEY=VALUE",
		Short: "Add or update an encrypted secret value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			recipients, err = promptForCurrentIdentity(
				cmd.InOrStdin(),
				cmd.ErrOrStderr(),
				recipientsPath,
				recipients,
				current,
				currentLine,
			)
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
