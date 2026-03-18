package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:          "oscar-export",
	Short:        "Export OSCAR cache data to CSV",
	SilenceUsage: true,
}

func Execute() error {
	return rootCmd.Execute()
}
