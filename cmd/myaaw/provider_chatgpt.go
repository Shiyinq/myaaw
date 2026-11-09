package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"

	"myaaw/internal/agentdir"
	"myaaw/internal/auth/chatgpt"
	"myaaw/internal/config"
	"myaaw/internal/onboard"
	"myaaw/internal/provider"
	"myaaw/internal/services/controlplane/service"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

// browserCommand uses argv, never a shell. Authorization URLs may carry an
// ID-token hint, so neither URLs nor exec errors containing arguments are logged.
func browserCommand(goos, authorizationURL string) *exec.Cmd {
	switch goos {
	case "darwin":
		return exec.Command("open", authorizationURL)
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", authorizationURL)
	default:
		return exec.Command("xdg-open", authorizationURL)
	}
}

var openChatGPTBrowser = func(authorizationURL string) error {
	if err := browserCommand(runtime.GOOS, authorizationURL).Run(); err != nil {
		return fmt.Errorf("cannot open the system browser; run sign-in in a local desktop session on this machine")
	}
	return nil
}

func signInChatGPT(ctx context.Context, credentialID string) (string, error) {
	manager, err := chatgpt.DefaultManager()
	if err != nil {
		return credentialID, fmt.Errorf("cannot open ChatGPT Subscription credentials")
	}
	attempt, err := manager.Start(ctx, credentialID)
	if err != nil {
		return credentialID, fmt.Errorf("cannot start ChatGPT Subscription sign-in; check the local credential ID and callback listener")
	}
	defer attempt.Cancel()
	id := attempt.Status().CredentialID
	if err := openChatGPTBrowser(attempt.AuthorizationURL); err != nil {
		return id, err
	}
	credential, err := attempt.Wait(ctx)
	if err != nil {
		// Auth errors are sanitized; never print a Credential or authorization URL.
		return id, err
	}
	return credential.ID, nil
}

func newConnectChatGPTCmd() *cobra.Command {
	var model, credentialID, retryCredentialID string
	var makeDefault bool
	cmd := &cobra.Command{
		Use:   "connect-chatgpt [name]",
		Short: "Connect or reconnect ChatGPT Subscription using this machine's browser",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := "chatgpt"
			if len(args) > 0 {
				name = strings.TrimSpace(args[0])
			}
			if name == "" {
				return fmt.Errorf("provider name is required")
			}
			cfg, err := config.LoadJSONConfigOnly()
			if err != nil {
				return err
			}
			if cfg == nil {
				cfg = &config.Config{}
			}
			if cfg.Providers == nil {
				cfg.Providers = make(map[string]config.ProviderConfig)
			}
			saved, exists := cfg.Providers[name]
			if exists && saved.Type != chatgpt.ProviderType {
				return fmt.Errorf("provider %q is not ChatGPT Subscription; choose another name", name)
			}
			id := strings.TrimSpace(credentialID)
			if cmd.Flags().Changed("retry-credential-id") {
				if cmd.Flags().Changed("credential-id") {
					return fmt.Errorf("use only one credential flag")
				}
				id = strings.TrimSpace(retryCredentialID)
				if id == "" {
					return fmt.Errorf("recovery credential ID is required")
				}
			}
			if id == "" {
				id = saved.CredentialID
			}
			// Supplying a saved local credential supports headless attachment. With
			// no flag, reconnect always performs browser reauthorization.
			if !cmd.Flags().Changed("credential-id") {
				ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
				defer cancel()
				fmt.Fprintln(cmd.OutOrStdout(), "Opening ChatGPT Subscription sign-in in the local browser. Waiting for callback...")
				id, err = signInChatGPT(ctx, id)
				if err != nil {
					if id != "" {
						fmt.Fprintf(cmd.ErrOrStderr(), "Recovery credential ID: %s (retry with --retry-credential-id)\n", id)
					}
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Local credential ID: %s\n", id)
			pc := config.ProviderConfig{Type: chatgpt.ProviderType, AuthMode: "oauth", CredentialID: id}
			if err := onboard.NormalizeProviderAuth(&pc); err != nil {
				return err
			}
			pc.DefaultModel = strings.TrimSpace(model)
			if pc.DefaultModel == "" {
				models, err := provider.ModelsForConfig(pc)
				if err != nil {
					return err
				}
				if len(models) == 0 {
					return fmt.Errorf("ChatGPT Subscription returned no available models; no API-key fallback is used")
				}
				if !stdinIsTerminal() {
					return fmt.Errorf("--model is required without an interactive terminal (credential ID: %s)", id)
				}
				options := make([]huh.Option[string], 0, len(models))
				for _, name := range models {
					options = append(options, huh.NewOption(name, name))
				}
				if err := huh.NewSelect[string]().Title("ChatGPT Subscription model").Options(options...).Value(&pc.DefaultModel).Run(); err != nil {
					return err
				}
			}
			cfg.Providers[name] = pc
			if makeDefault || cfg.DefaultProvider == "" {
				cfg.DefaultProvider = name
			}
			if err := config.SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ChatGPT Subscription provider %q saved for agent %q.\n", name, agentdir.Active())
			return nil
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "model ID (otherwise discover models and open a picker)")
	cmd.Flags().BoolVar(&makeDefault, "default", false, "make this the default provider")
	cmd.Flags().StringVar(&credentialID, "credential-id", "", "attach an existing verified local credential without browser login")
	// Recovery must call Start, not Get: pending registrations are not accounts.
	cmd.Flags().StringVar(&retryCredentialID, "retry-credential-id", "", "resume a failed browser registration using its recovery ID")
	return cmd
}

func newChatGPTStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status-chatgpt [name]", Short: "Show metadata-only ChatGPT Subscription connection status", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name := "chatgpt"
		if len(args) > 0 {
			name = args[0]
		}
		view, err := service.NewConfigService().Providers(agentdir.Active())
		if err != nil {
			return err
		}
		for _, p := range view.Providers {
			if p.Name == name && p.Type == chatgpt.ProviderType {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(p)
			}
		}
		return fmt.Errorf("ChatGPT Subscription provider %q not found", name)
	}}
}

func newDisconnectChatGPTCmd() *cobra.Command {
	return &cobra.Command{Use: "disconnect-chatgpt [name]", Short: "Detach a provider without deleting shared credentials or revoking ChatGPT access", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name := "chatgpt"
		if len(args) > 0 {
			name = args[0]
		}
		if err := service.NewConfigService().DisconnectProvider(agentdir.Active(), name); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Provider disconnected. Shared host credentials are retained; remote access is not revoked. Manage app access in ChatGPT Settings.")
		return nil
	}}
}

func init() {
	providerCmd.AddCommand(newConnectChatGPTCmd(), newChatGPTStatusCmd(), newDisconnectChatGPTCmd())
}
