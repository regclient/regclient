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
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/ref"
)

type registryOpts struct {
	rootOpts             *rootOpts
	format               string
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

func NewRegistryCmd(rOpts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry <cmd>",
		Short: "manage registries",
		Long: fmt.Sprintf(`Retrieve or update registry configurations.
By default, the configuration is loaded from $HOME/%s/%s.
This location can be overridden with the %s environment variable.
Note that these commands do not include logins imported from Docker or values injected with --host.`, ConfigHomeDir, ConfigFilename, ConfigEnv),
	}
	cmd.AddCommand(newRegistryConfigCmd(rOpts))
	cmd.AddCommand(newRegistryLoginCmd(rOpts))
	cmd.AddCommand(newRegistryLogoutCmd(rOpts))
	cmd.AddCommand(newRegistrySetCmd(rOpts))
	cmd.AddCommand(newRegistryWhoamiCmd(rOpts))
	return cmd
}

func newRegistryConfigCmd(rOpts *rootOpts) *cobra.Command {
	opts := registryOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
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
		RunE:              opts.runRegistryConfig,
	}
	cmd.Flags().StringVar(&opts.format, "format", "{{jsonPretty .}}", "Format output with go template syntax")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	return cmd
}

func newRegistryLoginCmd(rOpts *rootOpts) *cobra.Command {
	opts := registryOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
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
		RunE:              opts.runRegistryLogin,
	}
	cmd.Flags().StringVarP(&opts.pass, "pass", "p", "", "Password")
	_ = cmd.RegisterFlagCompletionFunc("pass", completeArgNone)
	cmd.Flags().BoolVar(&opts.passStdin, "pass-stdin", false, "Read password from stdin")
	cmd.Flags().BoolVar(&opts.skipCheck, "skip-check", false, "Skip checking connectivity to the registry")
	cmd.Flags().StringVarP(&opts.user, "user", "u", "", "Username")
	_ = cmd.RegisterFlagCompletionFunc("user", completeArgNone)
	return cmd
}

func newRegistryLogoutCmd(rOpts *rootOpts) *cobra.Command {
	opts := registryOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
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
		RunE:              opts.runRegistryLogout,
	}
	return cmd
}

func newRegistrySetCmd(rOpts *rootOpts) *cobra.Command {
	opts := registryOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
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
		RunE:              opts.runRegistrySet,
	}
	cmd.Flags().StringArrayVar(&opts.apiOpts, "api-opts", nil, "List of options (key=value))")
	cmd.Flags().Int64Var(&opts.blobChunk, "blob-chunk", 0, "Blob chunk size")
	_ = cmd.RegisterFlagCompletionFunc("blob-chunk", completeArgNone)
	cmd.Flags().Int64Var(&opts.blobMax, "blob-max", 0, "Blob size before switching to chunked push, -1 to disable")
	_ = cmd.RegisterFlagCompletionFunc("blob-max", completeArgNone)
	cmd.Flags().StringVar(&opts.cacert, "cacert", "", "CA Certificate (not a filename, use \"$(cat ca.pem)\" to use a file)")
	_ = cmd.RegisterFlagCompletionFunc("cacert", completeArgNone)
	cmd.Flags().StringVar(&opts.clientCert, "client-cert", "", "Client certificate for mTLS (not a filename, use \"$(cat client.pem)\" to use a file)")
	cmd.Flags().StringVar(&opts.clientKey, "client-key", "", "Client key for mTLS (not a filename, use \"$(cat client.key)\" to use a file)")
	cmd.Flags().StringVar(&opts.credHelper, "cred-helper", "", "Credential helper (full binary name, including docker-credential- prefix)")
	cmd.Flags().StringVar(&opts.hostname, "hostname", "", "Hostname or ip with port")
	_ = cmd.RegisterFlagCompletionFunc("hostname", completeArgNone)
	cmd.Flags().StringArrayVar(&opts.mirrors, "mirror", nil, "List of mirrors (registry names)")
	_ = cmd.RegisterFlagCompletionFunc("mirror", completeArgNone)
	cmd.Flags().StringVar(&opts.pathPrefix, "path-prefix", "", "Prefix to all repositories")
	_ = cmd.RegisterFlagCompletionFunc("path-prefix", completeArgNone)
	cmd.Flags().UintVar(&opts.priority, "priority", 0, "Priority (for sorting mirrors)")
	_ = cmd.RegisterFlagCompletionFunc("priority", completeArgNone)
	cmd.Flags().BoolVar(&opts.repoAuth, "repo-auth", false, "Separate auth requests per repository instead of per registry")
	cmd.Flags().Int64Var(&opts.reqConcurrent, "req-concurrent", 0, "Concurrent requests")
	cmd.Flags().Float64Var(&opts.reqPerSec, "req-per-sec", 0, "Requests per second")
	cmd.Flags().BoolVar(&opts.skipCheck, "skip-check", false, "Skip checking connectivity to the registry")
	cmd.Flags().StringVar(&opts.tls, "tls", "", "TLS (enabled, insecure, disabled)")
	_ = cmd.RegisterFlagCompletionFunc("tls", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"enabled",
			"insecure",
			"disabled",
		}, cobra.ShellCompDirectiveNoFileComp
	})

	// TODO: eventually remove
	cmd.Flags().StringArrayVar(&opts.dns, "dns", nil, "[Deprecated] DNS hostname or ip with port")
	_ = cmd.Flags().MarkHidden("dns")
	cmd.Flags().StringVar(&opts.scheme, "scheme", "", "[Deprecated] Scheme (http, https)")
	_ = cmd.Flags().MarkHidden("scheme")
	return cmd
}

