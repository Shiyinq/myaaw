package main

import (
	"fmt"
	"strings"

	"myaaw/internal/cli/theme"
	"myaaw/internal/config"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var channelCmd = &cobra.Command{
	Use:   "channel",
	Short: "Manage communication channels (Telegram, Discord)",
}

var channelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all channels and their status",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadJSONConfigOnly()
		if err != nil || cfg == nil {
			fmt.Println(theme.RenderError("Failed to load config.json. Have you run 'myaaw onboard'?"))
			return
		}

		fmt.Println(theme.RenderPrimary("--- Configured Channels ---"))
		
		if cfg.Channels.Telegram != nil && cfg.Channels.Telegram.Active {
			fmt.Printf("Telegram : %s (Mode: %s)\n", theme.RenderSuccess("Active"), cfg.Channels.Telegram.Mode)
		} else {
			fmt.Printf("Telegram : %s\n", theme.RenderError("Inactive"))
		}

		if cfg.Channels.Discord != nil && cfg.Channels.Discord.Active {
			fmt.Printf("Discord  : %s\n", theme.RenderSuccess("Active"))
		} else {
			fmt.Printf("Discord  : %s\n", theme.RenderError("Inactive"))
		}
	},
}

var channelCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Interactively create a new channel",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadJSONConfigOnly()
		if err != nil || cfg == nil {
			fmt.Println(theme.RenderError("Failed to load config.json. Have you run 'myaaw onboard'?"))
			return
		}

		// Check available channels
		telegramActive := cfg.Channels.Telegram != nil && cfg.Channels.Telegram.Active
		discordActive := cfg.Channels.Discord != nil && cfg.Channels.Discord.Active

		if telegramActive && discordActive {
			fmt.Println(theme.RenderError("All supported channels (Telegram and Discord) are already active. You cannot add duplicates."))
			return
		}

		var options []huh.Option[string]
		if !telegramActive {
			options = append(options, huh.NewOption("Telegram", "telegram"))
		}
		if !discordActive {
			options = append(options, huh.NewOption("Discord", "discord"))
		}

		var selectedChannel string
		var token string
		var telegramMode string

		fmt.Println(theme.RenderPrimary("--- Create Channel ---"))

		err = huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Select Channel").
					Options(options...).
					Value(&selectedChannel),
				huh.NewInput().
					Title("Bot Token").
					EchoMode(huh.EchoModePassword).
					Value(&token).
					Validate(func(v string) error {
						if strings.TrimSpace(v) == "" {
							return fmt.Errorf("token is required")
						}
						return nil
					}),
			),
		).Run()

		if err != nil {
			fmt.Println(theme.RenderError("Setup cancelled."))
			return
		}

		token = strings.TrimSpace(token)

		// Ask for mode if telegram
		if selectedChannel == "telegram" {
			err = huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Telegram Connection Mode").
						Options(
							huh.NewOption("Polling (Recommended)", "polling"),
							huh.NewOption("Webhook", "webhook"),
						).
						Value(&telegramMode),
				),
			).Run()

			if err != nil {
				fmt.Println(theme.RenderError("Setup cancelled."))
				return
			}
		}

		// Save config
		if selectedChannel == "telegram" {
			if cfg.Channels.Telegram == nil {
				cfg.Channels.Telegram = &config.ChannelConfig{}
			}
			cfg.Channels.Telegram.Active = true
			cfg.Channels.Telegram.Token = token
			cfg.Channels.Telegram.Mode = telegramMode
		} else if selectedChannel == "discord" {
			if cfg.Channels.Discord == nil {
				cfg.Channels.Discord = &config.ChannelConfig{}
			}
			cfg.Channels.Discord.Active = true
			cfg.Channels.Discord.Token = token
		}

		err = config.SaveConfig(cfg)
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to save config: %v", err)))
			return
		}

		fmt.Println(theme.RenderSuccess(fmt.Sprintf("Successfully added and activated %s channel!", strings.Title(selectedChannel))))
	},
}

var channelDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Interactively delete (disable) an active channel",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadJSONConfigOnly()
		if err != nil || cfg == nil {
			fmt.Println(theme.RenderError("Failed to load config.json. Have you run 'myaaw onboard'?"))
			return
		}

		telegramActive := cfg.Channels.Telegram != nil && cfg.Channels.Telegram.Active
		discordActive := cfg.Channels.Discord != nil && cfg.Channels.Discord.Active

		if !telegramActive && !discordActive {
			fmt.Println(theme.RenderError("No active channels to remove."))
			return
		}

		var options []huh.Option[string]
		if telegramActive {
			options = append(options, huh.NewOption("Telegram", "telegram"))
		}
		if discordActive {
			options = append(options, huh.NewOption("Discord", "discord"))
		}

		var selectedChannel string

		fmt.Println(theme.RenderPrimary("--- Delete Channel ---"))

		err = huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Select Channel to Delete/Disable").
					Options(options...).
					Value(&selectedChannel),
			),
		).Run()

		if err != nil {
			fmt.Println(theme.RenderError("Operation cancelled."))
			return
		}

		if selectedChannel == "telegram" {
			cfg.Channels.Telegram.Active = false
			cfg.Channels.Telegram.Token = ""
		} else if selectedChannel == "discord" {
			cfg.Channels.Discord.Active = false
			cfg.Channels.Discord.Token = ""
		}

		err = config.SaveConfig(cfg)
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to save config: %v", err)))
			return
		}

		fmt.Println(theme.RenderSuccess(fmt.Sprintf("Successfully disabled %s channel.", strings.Title(selectedChannel))))
	},
}

var channelEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Interactively edit an existing channel",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadJSONConfigOnly()
		if err != nil || cfg == nil {
			fmt.Println(theme.RenderError("Failed to load config.json. Have you run 'myaaw onboard'?"))
			return
		}

		options := []huh.Option[string]{
			huh.NewOption("Telegram", "telegram"),
			huh.NewOption("Discord", "discord"),
		}

		var selectedChannel string
		fmt.Println(theme.RenderPrimary("--- Edit Channel ---"))

		err = huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Select Channel to Edit").
					Options(options...).
					Value(&selectedChannel),
			),
		).Run()

		if err != nil {
			fmt.Println(theme.RenderError("Operation cancelled."))
			return
		}

		var active bool
		var token string
		var telegramMode string

		if selectedChannel == "telegram" {
			if cfg.Channels.Telegram != nil {
				active = cfg.Channels.Telegram.Active
				token = cfg.Channels.Telegram.Token
				telegramMode = cfg.Channels.Telegram.Mode
			}
			if telegramMode == "" {
				telegramMode = "polling"
			}
			
			err = huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title("Active").
						Value(&active),
					huh.NewInput().
						Title("Bot Token (Leave blank to keep unchanged)").
						EchoMode(huh.EchoModePassword).
						Value(&token),
					huh.NewSelect[string]().
						Title("Connection Mode").
						Options(
							huh.NewOption("Polling", "polling"),
							huh.NewOption("Webhook", "webhook"),
						).
						Value(&telegramMode),
				),
			).Run()
		} else {
			if cfg.Channels.Discord != nil {
				active = cfg.Channels.Discord.Active
				token = cfg.Channels.Discord.Token
			}

			err = huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title("Active").
						Value(&active),
					huh.NewInput().
						Title("Bot Token (Leave blank to keep unchanged)").
						EchoMode(huh.EchoModePassword).
						Value(&token),
				),
			).Run()
		}

		if err != nil {
			fmt.Println(theme.RenderError("Operation cancelled."))
			return
		}

		token = strings.TrimSpace(token)
		
		if selectedChannel == "telegram" {
			if cfg.Channels.Telegram == nil {
				cfg.Channels.Telegram = &config.ChannelConfig{}
			}
			cfg.Channels.Telegram.Active = active
			if token != "" {
				cfg.Channels.Telegram.Token = token
			}
			cfg.Channels.Telegram.Mode = telegramMode
		} else {
			if cfg.Channels.Discord == nil {
				cfg.Channels.Discord = &config.ChannelConfig{}
			}
			cfg.Channels.Discord.Active = active
			if token != "" {
				cfg.Channels.Discord.Token = token
			}
		}

		err = config.SaveConfig(cfg)
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to save config: %v", err)))
			return
		}

		fmt.Println(theme.RenderSuccess(fmt.Sprintf("Successfully updated %s channel.", strings.Title(selectedChannel))))
	},
}

func init() {
	channelCmd.AddCommand(channelListCmd)
	channelCmd.AddCommand(channelCreateCmd)
	channelCmd.AddCommand(channelDeleteCmd)
	channelCmd.AddCommand(channelEditCmd)
	rootCmd.AddCommand(channelCmd)
}
