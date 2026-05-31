package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(tcpCommand)
}

var tcpCommand = &cobra.Command{
	Use:   "tcp [port]",
	Short: "Forward tcp traffic",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		tcpHttpCommand("tcp", args[0])
	},
}
