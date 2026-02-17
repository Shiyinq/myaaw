package main

import (
	"fmt"
	"runtime"

	"myaaw/internal/cli/theme"

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
		k := theme.HighlightStyle.Width(18).Render
		v := theme.BaseStyle.Render

		fmt.Printf("%s %s\n", k("Myaaw CLI Version:"), v(Version))
		fmt.Printf("%s %s\n", k("Git Commit:"), v(Commit))
		fmt.Printf("%s %s\n", k("Build Date:"), v(Date))
		fmt.Printf("%s %s\n", k("Go Version:"), v(runtime.Version()))
		fmt.Printf("%s %s/%s\n", k("OS/Arch:"), v(runtime.GOOS), v(runtime.GOARCH))
	},
}
