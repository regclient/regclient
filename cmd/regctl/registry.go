package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var registryCmd = &cobra.Command{
	Use:   "registry <cmd>",
	Short: "manage registries",
}
var registryConfigCmd = &cobra.Command{
	Use:   "config [registry]",
	Short: "show registry config",
	Long: `Displays the configuration used for a registry. Passwords are not included
in the output.`,
	Args:              cobra.RangeArgs(0, 1),
	ValidArgsFunction: registryArgListReg,
	RunE:              runRegistryConfig,
}
var registryLoginCmd = &cobra.Command{
	Use:   "login <registry>",
	Short: "login to a registry",
	Long: `Provide login credentials for a registry. This may not be necessary if you
have already logged in with docker.`,
	Args:              cobra.RangeArgs(0, 1),
	ValidArgsFunction: registryArgListReg,
	RunE:              runRegistryLogin,
}
var registryLogoutCmd = &cobra.Command{
	Use:               "logout <registry>",
	Short:             "logout of a registry",
	Long:              `Remove registry credentials from the configuration.`,
	Args:              cobra.RangeArgs(0, 1),
	ValidArgsFunction: registryArgListReg,
	RunE:              runRegistryLogout,
}
var registrySetCmd = &cobra.Command{
	Use:   "set <registry>",
	Short: "set options on a registry",
	Long: `Set or modify the configuration of a registry. To pass a certificate, include
the contents of the file, e.g. --cacert "$(cat reg-ca.crt)"`,
	Args:              cobra.RangeArgs(0, 1),
	ValidArgsFunction: registryArgListReg,
	RunE:              runRegistrySet,
}
var registryOpts struct {
	user, pass           string // login opts
	passStdin            bool
	credHelper           string
	hostname, pathPrefix string
	cacert, tls          string // set opts
	mirrors              []string
	priority             uint
	repoAuth             bool
	blobChunk, blobMax   int64
	reqPerSec            float64
	reqConcurrent        int64
	apiOpts              []string
	scheme               string   // TODO: remove
	dns                  []string // TODO: remove
}

