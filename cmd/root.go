package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "gooooo [directory]", Short: "Interactively preview and organize files by type", SilenceUsage: true, Args: cobra.MaximumNArgs(1), RunE: runTUI}
	root.AddCommand(newOrganizeCmd(), newTUICmd())
	return root
}

func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
