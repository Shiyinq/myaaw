package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the application build information",
	Long:  `Print the build version, commit hash, build date, and Go runtime version.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Myaaw CLI Version: %s\n", Version)
		fmt.Printf("Git Commit:        %s\n", Commit)
		fmt.Printf("Build Date:        %s\n", Date)
		fmt.Printf("Go Version:        %s\n", runtime.Version())
		fmt.Printf("OS/Arch:           %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}
