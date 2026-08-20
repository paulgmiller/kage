package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

const (
	managedByAnnotationKey   = "managed-by"
	managedByAnnotationValue = "github.com/paulgmiller/kage"
	recipientsFilename       = "recipients.txt"
	defaultSecretFile        = "secrets/envtest"
)

type commandOptions struct {
	secretFile string
	input      io.Reader
	output     io.Writer
	errOutput  io.Writer
}

func run(args []string, input io.Reader, output, errOutput io.Writer) error {
	opts := &commandOptions{
		secretFile: defaultSecretFile,
		input:      input,
		output:     output,
		errOutput:  errOutput,
	}

	flags := newFlagSet("kage", rootUsage(output))
	addSecretFileFlags(flags, &opts.secretFile)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	args = flags.Args()
	if len(args) == 0 {
		rootUsage(output)()
		return nil
	}
	if args[0] == "help" {
		return runHelp(args[1:], opts)
	}

	switch args[0] {
	case "apply":
		return runApply(args[1:], opts)
	case "check":
		return runCheck(args[1:], opts)
	case "create":
		return runCreate(args[1:], opts)
	case "reencrypt":
		return runReencrypt(args[1:], opts)
	case "set":
		return runSet(args[1:], opts)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runHelp(args []string, opts *commandOptions) error {
	if len(args) == 0 {
		rootUsage(opts.output)()
		return nil
	}
	if len(args) > 1 {
		return errors.New("help accepts at most one command")
	}

	switch args[0] {
	case "apply":
		applyUsage(opts.output)()
	case "check":
		checkUsage(opts.output)()
	case "create":
		createUsage(opts.output)()
	case "reencrypt":
		reencryptUsage(opts.output)()
	case "set":
		setUsage(opts.output)()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	return nil
}

func newFlagSet(name string, usage func()) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = usage
	return flags
}

func addSecretFileFlags(flags *flag.FlagSet, secretFile *string) {
	current := *secretFile
	flags.StringVar(secretFile, "secret-file", current, "encrypted secret file")
	flags.StringVar(secretFile, "f", current, "encrypted secret file")
}

func parseSubcommandFlags(
	name string,
	args []string,
	opts *commandOptions,
	usage func(),
	configure func(*flag.FlagSet),
) (*flag.FlagSet, error) {
	flags := newFlagSet(name, usage)
	addSecretFileFlags(flags, &opts.secretFile)
	if configure != nil {
		configure(flags)
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, nil
		}
		return nil, err
	}
	return flags, nil
}

func rootUsage(output io.Writer) func() {
	return func() {
		fmt.Fprint(output, `Manage age-encrypted Kubernetes secrets

Usage:
  kage [options] <command> [command options]

Commands:
  apply       Synchronize the encrypted secrets to Kubernetes
  check       Show secret names and masked values
  create      Create an encrypted file from a Kubernetes namespace
  reencrypt   Re-encrypt the file using its recipients.txt
  set         Add or update an encrypted secret value

Options:
  -f, --secret-file FILE   encrypted secret file (default "secrets/envtest")
  -h, --help               show help

Use "kage help <command>" for more information about a command.
`)
	}
}