func init() {
	registryLoginCmd.Flags().StringVarP(&registryOpts.user, "user", "u", "", "Username")
	registryLoginCmd.Flags().StringVarP(&registryOpts.pass, "pass", "p", "", "Password")
	registryLoginCmd.Flags().BoolVarP(&registryOpts.passStdin, "pass-stdin", "", false, "Read password from stdin")
	registryLoginCmd.RegisterFlagCompletionFunc("user", completeArgNone)
	registryLoginCmd.RegisterFlagCompletionFunc("pass", completeArgNone)

	registrySetCmd.Flags().StringVarP(&registryOpts.credHelper, "cred-helper", "", "", "Credential helper (full binary name, including docker-credential- prefix)")
	registrySetCmd.Flags().StringVarP(&registryOpts.cacert, "cacert", "", "", "CA Certificate (not a filename, use \"$(cat ca.pem)\" to use a file)")
	registrySetCmd.Flags().StringVarP(&registryOpts.tls, "tls", "", "", "TLS (enabled, insecure, disabled)")
	registrySetCmd.Flags().StringVarP(&registryOpts.hostname, "hostname", "", "", "Hostname or ip with port")
	registrySetCmd.Flags().StringVarP(&registryOpts.pathPrefix, "path-prefix", "", "", "Prefix to all repositories")
	registrySetCmd.Flags().StringArrayVarP(&registryOpts.mirrors, "mirror", "", nil, "List of mirrors (registry names)")
	registrySetCmd.Flags().UintVarP(&registryOpts.priority, "priority", "", 0, "Priority (for sorting mirrors)")
	registrySetCmd.Flags().BoolVarP(&registryOpts.repoAuth, "repo-auth", "", false, "Separate auth requests per repository instead of per registry")
	registrySetCmd.Flags().Int64VarP(&registryOpts.blobChunk, "blob-chunk", "", 0, "Blob chunk size")
	registrySetCmd.Flags().Int64VarP(&registryOpts.blobMax, "blob-max", "", 0, "Blob size before switching to chunked push, -1 to disable")
	registrySetCmd.Flags().Float64VarP(&registryOpts.reqPerSec, "req-per-sec", "", 0, "Requests per second")
	registrySetCmd.Flags().Int64VarP(&registryOpts.reqConcurrent, "req-concurrent", "", 0, "Concurrent requests")
	registrySetCmd.Flags().StringArrayVarP(&registryOpts.apiOpts, "api-opts", "", nil, "List of options (key=value))")
	registrySetCmd.RegisterFlagCompletionFunc("cacert", completeArgNone)
	registrySetCmd.RegisterFlagCompletionFunc("tls", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"enabled",
			"insecure",
			"disabled",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	registrySetCmd.RegisterFlagCompletionFunc("hostname", completeArgNone)
	registrySetCmd.RegisterFlagCompletionFunc("path-prefix", completeArgNone)
	registrySetCmd.RegisterFlagCompletionFunc("mirror", completeArgNone)
	registrySetCmd.RegisterFlagCompletionFunc("priority", completeArgNone)
	registrySetCmd.RegisterFlagCompletionFunc("blob-chunk", completeArgNone)
	registrySetCmd.RegisterFlagCompletionFunc("blob-max", completeArgNone)

	// TODO: eventually remove
	registrySetCmd.Flags().StringVarP(&registryOpts.scheme, "scheme", "", "", "[Deprecated] Scheme (http, https)")
	registrySetCmd.Flags().StringArrayVarP(&registryOpts.dns, "dns", "", nil, "[Deprecated] DNS hostname or ip with port")
	registrySetCmd.Flags().MarkHidden("scheme")
	registrySetCmd.Flags().MarkHidden("dns")

	registryCmd.AddCommand(registryConfigCmd)
	registryCmd.AddCommand(registryLoginCmd)
	registryCmd.AddCommand(registryLogoutCmd)
	registryCmd.AddCommand(registrySetCmd)
	rootCmd.AddCommand(registryCmd)
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

func runRegistryConfig(cmd *cobra.Command, args []string) error {
	c, err := ConfigLoadDefault()
	if err != nil {
		return err
	}
	// empty out the password fields, do not print them
	for i := range c.Hosts {
		c.Hosts[i].Pass = ""
		c.Hosts[i].Token = ""
	}
	var hj []byte
	if len(args) > 0 {
		h, ok := c.Hosts[args[0]]
		if !ok {
			log.WithFields(logrus.Fields{
				"registry": args[0],
			}).Warn("No configuration found for registry")
			return nil
		}
		hj, err = json.MarshalIndent(h, "", "  ")
		if err != nil {
			return err
		}
	} else {
		hj, err = json.MarshalIndent(c.Hosts, "", "  ")
		if err != nil {
			return err
		}
	}

	fmt.Println(string(hj))
	return nil
}

func runRegistryLogin(cmd *cobra.Command, args []string) error {
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
		reader := bufio.NewReader(os.Stdin)
		defUser := ""
		if h.User != "" {
			defUser = " [" + h.User + "]"
		}
		fmt.Printf("Enter Username%s: ", defUser)
		user, _ := reader.ReadString('\n')
		user = strings.TrimSpace(user)
		if user != "" {
			h.User = user
		} else if h.User == "" {
			log.Error("Username is required")
			return ErrMissingInput
		}
	}
	if flagChanged(cmd, "pass") {
		h.Pass = registryOpts.pass
	} else if registryOpts.passStdin {
		pass, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read password from stdin: %w", err)
		}
		passwd := strings.TrimRight(string(pass), "\n")
		if passwd != "" {
			h.Pass = passwd
		} else {
			log.Error("Password is required")
			return ErrMissingInput
		}
	} else {
		// prompt for a password
		fmt.Print("Enter Password: ")
		pass, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("unable to read from tty (resolve by using \"-p\" flag, or winpty on Windows): %w", err)
		}
		passwd := strings.TrimRight(string(pass), "\n")
		fmt.Print("\n")
		if passwd != "" {
			h.Pass = passwd
		} else {
			log.Error("Password is required")
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

	log.WithFields(logrus.Fields{
		"registry": args[0],
	}).Info("Credentials set")
	return nil
}

func runRegistryLogout(cmd *cobra.Command, args []string) error {
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
		log.WithFields(logrus.Fields{
			"registry": h.Name,
		}).Warn("No configuration/credentials found")
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

	log.WithFields(logrus.Fields{
		"registry": args[0],
	}).Debug("Credentials unset")
	return nil
}

func runRegistrySet(cmd *cobra.Command, args []string) error {
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
		log.WithFields(logrus.Fields{
			"name":   h.Name,
			"scheme": registryOpts.scheme,
		}).Warn("Scheme flag is deprecated, for http set tls to disabled")
	}
	if flagChanged(cmd, "dns") {
		log.WithFields(logrus.Fields{
			"name": h.Name,
			"dns":  registryOpts.dns,
		}).Warn("DNS flag is deprecated, use hostname and mirrors instead")
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

	log.WithFields(logrus.Fields{
		"name": h.Name,
	}).Info("Registry configuration updated/set")
	return nil
}
