package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/paulgmiller/kage/pkg/kage"
	"github.com/spf13/cobra"
)

func newMergeCommand(opts *persistentOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "merge BASE OURS THEIRS",
		Short: "Three-way merge encrypted files into --secret-file",
		Long:  "Decrypt and merge a common ancestor and two branch versions. Conflicting entries fail with secret/key names, without printing values. The output uses the recipients.txt beside --secret-file and is replaced only after successful encryption.",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			identities, err := kage.DefaultSSHIdentities()
			if err != nil {
				return fmt.Errorf("load merge identity: %w", err)
			}
			var inputs [3]kage.File
			for i, path := range args {
				inputs[i], err = kage.ReadEncryptedFile(path, identities)
				if err != nil {
					// Parser errors can contain plaintext. Do not include them here.
					return fmt.Errorf("cannot decrypt or parse merge input %q", path)
				}
			}
			merged, err := kage.Merge(inputs[0], inputs[1], inputs[2])
			if err != nil {
				return err
			}
			recipients, err := kage.LoadRecipients(filepath.Join(filepath.Dir(opts.secretFile), recipientsFilename))
			if err != nil {
				return err
			}
			// Only ciphertext reaches disk. Rename within the destination directory
			// so an input can also be the output without being truncated on failure.
			temp, err := os.CreateTemp(filepath.Dir(opts.secretFile), ".kage-merge-*")
			if err != nil {
				return fmt.Errorf("create merge output: %w", err)
			}
			defer os.Remove(temp.Name())
			if err := temp.Close(); err != nil {
				return fmt.Errorf("close merge output: %w", err)
			}
			if err := kage.EncryptFile(temp.Name(), recipients, merged); err != nil {
				return err
			}
			if err := os.Rename(temp.Name(), opts.secretFile); err != nil {
				return fmt.Errorf("replace merge output: %w", err)
			}
			return nil
		},
	}
}
