package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"myaaw/internal/codesign"
	"myaaw/internal/config"

	"github.com/spf13/cobra"
)

var computerCmd = &cobra.Command{
	Use:   "computer",
	Short: "Enable or disable desktop automation (macOS only)",
	Long: "Toggle the built-in computer tool for an agent.\n\n" +
		"The built-in tool is macOS-only. On Linux and Windows it is registered " +
		"but every action returns COMPUTER_UNSUPPORTED_PLATFORM; use an external " +
		"MCP driver for desktop automation there.\n\n" +
		"Two gates must allow the tool: bot.computer_use.enabled in config.json " +
		"and builtin_tools.computer in tools.json (absent means enabled).",
}

var computerEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable the built-in computer tool",
	Run: func(cmd *cobra.Command, args []string) {
		agent, path, changed, err := setComputerUseForActiveAgent(true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
		if runtime.GOOS != "darwin" {
			fmt.Println(theme.RenderWarning("⚠️  The built-in computer tool is macOS-only (" + runtime.GOOS + " detected)."))
			fmt.Println(theme.RenderMuted("Every action will return COMPUTER_UNSUPPORTED_PLATFORM; use an MCP driver instead."))
		}
		if changed {
			fmt.Println(theme.RenderSuccess("✅ Computer use enabled for agent \"" + agent + "\"."))
		} else {
			fmt.Println(theme.RenderMuted("Computer use is already enabled for agent \"" + agent + "\"."))
		}
		fmt.Println(theme.RenderMuted("Config: " + path))
		if runtime.GOOS == "darwin" {
			ensureCodeSigning()
			printPermissionGuide()
		}
		printRestartHint()
	},
}

var computerDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable the built-in computer tool",
	Run: func(cmd *cobra.Command, args []string) {
		agent, path, changed, err := setComputerUseForActiveAgent(false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
		if changed {
			fmt.Println(theme.RenderSuccess("✅ Computer use disabled for agent \"" + agent + "\"."))
		} else {
			fmt.Println(theme.RenderMuted("Computer use is already disabled for agent \"" + agent + "\"."))
		}
		fmt.Println(theme.RenderMuted("Config: " + path))
		printRestartHint()
	},
}

var computerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the computer tool gates for the active agent",
	Run: func(cmd *cobra.Command, args []string) {
		agent, err := agentdir.ResolveActive(agentFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
		configPath, err := agentdir.ConfigPath(agent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}

		enabled, err := readComputerUseEnabled(configPath)
		if err != nil {
			fmt.Printf("  %-16s %s\n", "Feature gate", theme.RenderError("unreadable config: "+err.Error()))
		} else if enabled {
			fmt.Printf("  %-16s %s\n", "Feature gate", theme.RenderSuccess("ON (bot.computer_use.enabled)"))
		} else {
			fmt.Printf("  %-16s %s\n", "Feature gate", theme.RenderWarning("OFF (bot.computer_use.enabled)"))
		}

		registryEnabled, registryState := toolsJSONGate(agent)
		if registryEnabled {
			fmt.Printf("  %-16s %s\n", "Registry gate", theme.RenderSuccess(registryState))
		} else {
			fmt.Printf("  %-16s %s\n", "Registry gate", theme.RenderError(registryState))
		}

		if runtime.GOOS == "darwin" {
			fmt.Printf("  %-16s %s\n", "Platform", theme.RenderSuccess("macOS (native backend)"))
			if codesign.IdentityExists("") {
				fmt.Printf("  %-16s %s\n", "Code-signing", theme.RenderSuccess("Myaaw Agent (found)"))
			} else {
				fmt.Printf("  %-16s %s\n", "Code-signing", theme.RenderWarning("missing — run: myaaw codesign setup"))
			}
		} else {
			fmt.Printf("  %-16s %s\n", "Platform", theme.RenderWarning(runtime.GOOS+" — built-in tool is macOS-only"))
			fmt.Printf("  %-16s %s\n", "", theme.RenderMuted("Actions return COMPUTER_UNSUPPORTED_PLATFORM; use an MCP driver."))
		}

		effective := enabled && registryEnabled && runtime.GOOS == "darwin"
		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
		if effective {
			fmt.Printf("  %-16s %s\n", "Effective", theme.RenderSuccess("available"))
		} else {
			fmt.Printf("  %-16s %s\n", "Effective", theme.RenderMuted("not available"))
		}
		fmt.Printf("  %-16s %s\n", "Agent", agent)
		fmt.Printf("  %-16s %s\n", "Config", configPath)
	},
}

func init() {
	computerCmd.AddCommand(computerEnableCmd)
	computerCmd.AddCommand(computerDisableCmd)
	computerCmd.AddCommand(computerStatusCmd)
}

