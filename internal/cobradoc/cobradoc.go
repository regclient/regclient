// Package cobradoc is used to generate documentation from cobra commands.
package cobradoc

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/regclient/regclient/pkg/template"
)

// List outputs a list of cobra commands.
func List(cmd *cobra.Command, hidden bool, out io.Writer) {
	var recurse func(cmd *cobra.Command, out io.Writer)
	recurse = func(cmd *cobra.Command, out io.Writer) {
		fmt.Fprintf(out, "%s\n", cmd.CommandPath())
		for _, child := range cmd.Commands() {
			if !hidden && child.Hidden {
				continue
			}
			recurse(child, out)
		}
	}
	recurse(cmd, out)
}

// Markdown outputs docs on a cobra command in Markdown format.
func Markdown(cmd *cobra.Command, out io.Writer) error {
	cmd.InitDefaultHelpCmd()
	cmd.InitDefaultHelpFlag()

	buf := new(bytes.Buffer)
	name := cmd.CommandPath()

	buf.WriteString("## " + name + "\n\n")
	buf.WriteString(cmd.Short + "\n\n")
	if len(cmd.Long) > 0 {
		buf.WriteString("### Synopsis\n\n")
		buf.WriteString(strings.TrimSpace(cmd.Long) + "\n\n")
	}
	if cmd.Runnable() {
		buf.WriteString(fmt.Sprintf("```\n%s\n```\n\n", cmd.UseLine()))
	}
	if len(cmd.Example) > 0 {
		buf.WriteString("### Examples\n\n")
		buf.WriteString(fmt.Sprintf("```\n%s\n```\n\n", strings.TrimSpace(cmd.Example)))
	}
	if err := printOptions(buf, cmd); err != nil {
		return err
	}
	_, err := buf.WriteTo(out)
	return err
}

func printOptions(buf *bytes.Buffer, cmd *cobra.Command) error {
	flags := cmd.NonInheritedFlags()
	flags.SetOutput(buf)
	if flags.HasAvailableFlags() {
		buf.WriteString("### Options\n\n```\n")
		flags.PrintDefaults()
		buf.WriteString("```\n\n")
	}

	parentFlags := cmd.InheritedFlags()
	parentFlags.SetOutput(buf)
	if parentFlags.HasAvailableFlags() {
		buf.WriteString("### Options inherited from parent commands\n\n```\n")
		parentFlags.PrintDefaults()
		buf.WriteString("```\n\n")
	}
	return nil
}

type docOpts struct {
	format string
	hidden bool
	list   bool
}

// NewCmd generates a new cobra command for generating docs.
func NewCmd(rootName, newName string) *cobra.Command {
	opts := docOpts{}
	var docCmd = &cobra.Command{
		Hidden: true,
		Use:    newName,
		Short:  "Document CLI",
		Long:   `Document Cobra CLI`,
		Example: fmt.Sprintf(`
# list all commands
%[1]s %[2]s --list

# output documentation for the "version" command
%[1]s %[2]s version`, rootName, newName),
		Args: cobra.ArbitraryArgs,
		RunE: opts.runCLIDoc,
	}
	docCmd.Flags().BoolVar(&opts.hidden, "hidden", false, "Include hidden commands in the list")
	docCmd.Flags().BoolVar(&opts.list, "list", false, "List all commands")
	docCmd.Flags().StringVar(&opts.format, "format", "", "Format output with go template syntax")

	return docCmd
}

func (opts *docOpts) runCLIDoc(cmd *cobra.Command, args []string) error {
	if opts.list {
		List(cmd.Parent(), opts.hidden, cmd.OutOrStdout())
		return nil
	}
	docCmd, _, err := cmd.Parent().Find(args)
	if err != nil {
		return err
	}
	if opts.format != "" {
		return template.Writer(cmd.OutOrStdout(), opts.format, docCmd)
	}
	return Markdown(docCmd, cmd.OutOrStdout())
}
