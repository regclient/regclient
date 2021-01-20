package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
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
	Args: cobra.RangeArgs(0, 1),
	RunE: runRegistryConfig,
}
var registryLoginCmd = &cobra.Command{
	Use:   "login <registry>",
	Short: "login to a registry",
	Long: `Provide login credentials for a registry. This may not be necessary if you
have already logged in with docker.`,
	Args: cobra.RangeArgs(0, 1),
	RunE: runRegistryLogin,
}
var registryLogoutCmd = &cobra.Command{
	Use:   "logout <registry>",
	Short: "logout of a registry",
	Long:  `Remove registry credentials from the configuration.`,
	Args:  cobra.RangeArgs(0, 1),
	RunE:  runRegistryLogout,
}
var registrySetCmd = &cobra.Command{
	Use:   "set <registry>",
	Short: "set options on a registry",
	Long: `Set or modify the configuration of a registry. To pass a certificate, include
the contents of the file, e.g. --cacert "$(cat reg-ca.crt)"`,
	Args: cobra.RangeArgs(0, 1),
	RunE: runRegistrySet,
}
var registryOpts struct {
	user, pass          string // login opts
	cacert, scheme, tls string // set opts
	dns                 []string
}

func init() {
	registryLoginCmd.Flags().StringVarP(&registryOpts.user, "user", "u", "", "Username")
	registryLoginCmd.Flags().StringVarP(&registryOpts.pass, "pass", "p", "", "Password")

	registrySetCmd.Flags().StringVarP(&registryOpts.scheme, "scheme", "", "", "Scheme (http, https)")
	registrySetCmd.Flags().StringArrayVarP(&registryOpts.dns, "dns", "", nil, "DNS hostname or ip with port")
	registrySetCmd.Flags().StringVarP(&registryOpts.tls, "tls", "", "", "TLS (enabled, insecure, disabled)")
	registrySetCmd.Flags().StringVarP(&registryOpts.cacert, "cacert", "", "", "CA Certificate")

	registryCmd.AddCommand(registryConfigCmd)
	registryCmd.AddCommand(registryLoginCmd)
	registryCmd.AddCommand(registryLogoutCmd)
	registryCmd.AddCommand(registrySetCmd)
	rootCmd.AddCommand(registryCmd)
}

func runRegistryConfig(cmd *cobra.Command, args []string) error {
	c, err := regclient.ConfigLoadDefault()
	if err != nil {
		return err
	}
	// empty out the password fields, do not print them
	for i := range c.Hosts {
		c.Hosts[i].Pass = ""
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
	c, err := regclient.ConfigLoadDefault()
	if err != nil {
		return err
	}
	reader := bufio.NewReader(os.Stdin)
	if len(args) < 1 || args[0] == regclient.DockerRegistry || args[0] == regclient.DockerRegistryAuth {
		args = []string{regclient.DockerRegistryDNS}
	}
	h, ok := c.Hosts[args[0]]
	if !ok {
		h = &regclient.ConfigHost{}
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
		pass, err := terminal.ReadPassword(0)
		if err != nil {
			return err
		}
		passwd := strings.TrimSpace(string(pass))
		if passwd != "" {
			h.Pass = passwd
		} else {
			log.Error("Password is required")
			return ErrMissingInput
		}
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
	c, err := regclient.ConfigLoadDefault()
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
	c, err := regclient.ConfigLoadDefault()
	if err != nil {
		return err
	}
	if len(args) < 1 || args[0] == regclient.DockerRegistry || args[0] == regclient.DockerRegistryAuth {
		args = []string{regclient.DockerRegistryDNS}
	}
	h, ok := c.Hosts[args[0]]
	if !ok {
		h = regclient.ConfigHostNew()
		h.DNS = []string{args[0]}
		c.Hosts[args[0]] = h
	}

	if registryOpts.scheme != "" {
		h.Scheme = registryOpts.scheme
	}
	if registryOpts.dns != nil {
		h.DNS = registryOpts.dns
	}
	if registryOpts.tls != "" {
		if err := h.TLS.UnmarshalText([]byte(registryOpts.tls)); err != nil {
			return err
		}
	}
	if registryOpts.cacert != "" {
		h.RegCert = registryOpts.cacert
	}

	err = c.ConfigSave()
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"registry": args[0],
	}).Info("Registry configuration updated/set")
	return nil
}