// setComputerUseForActiveAgent flips bot.computer_use.enabled for the resolved
// agent and returns whether the stored value changed.
func setComputerUseForActiveAgent(enabled bool) (agent, path string, changed bool, err error) {
	agent, err = agentdir.ResolveActive(agentFlag)
	if err != nil {
		return "", "", false, err
	}
	path, err = agentdir.ConfigPath(agent)
	if err != nil {
		return agent, "", false, err
	}
	changed, err = setComputerUseInConfig(path, enabled)
	if err != nil {
		return agent, path, false, err
	}
	return agent, path, changed, nil
}

// setComputerUseInConfig rewrites config.json with the new flag, keeping the
// other sections intact. The write is atomic so a crash cannot truncate it.
func setComputerUseInConfig(path string, enabled bool) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false, fmt.Errorf("invalid config %s: %w", path, err)
	}
	before := cfg.Bot.ComputerUse != nil && cfg.Bot.ComputerUse.Enabled
	if cfg.Bot.ComputerUse == nil {
		cfg.Bot.ComputerUse = &config.ComputerUseConfig{}
	}
	cfg.Bot.ComputerUse.Enabled = enabled
	out, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return false, err
	}
	if err := writeFileAtomicCLI(path, append(out, '\n')); err != nil {
		return false, err
	}
	return before != enabled, nil
}

func readComputerUseEnabled(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false, err
	}
	return cfg.Bot.ComputerUse != nil && cfg.Bot.ComputerUse.Enabled, nil
}

// toolsJSONGate reports whether tools.json permits the computer tool. A missing
// key means enabled (see isBuiltinEnabled in the tools package).
func toolsJSONGate(agent string) (bool, string) {
	dir, err := agentdir.ToolsDir(agent)
	if err != nil {
		return true, "unknown (no tools dir)"
	}
	data, err := os.ReadFile(filepath.Join(dir, "tools.json"))
	if err != nil {
		return true, "enabled (no tools.json)"
	}
	var parsed struct {
		BuiltinTools map[string]bool `json:"builtin_tools"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return true, "unknown (invalid tools.json)"
	}
	if v, ok := parsed.BuiltinTools["computer"]; ok {
		if v {
			return true, "enabled in tools.json"
		}
		return false, "disabled in tools.json"
	}
	return true, "enabled (implicit)"
}

func printRestartHint() {
	fmt.Println(theme.RenderMuted("Restart the agent processes so the new tool list is used:"))
	fmt.Println(theme.RenderMuted("  myaaw api restart"))
	fmt.Println(theme.RenderMuted("  myaaw --agent <agent-name> server restart"))
}

// ensureCodeSigning makes sure the local "Myaaw Agent" identity exists so TCC
// grants survive rebuilds and updates. It creates the identity when missing.
func ensureCodeSigning() {
	fmt.Println()
	fmt.Println(theme.RenderPrimary("🔐 Code-signing"))
	identity := codesign.DefaultIdentity
	if codesign.IdentityExists("") {
		fmt.Println(theme.RenderSuccess("✅ Identity \"" + identity + "\" found; macOS permissions survive rebuilds and updates."))
		return
	}
	fmt.Println(theme.RenderWarning("⚠️  Identity \"" + identity + "\" not found; creating it now..."))
	fmt.Println(theme.RenderMuted("If a keychain dialog appears, enter your login password and choose \"Always Allow\"."))
	res, err := codesign.Setup("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ %v\n", err)
		fmt.Println(theme.RenderMuted("  Computer use stays enabled, but macOS permissions may reset after updates."))
		fmt.Println(theme.RenderMuted("  Fix the keychain/certificate issue, then retry: myaaw codesign setup"))
		return
	}
	if res.Reason == "already exists" {
		fmt.Println(theme.RenderSuccess("✅ Identity \"" + res.Identity + "\" is ready."))
	} else {
		fmt.Println(theme.RenderSuccess("✅ Identity \"" + res.Identity + "\" created."))
	}
}

// printPermissionGuide lists the macOS permissions computer use needs and how
// to grant them. Verification is left to `myaaw doctor` so this command does
// not trigger screen-capture dialogs.
func printPermissionGuide() {
	fmt.Println()
	fmt.Println(theme.RenderPrimary("🖥️  Enable macOS permissions for computer use"))
	fmt.Println(theme.RenderMuted("System Settings → Privacy & Security, granted to the app that runs Myaaw:"))
	fmt.Println("  1. Screen Recording           → screenshot (screencapture)")
	fmt.Println("  2. Accessibility              → mouse/keyboard (cliclick)")
	fmt.Println("  3. Automation → System Events → list_windows, focus_app, scroll (osascript)")
	fmt.Println(theme.RenderMuted("Restart the app after granting Screen Recording, then verify with: myaaw doctor"))
}

func writeFileAtomicCLI(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".myaaw-config-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}
	return os.Rename(name, path)
}
