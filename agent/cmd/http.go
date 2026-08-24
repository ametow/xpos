package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(httpCommand)
	httpCommand.Flags().StringP("subdomain", "s", "", "Custom subdomain for the tunnel")
}

var httpCommand = &cobra.Command{
	Use:   "http [port]",
	Short: "Forward http traffic",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		subdomain, err := cmd.Flags().GetString("subdomain")
		if err != nil {
			fmt.Println("error reading subdomain:", err)
			return
		}
		tcpHttpCommand("http", args[0], subdomain)
	},
}
