package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	agentruntime "myaaw/internal/agent/runtime"
	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/services/controlplane/service"

	"github.com/spf13/cobra"
)

var (
	runtimeListJSON      bool
	runtimeListCheckAuth bool
)

var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Inspect available native and external agent runtimes",
	Long:  "List built-in and external CLI agent runtimes detected on this host using the same discovery and auth checks as the WebUI settings page.",
}

var runtimeListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List available agent runtimes and host installation status",
	Long:    "Detect installed external CLI runtimes on this host (version, binary path, global defaults, and agents using each runtime). Pass --check-auth to also probe CLI login readiness.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRuntimeList(cmd.Context(), runtimeListCheckAuth, runtimeListJSON)
	},
}

type runtimeListJSONPayload struct {
	Native            agentruntime.DetectedRuntime `json:"native"`
	ManagementEnabled bool                         `json:"management_enabled"`
	Runtimes          []service.ManagedRuntimeView `json:"runtimes"`
}

func runRuntimeList(ctx context.Context, checkAuth, asJSON bool) error {
	var cpdb *service.ControlPlaneDB
	if err := agentdir.EnsureLayout(); err == nil {
		if opened, dbErr := service.OpenControlPlaneDB(); dbErr == nil {
			cpdb = opened
			defer cpdb.Close()
		}
	}

	// Reuse the exact same RuntimeManageService that powers GET /runtimes/manage
	// in WebUI Settings so binary detection, global defaults, agent usage, and
	// auth probing stay in a single source of truth.
	mgr := service.NewRuntimeManageService(cpdb, service.NewAgentService())
	managed, err := mgr.List(ctx, checkAuth, false)
	if err != nil {
		return err
	}

	nativePreset, _ := config.LookupRuntimePreset(config.RuntimeTypeMyaaw)
	nativeEntry := agentruntime.DetectedRuntime{
		RuntimePreset: nativePreset,
		Installed:     true,
		Version:       "built-in",
	}

	if asJSON {
		payload := runtimeListJSONPayload{
			Native:            nativeEntry,
			ManagementEnabled: managed.ManagementEnabled,
			Runtimes:          managed.Runtimes,
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Println(theme.RenderPrimary("Available Agent Runtimes"))
	fmt.Println()
	fmt.Printf("  %-18s %-26s %s\n",
		nativeEntry.ID,
		nativeEntry.Label,
		theme.RenderSuccess("built-in"),
	)

	installedCount := 0
	countable := 0
	for _, rt := range managed.Runtimes {
		if rt.DefaultCommand != "" {
			countable++
			if rt.Installed {
				installedCount++
			}
		}

		status := theme.RenderMuted("not installed")
		if rt.ID == "custom_acp" && !rt.Installed {
			status = theme.RenderMuted("custom binary")
		} else if rt.Installed {
			if rt.Version != "" {
				status = theme.RenderSuccess("installed · " + rt.Version)
			} else {
				status = theme.RenderSuccess("installed")
			}
		}

		extras := []string{}
		if checkAuth && (rt.Installed || rt.Defaults.APIKey.Configured) {
			if authText := formatCLIAuthStatus(rt.Auth); authText != "" {
				extras = append(extras, authText)
			}
		} else if !checkAuth && rt.Defaults.APIKey.Configured {
			extras = append(extras, theme.RenderSecondary("api key ready"))
		}
		if n := len(rt.UsedByAgents); n > 0 {
			extras = append(extras, theme.RenderMuted(fmt.Sprintf("%d agent(s): %s", n, strings.Join(rt.UsedByAgents, ", "))))
		}

		suffix := ""
		if len(extras) > 0 {
			suffix = "  " + strings.Join(extras, " · ")
		}
		fmt.Printf("  %-18s %-26s %s%s\n", rt.ID, rt.Label, status, suffix)

		if rt.BinaryPath != "" {
			fmt.Printf("  %-18s %s\n", "", theme.RenderMuted(shortenCLIHomePath(rt.BinaryPath)))
		} else if preview := recommendedCLIInstallPreview(rt.InstallMethods); preview != "" {
			fmt.Printf("  %-18s %s\n", "", theme.RenderMuted("install: "+preview))
		}
	}

	fmt.Println()
	fmt.Println(theme.RenderMuted(fmt.Sprintf("Detected %d/%d external runtimes installed on this host.", installedCount, countable)))
	fmt.Println(theme.RenderMuted("Use with: myaaw onboard --yes --agent <name> --runtime <preset>"))
	if !checkAuth {
		fmt.Println(theme.RenderMuted("Tip: pass --check-auth to also probe CLI login readiness, or --json for machine-readable output."))
	}
	return nil
}

func formatCLIAuthStatus(auth agentruntime.AuthStatusInfo) string {
	detail := ""
	if strings.TrimSpace(auth.Detail) != "" {
		detail = " (" + strings.TrimSpace(auth.Detail) + ")"
	}
	switch auth.Status {
	case agentruntime.AuthStatusAuthenticated:
		return theme.RenderSuccess("signed in" + detail)
	case agentruntime.AuthStatusFreeReady:
		return theme.RenderSuccess("free tier ready" + detail)
	case agentruntime.AuthStatusAPIKeyConfigured:
		return theme.RenderSecondary("api key ready" + detail)
	case agentruntime.AuthStatusUnauthenticated:
		return theme.RenderWarning("not signed in")
	default:
		return ""
	}
}

func recommendedCLIInstallPreview(methods []agentruntime.InstallMethod) string {
	for _, m := range methods {
		if m.Available && !m.ManualDownload && m.Recommended {
			return m.CommandPreview
		}
	}
	for _, m := range methods {
		if m.Available && m.Recommended {
			return m.CommandPreview
		}
	}
	if len(methods) > 0 {
		return methods[0].CommandPreview
	}
	return ""
}

func shortenCLIHomePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}

func init() {
	runtimeListCmd.Flags().BoolVar(&runtimeListJSON, "json", false, "print machine-readable JSON output")
	runtimeListCmd.Flags().BoolVar(&runtimeListCheckAuth, "check-auth", false, "also probe installed CLIs for login/auth readiness")
	runtimeCmd.AddCommand(runtimeListCmd)
	rootCmd.AddCommand(runtimeCmd)
}
