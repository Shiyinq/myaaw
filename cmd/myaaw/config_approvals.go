package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"myaaw/internal/agent/approvals"
	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"

	"github.com/spf13/cobra"
)

var approvalsCmd = &cobra.Command{
	Use:   "approvals",
	Short: "Inspect and manage tool approvals",
}

var approvalsShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the effective approval policy and grants",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadApprovalsConfig()
		if err != nil {
			return err
		}
		effective := config.MergeApprovals(cfg.Approvals)
		grants, err := loadApprovalGrants()
		if err != nil {
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printApprovalsJSON(effective, grants)
		}
		printEffectiveApprovals(effective)
		printApprovalGrants(grants)
		return nil
	},
}

var approvalsSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Update the approval policy (mode, timeout, unattended, subagents)",
	RunE: func(cmd *cobra.Command, args []string) error {
		var mode, unattended, subagents *string
		var timeout *int
		if cmd.Flags().Changed("mode") {
			value, _ := cmd.Flags().GetString("mode")
			mode = &value
		}
		if cmd.Flags().Changed("timeout") {
			value, _ := cmd.Flags().GetInt("timeout")
			timeout = &value
		}
		if cmd.Flags().Changed("unattended") {
			value, _ := cmd.Flags().GetString("unattended")
			unattended = &value
		}
		if cmd.Flags().Changed("subagents") {
			value, _ := cmd.Flags().GetString("subagents")
			subagents = &value
		}
		if err := setApprovalPolicy(mode, timeout, unattended, subagents); err != nil {
			return err
		}
		cfg, err := loadApprovalsConfig()
		if err != nil {
			return err
		}
		printEffectiveApprovals(config.MergeApprovals(cfg.Approvals))
		return nil
	},
}

var approvalsAllowCmd = &cobra.Command{
	Use:   "allow",
	Short: "Manage the approval whitelist",
}

var approvalsDenyCmd = &cobra.Command{
	Use:   "deny",
	Short: "Manage the hard-deny list",
}

var approvalsGrantsCmd = &cobra.Command{
	Use:   "grants",
	Short: "Inspect and revoke runtime grants",
}

var approvalsGrantsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List runtime grants",
	RunE: func(cmd *cobra.Command, args []string) error {
		grants, err := loadApprovalGrants()
		if err != nil {
			return err
		}
		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printGrantsJSON(grants)
		}
		printApprovalGrants(grants)
		return nil
	},
}

var approvalsGrantsRevokeCmd = &cobra.Command{
	Use:   "revoke <key>",
	Short: "Revoke a grant (all scopes, one actor, or one session)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := agentdir.ActiveApprovalsPath()
		if err != nil {
			return err
		}
		store := approvals.NewStore(path)
		key := strings.TrimSpace(args[0])
		if key == "" {
			return fmt.Errorf("key is required")
		}
		session, _ := cmd.Flags().GetString("session")
		channel, _ := cmd.Flags().GetString("channel")
		userID, _ := cmd.Flags().GetString("user")
		session = strings.TrimSpace(session)
		channel = strings.ToLower(strings.TrimSpace(channel))
		userID = strings.TrimSpace(userID)
		if session != "" {
			scope := approvals.SessionScope{
				Scope:          approvals.Scope{Channel: channel, UserID: userID},
				ConversationID: session,
			}
			removed, err := store.RevokeSession(key, scope)
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("grant %q for session %q not found", key, session)
			}
			fmt.Println(theme.RenderSuccess(fmt.Sprintf("✅ Revoked %s for session %s", key, session)))
			return nil
		}
		if channel != "" || userID != "" {
			removed, err := store.RevokeScope(key, approvals.Scope{Channel: channel, UserID: userID})
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("grant %q for %s not found", key, scopeLabel(channel, userID))
			}
			fmt.Println(theme.RenderSuccess(fmt.Sprintf("✅ Revoked %s for %s", key, scopeLabel(channel, userID))))
			return nil
		}

		if _, ok := store.Snapshot()[key]; !ok {
			return fmt.Errorf("grant %q not found", key)
		}
		if err := store.Revoke(key); err != nil {
			return err
		}
		fmt.Println(theme.RenderSuccess(fmt.Sprintf("✅ Revoked all grants for %s", key)))
		return nil
	},
}

var approvalsGrantsClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove every runtime grant",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := agentdir.ActiveApprovalsPath()
		if err != nil {
			return err
		}
		if err := approvals.NewStore(path).Clear(); err != nil {
			return err
		}
		fmt.Println(theme.RenderSuccess("✅ Cleared all approval grants"))
		return nil
	},
}

