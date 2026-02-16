package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"myaaw"
	"myaaw/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Interactive setup for first-time use",
	Long:  "Initialize ~/.myaaw configuration folder, setup .env, and get everything ready.",
	Run:   runOnboard,
}

func init() {
	rootCmd.AddCommand(onboardCmd)
}

func runOnboard(cmd *cobra.Command, args []string) {
	fmt.Println("🌟 Welcome to Myaaw Onboarding!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("❌ Error getting home directory: %v", err)
	}
	myaawHome := filepath.Join(homeDir, ".myaaw")

	// 1. Initialize Folder
	setupMyaawHome(myaawHome)

	// 2. Setup .env
	setupEnv(myaawHome)

	// 3. Docker setup (optional)
	askDockerSetup()

	fmt.Println("\n✨ Onboarding complete!")
	fmt.Println("💡 You can now run 'myaaw status' to check your setup.")
}

func setupMyaawHome(targetDir string) {
	if _, err := os.Stat(targetDir); err == nil {
		if !askYesNo(fmt.Sprintf("⚠️  %s already exists. Re-initialize with defaults?", targetDir), false) {
			fmt.Println("ℹ️  Skipping folder initialization.")
			return
		}
	}

	fmt.Printf("📂 Initializing %s...\n", targetDir)
	err := extractEmbedDir(myaaw.DefaultMyaawDir, ".myaaw", targetDir)
	if err != nil {
		log.Printf("❌ Error extracting defaults: %v", err)
	} else {
		fmt.Println("✅ Default configuration and skills extracted.")
	}
}

func setupEnv(targetDir string) {
	envFile := filepath.Join(targetDir, ".env")
	configFile := filepath.Join(targetDir, "config.json")

	exists := false
	if _, err := os.Stat(envFile); err == nil {
		exists = true
	}

	if exists {
		if !askYesNo("⚠️  .env already exists. Update it with new keys?", false) {
			fmt.Println("ℹ️  Skipping .env setup.")
		} else {

			runEnvWizard(envFile)
		}
	} else {
		err := os.WriteFile(envFile, []byte(myaaw.DefaultEnvExample), 0644)
		if err != nil {
			log.Fatalf("❌ Error creating .env: %v", err)
		}
		fmt.Println("📝 Created .env from template.")
		runEnvWizard(envFile)
	}

	if askYesNo("\n💬 Configure Channels (Telegram/Discord) and Heartbeat?", true) {
		runConfigWizard(configFile)
	}

	setupGlobalPath()
}

func runEnvWizard(envFile string) {
	fmt.Println("\n🔑 Environment Variables Wizard")
	fmt.Println("---------------------------------------")
	reader := bufio.NewReader(os.Stdin)

	apiKey := promptUser(reader, "Enter LLM Provider API Key", "")
	if apiKey != "" {
		updateEnvFile(envFile, "LLM_PROVIDER_API_KEY", apiKey)
	}

	fmt.Println("✅ Environment variables updated.")
}