func newRegistryWhoamiCmd(rOpts *rootOpts) *cobra.Command {
	opts := registryOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:   "whoami [registry]",
		Short: "show current login for a registry",
		Long:  `Displays the username for a given registry.`,
		Example: `
# show the login on Docker Hub
regctl registry whoami

# show the login on another registry
regctl registry whoami registry.example.org`,
		Args:              cobra.RangeArgs(0, 1),
		ValidArgsFunction: registryArgListReg,
		RunE:              opts.runRegistryWhoami,
	}
	return cmd
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

func (opts *registryOpts) runRegistryConfig(cmd *cobra.Command, args []string) error {
	c, err := ConfigLoadDefault()
	if err != nil {
		return err
	}
	if len(args) > 0 {
		h, ok := c.Hosts[args[0]]
		if !ok {
			opts.rootOpts.log.Warn("No configuration found for registry",
				slog.String("registry", args[0]))
			return nil
		}
		// load the username from a credential helper
		cred := h.GetCred()
		h.User = cred.User
		// do not output secrets
		h.Pass = ""
		h.Token = ""
		h.ClientKey = ""
		return template.Writer(cmd.OutOrStdout(), opts.format, h)
	} else {
		// do not output secrets
		for i := range c.Hosts {
			c.Hosts[i].Pass = ""
			c.Hosts[i].Token = ""
			c.Hosts[i].ClientKey = ""
		}
		return template.Writer(cmd.OutOrStdout(), opts.format, c)
	}
}

func (opts *registryOpts) runRegistryLogin(cmd *cobra.Command, args []string) error {
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
	if !config.HostValidate(args[0]) {
		return fmt.Errorf("invalid registry name provided: %s", args[0])
	}
	h := &config.Host{Name: args[0]}
	if curH, ok := c.Hosts[h.Name]; ok {
		h = curH
	} else {
		c.Hosts[h.Name] = h
	}
	if flagChanged(cmd, "user") {
		h.User = opts.user
	} else if opts.passStdin {
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
			opts.rootOpts.log.Error("Username is required")

			return ErrMissingInput
		}
	}
	if flagChanged(cmd, "pass") {
		h.Pass = opts.pass
	} else if opts.passStdin {
		pass, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("failed to read password from stdin: %w", err)
		}
		passwd := strings.TrimRight(string(pass), "\n")
		if passwd != "" {
			h.Pass = passwd
		} else {
			opts.rootOpts.log.Error("Password is required")

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
			opts.rootOpts.log.Error("Password is required")

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
	if !opts.skipCheck {
		r, err := ref.NewHost(args[0])
		if err != nil {
			return err
		}
		rc := opts.rootOpts.newRegClient()
		_, err = rc.Ping(ctx, r)
		if err != nil {
			opts.rootOpts.log.Warn("Failed to ping registry, credentials were still stored")

			return err
		}
	}
	opts.rootOpts.log.Info("Credentials set",
		slog.String("registry", args[0]))
	return nil
}