func init() {
	approvalsShowCmd.Flags().Bool("json", false, "print machine-readable JSON")
	approvalsGrantsListCmd.Flags().Bool("json", false, "print machine-readable JSON")

	approvalsSetCmd.Flags().String("mode", "", "manual or off")
	approvalsSetCmd.Flags().Int("timeout", 0, "seconds to wait for a decision (1-3600)")
	approvalsSetCmd.Flags().String("unattended", "", "allow or deny")
	approvalsSetCmd.Flags().String("subagents", "", "inherit, allow, or deny")

	approvalsAllowCmd.AddCommand(
		approvalsListCmd("allow"),
		approvalsMutateCmd("allow", "add"),
		approvalsMutateCmd("allow", "remove"),
	)
	approvalsDenyCmd.AddCommand(
		approvalsListCmd("deny"),
		approvalsMutateCmd("deny", "add"),
		approvalsMutateCmd("deny", "remove"),
	)

	approvalsGrantsRevokeCmd.Flags().String("session", "", "revoke only this conversation id")
	approvalsGrantsRevokeCmd.Flags().String("channel", "", "restrict to this channel (telegram, discord, api, cli)")
	approvalsGrantsRevokeCmd.Flags().String("user", "", "restrict to this user id")
	approvalsGrantsCmd.AddCommand(approvalsGrantsListCmd, approvalsGrantsRevokeCmd, approvalsGrantsClearCmd)

	approvalsCmd.AddCommand(approvalsShowCmd, approvalsSetCmd, approvalsAllowCmd, approvalsDenyCmd, approvalsGrantsCmd)
	configCmd.AddCommand(approvalsCmd)
}

func approvalsListCmd(list string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List " + list + " entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadApprovalsConfig()
			if err != nil {
				return err
			}
			effective := config.MergeApprovals(cfg.Approvals)
			entries := effective.Allow
			heading := "Allow"
			if list == "deny" {
				entries = effective.Deny
				heading = "Deny"
			}
			fmt.Println(theme.RenderPrimary(heading))
			if len(entries) == 0 {
				fmt.Println(theme.RenderMuted("  (empty)"))
				return nil
			}
			for _, entry := range entries {
				fmt.Println("  " + entry)
			}
			return nil
		},
	}
}

func approvalsMutateCmd(list, action string) *cobra.Command {
	verb := "Add"
	if action == "remove" {
		verb = "Remove"
	}
	return &cobra.Command{
		Use:   action + " <key>",
		Short: verb + " one " + list + " entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := strings.TrimSpace(args[0])
			if key == "" {
				return fmt.Errorf("key is required")
			}
			if err := mutateApprovalList(list, action, key); err != nil {
				return err
			}
			cfg, err := loadApprovalsConfig()
			if err != nil {
				return err
			}
			printEffectiveApprovals(config.MergeApprovals(cfg.Approvals))
			return nil
		},
	}
}

func mutateApprovalList(list, action, key string) error {
	cfg, err := loadApprovalsConfig()
	if err != nil {
		return err
	}
	next := config.MergeApprovals(cfg.Approvals)
	target, other := &next.Allow, next.Deny
	if list == "deny" {
		target, other = &next.Deny, next.Allow
	}

	if action == "add" {
		for _, existing := range other {
			if existing == key {
				return fmt.Errorf("key %q is already in the %s list; remove it there first", key, otherListName(list))
			}
		}
		*target = append(*target, key)
	} else {
		filtered := (*target)[:0]
		found := false
		for _, existing := range *target {
			if existing == key {
				found = true
				continue
			}
			filtered = append(filtered, existing)
		}
		if !found {
			return fmt.Errorf("key %q not found in the %s list", key, list)
		}
		*target = filtered
	}

	return saveApprovalsConfig(cfg, next)
}

func setApprovalPolicy(mode *string, timeout *int, unattended, subagents *string) error {
	cfg, err := loadApprovalsConfig()
	if err != nil {
		return err
	}
	next := config.MergeApprovals(cfg.Approvals)
	if mode != nil {
		next.Mode = *mode
	}
	if timeout != nil {
		next.TimeoutSeconds = timeout
	}
	if unattended != nil {
		next.Unattended = *unattended
	}
	if subagents != nil {
		next.Subagents = *subagents
	}
	return saveApprovalsConfig(cfg, next)
}

