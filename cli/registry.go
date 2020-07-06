package cli

import (
	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "manage registries",
}
var registryConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "show registry config",
	Args:  cobra.RangeArgs(1, 1),
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
var registryTLSCmd = &cobra.Command{
	Use:   "tls",
	Short: "set TLS options on a registry",
	Args:  cobra.RangeArgs(2, 2),
	RunE:  runRegistryTLS,
}

func init() {
	registryCmd.AddCommand(registryConfigCmd)
	registryCmd.AddCommand(registryLoginCmd)
	registryCmd.AddCommand(registryLogoutCmd)
	registryCmd.AddCommand(registryTLSCmd)
	rootCmd.AddCommand(registryCmd)
}

func runRegistryConfig(cmd *cobra.Command, args []string) error {
	return nil
}

func runRegistryLogin(cmd *cobra.Command, args []string) error {
	return nil
}

func runRegistryLogout(cmd *cobra.Command, args []string) error {
	return nil
}

func runRegistryTLS(cmd *cobra.Command, args []string) error {
	return nil
}
