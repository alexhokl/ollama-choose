package cli

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ollama-choose",
	Short: "Interactively choose an Ollama model",
}

// Execute runs the root command. Exit codes: 0 on success, 1 for a false
// judgment (see ErrJudgedFalse), 2 for any other error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if errors.Is(err, ErrJudgedFalse) {
			os.Exit(1)
		}
		os.Exit(2)
	}
}
