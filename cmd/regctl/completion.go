package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
	"github.com/spf13/cobra"
)

func NewCompletionCmd(rootOpts *rootCmd) *cobra.Command {
	var completionTopCmd = &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate completion script",
		Long: fmt.Sprintf(`To load completions:

Bash:

  $ source <(%[1]s completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ %[1]s completion bash > /etc/bash_completion.d/%[1]s
  # macOS:
  $ %[1]s completion bash > $(brew --prefix)/etc/bash_completion.d/%[1]s

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ %[1]s completion zsh > "${fpath[1]}/_%[1]s"

  # You will need to start a new shell for this setup to take effect.

fish:

  $ %[1]s completion fish | source

  # To load completions for each session, execute once:
  $ %[1]s completion fish > ~/.config/fish/completions/%[1]s.fish

PowerShell:

  PS> %[1]s completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> %[1]s completion powershell > %[1]s.ps1
  # and source this file from your PowerShell profile.
`, rootOpts.name),
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Run: func(cmd *cobra.Command, args []string) {
			switch args[0] {
			case "bash":
				_ = cmd.Root().GenBashCompletionV2(os.Stdout, true)
			case "zsh":
				_ = cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				_ = cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				_ = cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
		},
	}

	return completionTopCmd
}

type completeFunc func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)

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
		types.MediaTypeDocker2Manifest,
		types.MediaTypeDocker2ManifestList,
		types.MediaTypeOCI1Manifest,
		types.MediaTypeOCI1ManifestList,
		types.MediaTypeDocker1Manifest,
		types.MediaTypeDocker1ManifestSigned,
	}, cobra.ShellCompDirectiveNoFileComp
}

func (rootOpts *rootCmd) completeArgTag(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	result := []string{}
	// TODO: is it possible to expand registry, then repo, then tag?
	input := strings.TrimRight(toComplete, ":")
	r, err := ref.New(input)
	if err != nil || r.Digest != "" {
		return result, cobra.ShellCompDirectiveNoFileComp
	}
	rc := rootOpts.newRegClient()
	tl, err := rc.TagList(context.Background(), r)
	if err != nil {
		return result, cobra.ShellCompDirectiveNoFileComp
	}
	tags, err := tl.GetTags()
	if err != nil {
		return result, cobra.ShellCompDirectiveNoFileComp
	}
	for _, tag := range tags {
		resultRef, _ := ref.New(input)
		resultRef.Tag = tag
		resultCN := resultRef.CommonName()
		if strings.HasPrefix(resultCN, toComplete) {
			result = append(result, resultCN)
		}
	}
	return result, cobra.ShellCompDirectiveNoFileComp
}
