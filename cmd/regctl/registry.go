package main

import (
	"encoding/json"
	"fmt"

	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "manage registries",
}
var registryConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "show registry config",
	Args:  cobra.RangeArgs(0, 1),
	RunE:  runRegistryConfig,
}
var registryLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "login to a registry",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runRegistryLogin,
}
var registryLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "logout of a registry",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runRegistryLogout,
}
var registrySetCmd = &cobra.Command{
	Use:   "set",
	Short: "set options on a registry",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runRegistrySet,
}
var registryOpts struct {
	user, pass  string // login opts
	scheme, tls string // set opts
	dns         []string
}

func init() {
	registryLoginCmd.Flags().StringVarP(&registryOpts.user, "user", "u", "", "Username")
	registryLoginCmd.Flags().StringVarP(&registryOpts.pass, "pass", "p", "", "Password")
	registryLoginCmd.MarkFlagRequired("user")

	registrySetCmd.Flags().StringVarP(&registryOpts.scheme, "scheme", "", "", "Scheme (http, https)")
	registrySetCmd.Flags().StringArrayVarP(&registryOpts.dns, "dns", "", nil, "DNS hostname or ip with port")
	registrySetCmd.Flags().StringVarP(&registryOpts.tls, "tls", "", "", "TLS (enabled, insecure, disabled)")

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
	// prompt for password if not provided on cli
	if registryOpts.pass == "" {
		return ErrNotImplemented
	}
	c, err := regclient.ConfigLoadDefault()
	if err != nil {
		return err
	}
	h, ok := c.Hosts[args[0]]
	if !ok {
		h = &regclient.ConfigHost{}
		c.Hosts[args[0]] = h
	}
	h.User = registryOpts.user
	h.Pass = registryOpts.pass
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

	err = c.ConfigSave()
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"registry": args[0],
	}).Info("Registry configuration updated/set")
	return nil
}
