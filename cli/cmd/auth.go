package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/ametow/xpos/cli/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(authCommand)
}

var authCommand = &cobra.Command{
	Use:   "auth [token]",
	Short: "Sets auth token in config",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		token := args[0]

		config := config.Config{
			Local: struct {
				AuthToken string `yaml:"auth_token"`
			}{token},
		}
		if err := config.Write(); err != nil {
			log.Fatalf("error writing config: %s", err)
		}
		fmt.Println("auth token is set")
		os.Exit(0)
	},
}
