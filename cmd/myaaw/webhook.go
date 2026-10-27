package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var webhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Manage Telegram webhook",
	Long:  "Set, get info, or delete the Telegram bot webhook.",
}

var webhookInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Get webhook info",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadBaseConfig()
		token := config.TelegramBotToken
		if token == "" {
			fmt.Println(theme.RenderError("❌ Telegram bot token not configured"))
			return
		}
		getWebhookInfo(token)
	},
}

var webhookSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set webhook URL",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadBaseConfig()
		token := config.TelegramBotToken
		if token == "" {
			fmt.Println(theme.RenderError("❌ Telegram bot token not configured"))
			return
		}

		if len(args) > 0 {
			setWebhookURL(token, args[0])
		} else {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print(theme.RenderPrimary("Enter your domain or ngrok URL: "))
			domain, _ := reader.ReadString('\n')
			domain = strings.TrimSpace(domain)
			setWebhookURL(token, domain)
		}
	},
}

var webhookDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete webhook",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadBaseConfig()
		token := config.TelegramBotToken
		if token == "" {
			fmt.Println(theme.RenderError("❌ Telegram bot token not configured"))
			return
		}
		deleteWebhookURL(token)
	},
}

func init() {
	webhookCmd.AddCommand(webhookInfoCmd)
	webhookCmd.AddCommand(webhookSetCmd)
	webhookCmd.AddCommand(webhookDeleteCmd)
}

func setWebhookURL(botToken string, url string) {
	if err := registerTelegramWebhook(botToken, url); err != nil {
		log.Fatalf("Error setting webhook: %v", err)
	}
	fmt.Println(theme.RenderSuccess("✅ Webhook set successfully!"))
}

// registerTelegramWebhook points Telegram at https://<baseURL>/webhook/telegram.
// It returns an error so onboarding can report a failure without exiting.
func registerTelegramWebhook(botToken, baseURL string) error {
	trimmedURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmedURL == "" {
		return fmt.Errorf("webhook URL is required")
	}
	if !strings.HasPrefix(trimmedURL, "https://") {
		return fmt.Errorf("Telegram requires an https:// webhook URL")
	}

	webhookURL := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook?url=%s/webhook/telegram", botToken, trimmedURL)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(webhookURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err == nil && !result.OK {
		return fmt.Errorf("telegram error: %s", result.Description)
	}
	return nil
}

func getWebhookInfo(botToken string) {
	infoUrl := fmt.Sprintf("https://api.telegram.org/bot%s/getWebhookInfo", botToken)
	resp, err := http.Get(infoUrl)
	if err != nil {
		log.Fatalf("Error getting webhook info: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading response body: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Fatalf("Error parsing JSON response: %v", err)
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("Error formatting JSON: %v", err)
	}
	fmt.Println(theme.RenderPrimary("📡 Webhook Info:"))
	fmt.Println(string(jsonData))
}

func deleteWebhookURL(botToken string) {
	deleteUrl := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook?url=", botToken)
	resp, err := http.Get(deleteUrl)
	if err != nil {
		log.Fatalf("Error deleting webhook: %v", err)
	}
	defer resp.Body.Close()

	fmt.Printf("%s Response status: %s\n", theme.RenderSuccess("✅ Webhook deleted successfully!"), resp.Status)
}
