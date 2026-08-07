package cmd

import (
	"github.com/spf13/cobra"
)

var httpDebug bool

func init() {
	rootCmd.AddCommand(httpCommand)
	httpCommand.Flags().BoolVar(&httpDebug, "debug", false, "enable local debug proxy")
}

var httpCommand = &cobra.Command{
	Use:   "http [port]",
	Short: "Forward http traffic",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		tcpHttpCommand("http", args[0], httpDebug)
	},
}
