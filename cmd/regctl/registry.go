package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types/ref"
)

type registryCmd struct {
	rootOpts             *rootCmd
	formatConf           string
	user, pass           string // login opts
	passStdin            bool
	credHelper           string
	hostname, pathPrefix string
	cacert, tls          string // set opts
	clientCert           string
	clientKey            string
	mirrors              []string
	priority             uint
	repoAuth             bool
	blobChunk, blobMax   int64
	reqPerSec            float64
	reqConcurrent        int64
	skipCheck            bool
	apiOpts              []string
	scheme               string   // TODO: remove
	dns                  []string // TODO: remove
}

func NewRegistryCmd(rootOpts *rootCmd) *cobra.Command {
	registryOpts := registryCmd{
		rootOpts: rootOpts,
	}
	var registryTopCmd = &cobra.Command{
		Use:   "registry <cmd>",
		Short: "manage registries",
	}
	var registryConfigCmd = &cobra.Command{
		Use:   "config [registry]",
		Short: "show registry config",
		Long: `Displays the configuration used for a registry. Secrets are not included
in the output (e.g. passwords, tokens, and TLS keys).`,
		Example: `
# show the full config
regctl registry config

# show the configuration for a single registry
regctl registry config registry.example.org

# show the configuration for Docker Hub
regctl registry config docker.io

# show the username used to login to docker hub
regctl registry config docker.io --format '{{.User}}'`,
		Args:              cobra.RangeArgs(0, 1),
		ValidArgsFunction: registryArgListReg,
		RunE:              registryOpts.runRegistryConfig,
	}
	var registryLoginCmd = &cobra.Command{
		Use:   "login <registry>",
		Short: "login to a registry",
		Long: `Provide login credentials for a registry. This may not be necessary if you
have already logged in with docker.`,
		Example: `
# login to Docker Hub
regctl registry login

# login to registry
regctl registry login registry.example.org

# login to GHCR with a provided password
echo "${token}" | regctl registry login ghcr.io -u "${username}" --pass-stdin`,
		Args:              cobra.RangeArgs(0, 1),
		ValidArgsFunction: registryArgListReg,
		RunE:              registryOpts.runRegistryLogin,
	}
	var registryLogoutCmd = &cobra.Command{
		Use:   "logout <registry>",
		Short: "logout of a registry",
		Long:  `Remove registry credentials from the configuration.`,
		Example: `
# logout from Docker Hub
regctl registry logout

# logout from a specific registry
regctl registry logout registry.example.org`,
		Args:              cobra.RangeArgs(0, 1),
		ValidArgsFunction: registryArgListReg,
		RunE:              registryOpts.runRegistryLogout,
	}
	var registrySetCmd = &cobra.Command{
		Use:   "set <registry>",
		Short: "set options on a registry",
		Long:  `Set or modify the configuration of a registry.`,
		Example: `
# configure a registry for HTTP
regctl registry set localhost:5000 --tls disabled

# configure a self signed certificate
regctl registry set registry.example.org --cacert "$(cat reg-ca.crt)"

# specify a local mirror for Docker Hub
regctl registry set docker.io --mirror hub-mirror.example.org

# specify the requests per sec throttle
regctl registry set quay.io --req-per-sec 10`,
		Args:              cobra.RangeArgs(0, 1),
		ValidArgsFunction: registryArgListReg,
		RunE:              registryOpts.runRegistrySet,
	}

	registryConfigCmd.Flags().StringVar(&registryOpts.formatConf, "format", "{{jsonPretty .}}", "Format output with go template syntax")

	registryLoginCmd.Flags().StringVarP(&registryOpts.user, "user", "u", "", "Username")
	registryLoginCmd.Flags().StringVarP(&registryOpts.pass, "pass", "p", "", "Password")
	registryLoginCmd.Flags().BoolVar(&registryOpts.passStdin, "pass-stdin", false, "Read password from stdin")
	registryLoginCmd.Flags().BoolVar(&registryOpts.skipCheck, "skip-check", false, "Skip checking connectivity to the registry")
	_ = registryLoginCmd.RegisterFlagCompletionFunc("user", completeArgNone)
	_ = registryLoginCmd.RegisterFlagCompletionFunc("pass", completeArgNone)

	registrySetCmd.Flags().StringVar(&registryOpts.credHelper, "cred-helper", "", "Credential helper (full binary name, including docker-credential- prefix)")
	registrySetCmd.Flags().StringVar(&registryOpts.cacert, "cacert", "", "CA Certificate (not a filename, use \"$(cat ca.pem)\" to use a file)")
	registrySetCmd.Flags().StringVar(&registryOpts.clientCert, "client-cert", "", "Client certificate for mTLS (not a filename, use \"$(cat client.pem)\" to use a file)")
	registrySetCmd.Flags().StringVar(&registryOpts.clientKey, "client-key", "", "Client key for mTLS (not a filename, use \"$(cat client.key)\" to use a file)")
	registrySetCmd.Flags().StringVar(&registryOpts.tls, "tls", "", "TLS (enabled, insecure, disabled)")
	registrySetCmd.Flags().StringVar(&registryOpts.hostname, "hostname", "", "Hostname or ip with port")
	registrySetCmd.Flags().StringVar(&registryOpts.pathPrefix, "path-prefix", "", "Prefix to all repositories")
	registrySetCmd.Flags().StringArrayVar(&registryOpts.mirrors, "mirror", nil, "List of mirrors (registry names)")
	registrySetCmd.Flags().UintVar(&registryOpts.priority, "priority", 0, "Priority (for sorting mirrors)")
	registrySetCmd.Flags().BoolVar(&registryOpts.repoAuth, "repo-auth", false, "Separate auth requests per repository instead of per registry")
	registrySetCmd.Flags().Int64Var(&registryOpts.blobChunk, "blob-chunk", 0, "Blob chunk size")
	registrySetCmd.Flags().Int64Var(&registryOpts.blobMax, "blob-max", 0, "Blob size before switching to chunked push, -1 to disable")
	registrySetCmd.Flags().Float64Var(&registryOpts.reqPerSec, "req-per-sec", 0, "Requests per second")
	registrySetCmd.Flags().Int64Var(&registryOpts.reqConcurrent, "req-concurrent", 0, "Concurrent requests")
	registrySetCmd.Flags().BoolVar(&registryOpts.skipCheck, "skip-check", false, "Skip checking connectivity to the registry")
	registrySetCmd.Flags().StringArrayVar(&registryOpts.apiOpts, "api-opts", nil, "List of options (key=value))")
	_ = registrySetCmd.RegisterFlagCompletionFunc("cacert", completeArgNone)
	_ = registrySetCmd.RegisterFlagCompletionFunc("tls", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"enabled",
			"insecure",
			"disabled",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = registrySetCmd.RegisterFlagCompletionFunc("hostname", completeArgNone)
	_ = registrySetCmd.RegisterFlagCompletionFunc("path-prefix", completeArgNone)
	_ = registrySetCmd.RegisterFlagCompletionFunc("mirror", completeArgNone)
	_ = registrySetCmd.RegisterFlagCompletionFunc("priority", completeArgNone)
	_ = registrySetCmd.RegisterFlagCompletionFunc("blob-chunk", completeArgNone)
	_ = registrySetCmd.RegisterFlagCompletionFunc("blob-max", completeArgNone)

	// TODO: eventually remove
	registrySetCmd.Flags().StringVar(&registryOpts.scheme, "scheme", "", "[Deprecated] Scheme (http, https)")
	registrySetCmd.Flags().StringArrayVar(&registryOpts.dns, "dns", nil, "[Deprecated] DNS hostname or ip with port")
	_ = registrySetCmd.Flags().MarkHidden("scheme")
	_ = registrySetCmd.Flags().MarkHidden("dns")

	registryTopCmd.AddCommand(registryConfigCmd)
	registryTopCmd.AddCommand(registryLoginCmd)
	registryTopCmd.AddCommand(registryLogoutCmd)
	registryTopCmd.AddCommand(registrySetCmd)
	return registryTopCmd
}

func registryArgListReg(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	result := []string{}
	c, err := ConfigLoadDefault()
	if err != nil {
		return result, cobra.ShellCompDirectiveNoFileComp
	}
	for host := range c.Hosts {
		if strings.HasPrefix(host, toComplete) {
			result = append(result, host)
		}
	}
	return result, cobra.ShellCompDirectiveNoFileComp
}

func (registryOpts *registryCmd) runRegistryConfig(cmd *cobra.Command, args []string) error {
	c, err := ConfigLoadDefault()
	if err != nil {
		return err
	}
	// empty out the password fields, do not print them
	for i := range c.Hosts {
		c.Hosts[i].Pass = ""
		c.Hosts[i].Token = ""
		c.Hosts[i].ClientKey = ""
	}
	if len(args) > 0 {
		h, ok := c.Hosts[args[0]]
		if !ok {
			registryOpts.rootOpts.log.Warn("No configuration found for registry",
				slog.String("registry", args[0]))
			return nil
		}
		return template.Writer(cmd.OutOrStdout(), registryOpts.formatConf, h)
	} else {
		return template.Writer(cmd.OutOrStdout(), registryOpts.formatConf, c)
	}
}

func (registryOpts *registryCmd) runRegistryLogin(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	// disable signal handler to allow ctrl-c to be used on prompts (context cancel on a blocking reader is difficult)
	signal.Reset(os.Interrupt, syscall.SIGTERM)
	c, err := ConfigLoadDefault()
	if err != nil {
		return err
	}
	if len(args) < 1 {
		args = []string{regclient.DockerRegistry}
	}
	h := config.HostNewName(args[0])
	if curH, ok := c.Hosts[h.Name]; ok {
		h = curH
	} else {
		c.Hosts[h.Name] = h
	}
	if flagChanged(cmd, "user") {
		h.User = registryOpts.user
	} else if registryOpts.passStdin {
		return fmt.Errorf("user must be provided to read password from stdin")
	} else {
		// prompt for username
		reader := bufio.NewReader(cmd.InOrStdin())
		defUser := ""
		if h.User != "" {
			defUser = " [" + h.User + "]"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Enter Username%s: ", defUser)
		user, _ := reader.ReadString('\n')
		user = strings.TrimSpace(user)
		if user != "" {
			h.User = user
		} else if h.User == "" {
			registryOpts.rootOpts.log.Error("Username is required")

			return ErrMissingInput
		}
	}
	if flagChanged(cmd, "pass") {
		h.Pass = registryOpts.pass
	} else if registryOpts.passStdin {
		pass, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("failed to read password from stdin: %w", err)
		}
		passwd := strings.TrimRight(string(pass), "\n")
		if passwd != "" {
			h.Pass = passwd
		} else {
			registryOpts.rootOpts.log.Error("Password is required")

			return ErrMissingInput
		}
	} else {
		// prompt for a password
		var fd int
		if ifd, ok := cmd.InOrStdin().(interface{ Fd() uintptr }); ok {
			fd = int(ifd.Fd())
		} else {
			return fmt.Errorf("file descriptor needed to prompt for password (resolve by using \"-p\" flag)")
		}
		fmt.Fprint(cmd.OutOrStdout(), "Enter Password: ")
		pass, err := term.ReadPassword(fd)
		if err != nil {
			return fmt.Errorf("unable to read from tty (resolve by using \"-p\" flag, or winpty on Windows): %w", err)
		}
		passwd := strings.TrimRight(string(pass), "\n")
		fmt.Fprint(cmd.OutOrStdout(), "\n")
		if passwd != "" {
			h.Pass = passwd
		} else {
			registryOpts.rootOpts.log.Error("Password is required")

			return ErrMissingInput
		}
	}
	// if username is <token> then process password as an identity token
	if h.User == "<token>" {
		h.Token = h.Pass
		h.User = ""
		h.Pass = ""
	} else {
		h.Token = ""
	}
	err = c.ConfigSave()
	if err != nil {
		return err
	}
	if !registryOpts.skipCheck {
		r, err := ref.NewHost(args[0])
		if err != nil {
			return err
		}
		rc := registryOpts.rootOpts.newRegClient()
		_, err = rc.Ping(ctx, r)
		if err != nil {
			registryOpts.rootOpts.log.Warn("Failed to ping registry, credentials were still stored")

			return err
		}
	}
	registryOpts.rootOpts.log.Info("Credentials set",
		slog.String("registry", args[0]))
	return nil
}

func (registryOpts *registryCmd) runRegistryLogout(cmd *cobra.Command, args []string) error {
	c, err := ConfigLoadDefault()
	if err != nil {
		return err
	}
	if len(args) < 1 {
		args = []string{regclient.DockerRegistry}
	}
	h := config.HostNewName(args[0])
	if curH, ok := c.Hosts[h.Name]; ok {
		h = curH
	} else {
		registryOpts.rootOpts.log.Warn("No configuration/credentials found",
			slog.String("registry", h.Name))
		return nil
	}
	h.User = ""
	h.Pass = ""
	h.Token = ""
	// TODO: add credHelper calls to erase a password
	err = c.ConfigSave()
	if err != nil {
		return err
	}

	registryOpts.rootOpts.log.Debug("Credentials unset",
		slog.String("registry", args[0]))
	return nil
}

func (registryOpts *registryCmd) runRegistrySet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	c, err := ConfigLoadDefault()
	if err != nil {
		return err
	}
	if len(args) < 1 {
		args = []string{regclient.DockerRegistry}
	}
	h := config.HostNewName(args[0])
	if curH, ok := c.Hosts[h.Name]; ok {
		h = curH
	} else {
		c.Hosts[h.Name] = h
	}
	if flagChanged(cmd, "scheme") {
		registryOpts.rootOpts.log.Warn("Scheme flag is deprecated, for http set tls to disabled",
			slog.String("name", h.Name),
			slog.String("scheme", registryOpts.scheme))
	}
	if flagChanged(cmd, "dns") {
		registryOpts.rootOpts.log.Warn("DNS flag is deprecated, use hostname and mirrors instead",
			slog.String("name", h.Name),
			slog.Any("dns", registryOpts.dns))
	}
	if flagChanged(cmd, "cred-helper") {
		h.CredHelper = registryOpts.credHelper
	}
	if flagChanged(cmd, "tls") {
		if err := h.TLS.UnmarshalText([]byte(registryOpts.tls)); err != nil {
			return err
		}
	}
	if flagChanged(cmd, "cacert") {
		h.RegCert = registryOpts.cacert
	}
	if flagChanged(cmd, "client-cert") {
		h.ClientCert = registryOpts.clientCert
	}
	if flagChanged(cmd, "client-key") {
		h.ClientKey = registryOpts.clientKey
	}
	if flagChanged(cmd, "hostname") {
		h.Hostname = registryOpts.hostname
	}
	if flagChanged(cmd, "path-prefix") {
		h.PathPrefix = registryOpts.pathPrefix
	}
	if flagChanged(cmd, "mirror") {
		h.Mirrors = registryOpts.mirrors
	}
	if flagChanged(cmd, "priority") {
		h.Priority = registryOpts.priority
	}
	if flagChanged(cmd, "repo-auth") {
		h.RepoAuth = registryOpts.repoAuth
	}
	if flagChanged(cmd, "blob-chunk") {
		h.BlobChunk = registryOpts.blobChunk
	}
	if flagChanged(cmd, "blob-max") {
		h.BlobMax = registryOpts.blobMax
	}
	if flagChanged(cmd, "req-per-sec") {
		h.ReqPerSec = registryOpts.reqPerSec
	}
	if flagChanged(cmd, "req-concurrent") {
		h.ReqConcurrent = registryOpts.reqConcurrent
	}
	if flagChanged(cmd, "api-opts") {
		if h.APIOpts == nil {
			h.APIOpts = map[string]string{}
		}
		for _, kv := range registryOpts.apiOpts {
			kvArr := strings.SplitN(kv, "=", 2)
			if len(kvArr) == 2 && kvArr[1] != "" {
				// set a value
				h.APIOpts[kvArr[0]] = kvArr[1]
			} else if h.APIOpts[kvArr[0]] != "" {
				// unset a value by not giving the key a value
				delete(h.APIOpts, kvArr[0])
			}
		}
	}

	err = c.ConfigSave()
	if err != nil {
		return err
	}

	if !registryOpts.skipCheck {
		r, err := ref.NewHost(args[0])
		if err != nil {
			return err
		}
		rc := registryOpts.rootOpts.newRegClient()
		_, err = rc.Ping(ctx, r)
		if err != nil {
			registryOpts.rootOpts.log.Warn("Failed to ping registry, configuration still updated")

			return err
		}
	}

	registryOpts.rootOpts.log.Info("Registry configuration updated/set",
		slog.String("name", h.Name))
	return nil
}