func runConfigWizard(configPath string) {
	fmt.Println("\n📡 Channel & Heartbeat Wizard")
	fmt.Println("---------------------------------------")
	reader := bufio.NewReader(os.Stdin)

	// 1. Load current config
	cfg, err := loadConfigFromFile(configPath)
	if err != nil {
		fmt.Printf("⚠️  Warning: Could not load config.json: %v\n", err)
		return
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	fmt.Println("ℹ️  This wizard helps you set basic tokens.")
	fmt.Printf("👉 For advanced settings, please edit: %s\n", configPath)

	// --- Channel Selection ---
	fmt.Println("\nWhich channels would you like to configure?")
	fmt.Println("1) Telegram")
	fmt.Println("2) Discord")
	fmt.Println("3) Both")
	fmt.Println("4) Skip / Keep Current")
	choice := promptUser(reader, "Select an option [1-4]", "4")

	if choice == "1" || choice == "3" {
		if cfg.Channels.Telegram == nil {
			cfg.Channels.Telegram = &config.ChannelConfig{Active: false}
		}
		isUnset := cfg.Channels.Telegram.Token == "" || isPlaceholder(cfg.Channels.Telegram.Token)
		current := cfg.Channels.Telegram.Token
		if !isUnset {
			fmt.Printf("ℹ️  Telegram token is already set: %s\n", maskToken(current))
			if askYesNo("Update Telegram token?", false) {
				isUnset = true
			}
		}
		if isUnset {
			token := promptUser(reader, "Enter Telegram Bot Token", "")
			if token != "" {
				cfg.Channels.Telegram.Active = true
				cfg.Channels.Telegram.Token = token
				fmt.Println("✅ Telegram configuration updated.")
			}
		}
	}

	if choice == "2" || choice == "3" {
		if cfg.Channels.Discord == nil {
			cfg.Channels.Discord = &config.ChannelConfig{Active: false}
		}
		isUnset := cfg.Channels.Discord.Token == "" || isPlaceholder(cfg.Channels.Discord.Token)
		current := cfg.Channels.Discord.Token
		if !isUnset {
			fmt.Printf("ℹ️  Discord token is already set: %s\n", maskToken(current))
			if askYesNo("Update Discord token?", false) {
				isUnset = true
			}
		}
		if isUnset {
			token := promptUser(reader, "Enter Discord Bot Token", "")
			if token != "" {
				cfg.Channels.Discord.Active = true
				cfg.Channels.Discord.Token = token
				fmt.Println("✅ Discord configuration updated.")
			}
		}
	}

	// --- Heartbeat Routing ---
	fmt.Println("\n[Heartbeat Service]")
	if askYesNo("Activate Heartbeat?", false) {
		cfg.Heartbeat.Active = true
		to := promptUser(reader, "Enter Heartbeat Recipient ID", cfg.Heartbeat.To)
		if to == "" || isPlaceholder(to) {
			to = "123456789"
		}
		cfg.Heartbeat.To = to

		fmt.Println("Route heartbeat to:")
		fmt.Println("1) Telegram")
		fmt.Println("2) Discord")
		hbChannel := promptUser(reader, "Select channel [1-2]", "1")
		if hbChannel == "2" {
			cfg.Heartbeat.Channel = "discord"
		} else {
			cfg.Heartbeat.Channel = "telegram"
		}
		cfg.Heartbeat.Every = "30m"
		fmt.Printf("✅ Heartbeat set to channel '%s' for recipient: %s\n", cfg.Heartbeat.Channel, to)
	}

	// --- Bot Owner ---
	isOwnerPlaceholder := len(cfg.Bot.OwnerIDs) == 1 && isPlaceholder(cfg.Bot.OwnerIDs[0])
	if len(cfg.Bot.OwnerIDs) == 0 || isOwnerPlaceholder {
		fmt.Println("\n[Bot Owner]")
		ownerID := promptUser(reader, "Enter Your Primary Owner ID (Telegram/Discord ID)", "")
		if ownerID != "" {
			cfg.Bot.OwnerIDs = []string{ownerID}
			fmt.Println("✅ Owner ID set.")
		}
	}

	// Save back
	err = saveConfigToFile(configPath, cfg)
	if err != nil {
		fmt.Printf("❌ Failed to save config.json: %v\n", err)
	} else {
		fmt.Println("\n✅ Configuration file updated successfully.")
	}
}

func maskToken(t string) string {
	if len(t) < 10 {
		return "****"
	}
	return t[:4] + "...." + t[len(t)-4:]
}

func isPlaceholder(val string) bool {
	v := strings.ToUpper(val)
	return strings.HasPrefix(v, "YOUR_") || strings.Contains(v, "PLACEHOLDER")
}

func loadConfigFromFile(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveConfigToFile(path string, cfg *config.Config) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func setupGlobalPath() {
	fmt.Println("\n🌍 Global Command Setup")
	fmt.Println("---------------------------------------")
	exe, _ := os.Executable()

	if runtime.GOOS == "windows" {
		fmt.Println("To use 'myaaw' from anywhere, run this in PowerShell as Administrator:")
		fmt.Printf("[Environment]::SetEnvironmentVariable(\"Path\", $env:Path + \";%s\", \"User\")\n", filepath.Dir(exe))
	} else {
		fmt.Printf("Current binary location: %s\n", exe)
		if askYesNo("Would you like to move 'myaaw' to /usr/local/bin/ so it's available everywhere?", false) {
			fmt.Println("ℹ️  Running: sudo mv", exe, "/usr/local/bin/myaaw")
			// Use exec.Command to allow interactive sudo password prompt
			cmd := exec.Command("sudo", "mv", exe, "/usr/local/bin/myaaw")
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			err := cmd.Run()
			if err != nil {
				fmt.Println("⚠️  Failed to move binary automatically.")
				fmt.Println("Please run this command manually to make 'myaaw' global:")
				fmt.Printf("sudo mv %s /usr/local/bin/myaaw\n", exe)
			} else {
				fmt.Println("✅ Success! 'myaaw' is now a global command.")
			}
		}
	}
}

func askDockerSetup() {
	fmt.Println("\n🐳 Database Setup (Infrastructure)")
	fmt.Println("---------------------------------------")
	if askYesNo("Would you like to setup databases now? (Docker required)", true) {
		fmt.Println("ℹ️  Running 'myaaw docker setup'...")
		if err := runDockerSetup(); err != nil {
			fmt.Printf("❌ Docker setup failed: %v\n", err)
		}
	}
}

// runDockerSetup is implemented in docker.go

// Helpers

func askYesNo(question string, defaultYes bool) bool {
	prompt := "[y/N]"
	if defaultYes {
		prompt = "[Y/n]"
	}
	fmt.Printf("%s %s: ", question, prompt)

	var input string
	fmt.Scanln(&input)
	input = strings.ToLower(strings.TrimSpace(input))

	if input == "" {
		return defaultYes
	}
	return input == "y" || input == "yes"
}

func promptUser(reader *bufio.Reader, label string, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", label, defaultValue)
	} else {
		fmt.Printf("%s: ", label)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}
	return input
}

func updateEnvFile(path, key, value string) {
	input, err := os.ReadFile(path)
	if err != nil {
		log.Printf("Error reading env file: %v", err)
		return
	}

	lines := strings.Split(string(input), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, key+"=") {
			lines[i] = fmt.Sprintf("%s=%s", key, value)
			found = true
			break
		}
	}

	if !found {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	output := strings.Join(lines, "\n")
	err = os.WriteFile(path, []byte(output), 0644)
	if err != nil {
		log.Printf("Error writing env file: %v", err)
	}
}

func extractEmbedDir(embeddedFS embed.FS, src string, dest string) error {
	entries, err := embeddedFS.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			err = os.MkdirAll(destPath, 0755)
			if err != nil {
				return err
			}
			err = extractEmbedDir(embeddedFS, srcPath, destPath)
			if err != nil {
				return err
			}
		} else {
			data, err := embeddedFS.ReadFile(srcPath)
			if err != nil {
				return err
			}
			err = os.WriteFile(destPath, data, 0644)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
