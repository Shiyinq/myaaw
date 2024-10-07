package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func loadEnv() string {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatalf("Bot token not found in .env")
	}
	return botToken
}

func setWebhook(botToken string, url string) {
	trimmedUrl := strings.TrimFunc(url, func(r rune) bool { return r == '/' })

	webhookUrl := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook?url=%s/webhook/telegram", botToken, trimmedUrl)
	resp, err := http.Get(webhookUrl)
	if err != nil {
		log.Fatalf("Error setting webhook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Printf("Webhook set successfully! Response status: %s\n", resp.Status)
	} else {
		fmt.Printf("Error setting webhook: %s\n", resp.Status)
	}
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
	fmt.Println("Webhook Info JSON:")
	fmt.Println(string(jsonData))
}

func deleteWebhook(botToken string) {
	deleteUrl := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook?url=", botToken)
	resp, err := http.Get(deleteUrl)
	if err != nil {
		log.Fatalf("Error deleting webhook: %v", err)
	}
	defer resp.Body.Close()

	fmt.Printf("Webhook deleted successfully! Response status: %s\n", resp.Status)
}

func main() {
	botToken := loadEnv()

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Welcome to Telegram Webhook CLI")
	fmt.Println("===============================")
	fmt.Println("Choose an option:")
	fmt.Println("1. Set Webhook")
	fmt.Println("2. Get Webhook Info")
	fmt.Println("3. Delete Webhook")
	fmt.Println("4. Exit CLI")
	fmt.Print("\nEnter choice: ")

	for {
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			fmt.Print("Enter your domain or ngrok URL: ")
			domain, _ := reader.ReadString('\n')
			domain = strings.TrimSpace(domain)
			setWebhook(botToken, domain)
		case "2":
			getWebhookInfo(botToken)
		case "3":
			deleteWebhook(botToken)
		case "4":
			fmt.Println("Goodbye!")
			os.Exit(0)
		default:
			fmt.Println("Invalid choice, please choose 1, 2, 3, or 4.")
		}
		fmt.Print("\nEnter next choice: ")
	}
}
