package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/regclient/regclient/regclient"
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
	hostname, pathPrefix string
	cacert, tls          string // set opts
	mirrors              []string
	priority             uint
	scheme               string   // TODO: remove
	dns                  []string // TODO: remove
}

func init() {
	registryLoginCmd.Flags().StringVarP(&registryOpts.user, "user", "u", "", "Username")
	registryLoginCmd.Flags().StringVarP(&registryOpts.pass, "pass", "p", "", "Password")
	registryLoginCmd.RegisterFlagCompletionFunc("user", completeArgNone)
	registryLoginCmd.RegisterFlagCompletionFunc("pass", completeArgNone)

	registrySetCmd.Flags().StringVarP(&registryOpts.cacert, "cacert", "", "", "CA Certificate (not a filename, use \"$(cat ca.pem)\" to use a file)")
	registrySetCmd.Flags().StringVarP(&registryOpts.tls, "tls", "", "", "TLS (enabled, insecure, disabled)")
	registrySetCmd.Flags().StringVarP(&registryOpts.hostname, "hostname", "", "", "Hostname or ip with port")
	registrySetCmd.Flags().StringVarP(&registryOpts.pathPrefix, "path-prefix", "", "", "Prefix to all repositories")
	registrySetCmd.Flags().StringArrayVarP(&registryOpts.mirrors, "mirror", "", nil, "List of mirrors (registry names)")
	registrySetCmd.Flags().UintVarP(&registryOpts.priority, "priority", "", 0, "Priority (for sorting mirrors)")
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
	reader := bufio.NewReader(os.Stdin)
	if len(args) < 1 || args[0] == regclient.DockerRegistry || args[0] == regclient.DockerRegistryAuth {
		args = []string{regclient.DockerRegistryDNS}
	}
	h, ok := c.Hosts[args[0]]
	if !ok {
		h = &ConfigHost{}
		c.Hosts[args[0]] = h
	}
	if registryOpts.user != "" {
		h.User = registryOpts.user
	} else {
		// prompt for username
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
	if registryOpts.pass != "" {
		h.Pass = registryOpts.pass
	} else {
		// prompt for a password
		fmt.Print("Enter Password: ")
		pass, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("Unable to read from tty (resolve by using \"-p\" flag, or winpty on Windows): %w", err)
		}
		passwd := strings.TrimSpace(string(pass))
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
	if len(args) < 1 || args[0] == regclient.DockerRegistry || args[0] == regclient.DockerRegistryAuth {
		args = []string{regclient.DockerRegistryDNS}
	}
	h, ok := c.Hosts[args[0]]
	if !ok {
		log.WithFields(logrus.Fields{
			"registry": args[0],
		}).Warn("No configuration/credentials found")
		return nil
	}
	h.User = ""
	h.Pass = ""
	h.Token = ""
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
	var name string
	if len(args) < 1 || args[0] == regclient.DockerRegistryDNS || args[0] == regclient.DockerRegistryAuth {
		name = regclient.DockerRegistry
	} else {
		name = args[0]
	}
	h, ok := c.Hosts[name]
	if !ok {
		h = ConfigHostNew()
		h.Hostname = name
		c.Hosts[name] = h
	}

	if registryOpts.scheme != "" {
		log.WithFields(logrus.Fields{
			"name":   name,
			"scheme": registryOpts.scheme,
		}).Warn("Scheme flag is deprecated, for http set tls to disabled")
	}
	if registryOpts.dns != nil {
		log.WithFields(logrus.Fields{
			"name": name,
			"dns":  registryOpts.dns,
		}).Warn("DNS flag is deprecated, use hostname and mirrors instead")
	}
	if registryOpts.tls != "" {
		if err := h.TLS.UnmarshalText([]byte(registryOpts.tls)); err != nil {
			return err
		}
	}
	if registryOpts.cacert != "" {
		h.RegCert = registryOpts.cacert
	}
	if registryOpts.hostname != "" {
		h.Hostname = registryOpts.hostname
	}
	if registryOpts.pathPrefix != "" {
		h.PathPrefix = registryOpts.pathPrefix
	}
	if len(registryOpts.mirrors) > 0 {
		h.Mirrors = registryOpts.mirrors
	}
	if registryOpts.priority != 0 {
		h.Priority = registryOpts.priority
	}

	err = c.ConfigSave()
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"name": name,
	}).Info("Registry configuration updated/set")
	return nil
}
