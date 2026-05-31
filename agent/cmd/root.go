package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:               "xpos",
		Short:             "xpos - tunneling service that links your localhost to the internet",
		Long:              `XPOS is a free and open-source tool that allows local servers to be accessible on the public internet. It supports TCP protocols like HTTP, SSH, and more.`,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
