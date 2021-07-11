package main

import (
	"context"
	"os"
	"strings"

	"github.com/regclient/regclient/regclient"
	"github.com/regclient/regclient/regclient/types"
	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Long: `To load completions:

Bash:

  $ source <(regctl completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ regctl completion bash > /etc/bash_completion.d/regctl
  # macOS:
  $ regctl completion bash > /usr/local/etc/bash_completion.d/regctl

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ regctl completion zsh > "${fpath[1]}/_regctl"

  # You will need to start a new shell for this setup to take effect.

fish:

  $ regctl completion fish | source

  # To load completions for each session, execute once:
  $ regctl completion fish > ~/.config/fish/completions/regctl.fish

PowerShell:

  PS> regctl completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> regctl completion powershell > regctl.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.ExactValidArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			cmd.Root().GenBashCompletionV2(os.Stdout, true)
		case "zsh":
			cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}

type completeFunc func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)

func completeArgNone(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func completeArgDefault(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveDefault
}

func completeArgPlatform(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{
		"linux/amd64", "linux/386",
		"linux/arm/v6", "linux/arm/v7", "linux/arm64/v8",
		"linux/mips64le", "linux/ppc64le", "linux/s390x",
		"windows/amd64/10.0.17763.1577", "windows/amd64/10.0.14393.4046",
	}, cobra.ShellCompDirectiveNoFileComp
}

func completeArgMediaTypeManifest(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{
		regclient.MediaTypeDocker2Manifest,
		regclient.MediaTypeDocker2ManifestList,
		regclient.MediaTypeOCI1Manifest,
		regclient.MediaTypeOCI1ManifestList,
		regclient.MediaTypeDocker1Manifest,
		regclient.MediaTypeDocker1ManifestSigned,
	}, cobra.ShellCompDirectiveNoFileComp
}

// completeArgList takes a list of completion functions and completes each arg separately
func completeArgList(funcList []completeFunc) completeFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		pos := len(args)
		if pos >= len(funcList) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return funcList[pos](cmd, args, toComplete)
	}
}

func completeArgTag(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	result := []string{}
	// TODO: is it possible to expand registry, then repo, then tag?
	input := strings.TrimRight(toComplete, ":")
	ref, err := types.NewRef(input)
	if err != nil || ref.Digest != "" {
		return result, cobra.ShellCompDirectiveNoFileComp
	}
	rc := newRegClient()
	tl, err := rc.TagList(context.Background(), ref)
	if err != nil {
		return result, cobra.ShellCompDirectiveNoFileComp
	}
	tags, err := tl.GetTags()
	if err != nil {
		return result, cobra.ShellCompDirectiveNoFileComp
	}
	for _, tag := range tags {
		resultRef, _ := types.NewRef(input)
		resultRef.Tag = tag
		resultCN := resultRef.CommonName()
		if strings.HasPrefix(resultCN, toComplete) {
			result = append(result, resultCN)
		}
	}
	return result, cobra.ShellCompDirectiveNoFileComp
}
