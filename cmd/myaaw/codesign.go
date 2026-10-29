package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

	"myaaw/internal/cli/theme"
	"myaaw/internal/codesign"

	"github.com/spf13/cobra"
)

var codesignCmd = &cobra.Command{
	Use:   "codesign",
	Short: "Manage the local macOS code-signing identity",
	Long: "macOS only. Create or inspect the local \"Myaaw Agent\" certificate. " +
		"A stable code identity keeps macOS permissions (Screen Recording, Accessibility, " +
		"Automation) valid across rebuilds and updates. On other platforms every " +
		"subcommand is a no-op.",
}

var codesignSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Create the local \"Myaaw Agent\" signing certificate",
	Run: func(cmd *cobra.Command, args []string) {
		if runtime.GOOS != "darwin" {
			fmt.Println(theme.RenderMuted("codesign is macOS-only; nothing to do."))
			return
		}

		fmt.Println(theme.RenderPrimary("🔐 Local code-signing identity"))
		res, err := codesign.Setup("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
		if res.Reason == "already exists" {
			fmt.Println(theme.RenderSuccess("✅ Identity \"" + res.Identity + "\" already exists."))
		} else {
			fmt.Println(theme.RenderSuccess("✅ Identity \"" + res.Identity + "\" created."))
		}
		fmt.Println(theme.RenderMuted("If a keychain dialog appeared, choose \"Always Allow\"."))
		fmt.Println(theme.RenderMuted("install.sh and `myaaw update` re-sign automatically from now on."))
	},
}

var codesignStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the signing identity and the installed binary's signature",
	Run: func(cmd *cobra.Command, args []string) {
		if runtime.GOOS != "darwin" {
			fmt.Println(theme.RenderMuted("codesign is macOS-only; nothing to inspect."))
			return
		}

		fmt.Println(theme.RenderPrimary("🔐 Code-signing status"))
		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

		if codesign.IdentityExists("") {
			fmt.Printf("  %-14s %s\n", "Identity", theme.RenderSuccess(codesign.DefaultIdentity+" (found)"))
		} else {
			fmt.Printf("  %-14s %s\n", "Identity", theme.RenderError(codesign.DefaultIdentity+" (missing)"))
			fmt.Printf("  %-14s %s\n", "", theme.RenderMuted("Run: myaaw codesign setup"))
		}

		exe, err := os.Executable()
		if err != nil {
			return
		}
		authority, identifier, adhoc := codesign.SignatureInfo(exe)
		fmt.Printf("  %-14s %s\n", "Binary", exe)
		if adhoc {
			fmt.Printf("  %-14s %s\n", "Signature", theme.RenderError("ad-hoc (unsigned) — permissions reset on update"))
		} else {
			fmt.Printf("  %-14s %s\n", "Signature", theme.RenderSuccess("signed"))
			if identifier != "" {
				fmt.Printf("  %-14s %s\n", "Identifier", identifier)
			}
			if authority != "" {
				fmt.Printf("  %-14s %s\n", "Authority", authority)
			}
		}
	},
}

var codesignRemoveYes bool

var codesignRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove the local \"Myaaw Agent\" signing certificate",
	Long: "Delete the local signing identity and its code-signing trust settings. " +
		"Existing binaries stay signed; future builds and updates will not be signed " +
		"automatically until `myaaw codesign setup` is run again.",
	Run: func(cmd *cobra.Command, args []string) {
		if runtime.GOOS != "darwin" {
			fmt.Println(theme.RenderMuted("codesign is macOS-only; nothing to do."))
			return
		}

		identity := strings.TrimSpace(os.Getenv("MYAAW_CODESIGN_IDENTITY"))
		if identity == "" {
			identity = codesign.DefaultIdentity
		}
		if !codesign.IdentityExists(identity) {
			fmt.Println(theme.RenderMuted("Identity \"" + identity + "\" is not installed; nothing to remove."))
			return
		}

		if !codesignRemoveYes {
			fmt.Printf("Remove identity %q and its trust settings? [y/N] ", identity)
			answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			switch strings.ToLower(strings.TrimSpace(answer)) {
			case "y", "yes":
			default:
				fmt.Println("Cancelled.")
				return
			}
		}

		res, err := codesign.Remove(identity)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
		fmt.Println(theme.RenderSuccess("✅ Identity \"" + res.Identity + "\" removed."))
		fmt.Println(theme.RenderMuted("Existing binaries stay signed. Re-sign ad-hoc with:"))
		fmt.Println(theme.RenderMuted("  cp \"$(command -v myaaw)\" /tmp/myaaw && codesign --force --sign - /tmp/myaaw && mv /tmp/myaaw \"$(command -v myaaw)\""))
	},
}

func init() {
	codesignCmd.AddCommand(codesignSetupCmd)
	codesignCmd.AddCommand(codesignStatusCmd)
	codesignCmd.AddCommand(codesignRemoveCmd)
	codesignRemoveCmd.Flags().BoolVarP(&codesignRemoveYes, "yes", "y", false, "Skip the confirmation prompt")
}
