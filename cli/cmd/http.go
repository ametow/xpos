package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(httpCommand)
}

var httpCommand = &cobra.Command{
	Use:   "http [port]",
	Short: "Forward http traffic",
	Long:  ``,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		tcpHttpCommand("http", args[0])
	},
}