func (opts *registryOpts) runRegistryLogout(cmd *cobra.Command, args []string) error {
	c, err := ConfigLoadDefault()
	if err != nil {
		return err
	}
	if len(args) < 1 {
		args = []string{regclient.DockerRegistry}
	}
	if !config.HostValidate(args[0]) {
		return fmt.Errorf("invalid registry name provided: %s", args[0])
	}
	h, ok := c.Hosts[args[0]]
	if !ok {
		opts.rootOpts.log.Warn("No configuration/credentials found",
			slog.String("registry", h.Name))
		return nil
	}
	h.User = ""
	h.Pass = ""
	h.Token = ""
	if h.IsZero() {
		delete(c.Hosts, h.Name)
	}
	// TODO: add credHelper calls to erase a password
	err = c.ConfigSave()
	if err != nil {
		return err
	}

	opts.rootOpts.log.Debug("Credentials unset",
		slog.String("registry", args[0]))
	return nil
}

func (opts *registryOpts) runRegistrySet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	c, err := ConfigLoadDefault()
	if err != nil {
		return err
	}
	if len(args) < 1 {
		args = []string{regclient.DockerRegistry}
	}
	if !config.HostValidate(args[0]) {
		return fmt.Errorf("invalid registry name provided: %s", args[0])
	}
	h := config.HostNewName(args[0])
	if curH, ok := c.Hosts[h.Name]; ok {
		h = curH
	} else {
		c.Hosts[h.Name] = h
	}
	if flagChanged(cmd, "scheme") {
		opts.rootOpts.log.Warn("Scheme flag is deprecated, for http set tls to disabled",
			slog.String("name", h.Name),
			slog.String("scheme", opts.scheme))
	}
	if flagChanged(cmd, "dns") {
		opts.rootOpts.log.Warn("DNS flag is deprecated, use hostname and mirrors instead",
			slog.String("name", h.Name),
			slog.Any("dns", opts.dns))
	}
	if flagChanged(cmd, "cred-helper") {
		h.CredHelper = opts.credHelper
	}
	if flagChanged(cmd, "tls") {
		if err := h.TLS.UnmarshalText([]byte(opts.tls)); err != nil {
			return err
		}
	}
	if flagChanged(cmd, "cacert") {
		h.RegCert = opts.cacert
	}
	if flagChanged(cmd, "client-cert") {
		h.ClientCert = opts.clientCert
	}
	if flagChanged(cmd, "client-key") {
		h.ClientKey = opts.clientKey
	}
	if flagChanged(cmd, "hostname") {
		h.Hostname = opts.hostname
	}
	if flagChanged(cmd, "path-prefix") {
		h.PathPrefix = opts.pathPrefix
	}
	if flagChanged(cmd, "mirror") {
		h.Mirrors = opts.mirrors
	}
	if flagChanged(cmd, "priority") {
		h.Priority = opts.priority
	}
	if flagChanged(cmd, "repo-auth") {
		h.RepoAuth = opts.repoAuth
	}
	if flagChanged(cmd, "blob-chunk") {
		h.BlobChunk = opts.blobChunk
	}
	if flagChanged(cmd, "blob-max") {
		h.BlobMax = opts.blobMax
	}
	if flagChanged(cmd, "req-per-sec") {
		h.ReqPerSec = opts.reqPerSec
	}
	if flagChanged(cmd, "req-concurrent") {
		h.ReqConcurrent = opts.reqConcurrent
	}
	if flagChanged(cmd, "api-opts") {
		if h.APIOpts == nil {
			h.APIOpts = map[string]string{}
		}
		for _, kv := range opts.apiOpts {
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
	if h.IsZero() {
		delete(c.Hosts, h.Name)
	}

	err = c.ConfigSave()
	if err != nil {
		return err
	}

	if !opts.skipCheck {
		r, err := ref.NewHost(args[0])
		if err != nil {
			return err
		}
		rc := opts.rootOpts.newRegClient()
		_, err = rc.Ping(ctx, r)
		if err != nil {
			opts.rootOpts.log.Warn("Failed to ping registry, configuration still updated")

			return err
		}
	}

	opts.rootOpts.log.Info("Registry configuration updated/set",
		slog.String("name", h.Name))
	return nil
}

func (opts *registryOpts) runRegistryWhoami(cmd *cobra.Command, args []string) error {
	c, err := ConfigLoadDefault()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		args = []string{regclient.DockerRegistry}
	}
	h, ok := c.Hosts[args[0]]
	if !ok {
		return fmt.Errorf("no login found for %s%.0w", args[0], errs.ErrNoLogin)
	}
	cred := h.GetCred()
	if cred.User == "" && cred.Token != "" {
		cred.User = "<token>"
	}
	if cred.User == "" {
		return fmt.Errorf("no login found for %s%.0w", args[0], errs.ErrNoLogin)
	}
	// output the user
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", cred.User)
	return err
}
