package cmd

import (
	"github.com/spf13/cobra"
)

var tcpDebug bool

func init() {
	rootCmd.AddCommand(tcpCommand)
	tcpCommand.Flags().BoolVar(&tcpDebug, "debug", false, "enable local debug proxy")
}

var tcpCommand = &cobra.Command{
	Use:   "tcp [port]",
	Short: "Forward tcp traffic",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		tcpHttpCommand("tcp", args[0], tcpDebug)
	},
}