func otherListName(list string) string {
	if list == "allow" {
		return "deny"
	}
	return "allow"
}

func loadApprovalsConfig() (*config.Config, error) {
	cfg, err := config.LoadJSONConfigOnly()
	if err != nil || cfg == nil {
		return nil, fmt.Errorf("no config.json found for the active agent; run 'myaaw onboard' first")
	}
	return cfg, nil
}

func saveApprovalsConfig(cfg *config.Config, next config.ApprovalsConfig) error {
	next = config.NormalizeApprovals(next)
	if err := config.ValidateApprovals(&next); err != nil {
		return err
	}
	cfg.Approvals = &next
	if err := config.SaveConfig(cfg); err != nil {
		return err
	}
	return nil
}

func loadApprovalGrants() (map[string]approvals.Grant, error) {
	path, err := agentdir.ActiveApprovalsPath()
	if err != nil {
		return nil, err
	}
	return approvals.NewStore(path).Snapshot(), nil
}

func printEffectiveApprovals(a config.ApprovalsConfig) {
	timeout := 60
	if a.TimeoutSeconds != nil {
		timeout = *a.TimeoutSeconds
	}
	fmt.Println(theme.RenderPrimary("Approvals"))
	fmt.Printf("  mode:               %s\n", a.Mode)
	fmt.Printf("  timeout_seconds:    %d\n", timeout)
	fmt.Printf("  unattended:         %s\n", a.Unattended)
	fmt.Printf("  subagents:          %s\n", a.Subagents)
	fmt.Printf("  allow (%d):          %s\n", len(a.Allow), strings.Join(a.Allow, ", "))
	fmt.Printf("  deny (%d):           %s\n", len(a.Deny), strings.Join(a.Deny, ", "))
}

func printApprovalGrants(grants map[string]approvals.Grant) {
	fmt.Println()
	fmt.Println(theme.RenderPrimary("Grants"))
	if len(grants) == 0 {
		fmt.Println(theme.RenderMuted("  (none)"))
		return
	}
	keys := make([]string, 0, len(grants))
	for key := range grants {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		grant := grants[key]
		fmt.Printf("  %s\n", key)
		for _, scope := range grant.Always {
			fmt.Printf("    always:  %s\n", scopeLabel(scope.Channel, scope.UserID))
		}
		for _, entry := range grant.Sessions {
			fmt.Printf("    session: %s conversation=%s created=%s\n",
				scopeLabel(entry.Channel, entry.UserID), entry.ConversationID,
				entry.CreatedAt.Format(time.RFC3339))
		}
	}
}

func scopeLabel(channel, userID string) string {
	if channel == "" {
		channel = "*"
	}
	if userID == "" {
		userID = "*"
	}
	return "channel=" + channel + " user=" + userID
}

type scopeJSON struct {
	Channel string `json:"channel"`
	UserID  string `json:"user_id"`
}

type sessionGrantJSON struct {
	Channel   string    `json:"channel"`
	UserID    string    `json:"user_id"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type grantJSON struct {
	Key      string             `json:"key"`
	Always   []scopeJSON        `json:"always,omitempty"`
	Sessions []sessionGrantJSON `json:"sessions,omitempty"`
}

type approvalsViewJSON struct {
	Config config.ApprovalsConfig `json:"config"`
	Grants []grantJSON            `json:"grants"`
}

func printApprovalsJSON(a config.ApprovalsConfig, grants map[string]approvals.Grant) error {
	data, err := json.MarshalIndent(approvalsViewJSON{Config: a, Grants: grantsJSON(grants)}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printGrantsJSON(grants map[string]approvals.Grant) error {
	data, err := json.MarshalIndent(grantsJSON(grants), "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func grantsJSON(grants map[string]approvals.Grant) []grantJSON {
	out := make([]grantJSON, 0, len(grants))
	for key, grant := range grants {
		item := grantJSON{Key: key}
		for _, scope := range grant.Always {
			item.Always = append(item.Always, scopeJSON{Channel: scope.Channel, UserID: scope.UserID})
		}
		for _, entry := range grant.Sessions {
			item.Sessions = append(item.Sessions, sessionGrantJSON{
				Channel:   entry.Channel,
				UserID:    entry.UserID,
				ID:        entry.ConversationID,
				CreatedAt: entry.CreatedAt,
			})
		}
		sort.Slice(item.Sessions, func(i, j int) bool {
			return item.Sessions[i].CreatedAt.Before(item.Sessions[j].CreatedAt)
		})
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
