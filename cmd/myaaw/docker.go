package main

import (
	"fmt"
	"log"
	"myaaw"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Manage infrastructure services (MongoDB, Redis, RabbitMQ)",
	Long:  "Helper to manage infrastructure services using Docker Compose.",
}

var dockerSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Start infrastructure services in detached mode",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDockerSetup(); err != nil {
			log.Fatalf("❌ Error: %v", err)
		}
	},
}

var dockerStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop infrastructure services",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDockerCompose("stop"); err != nil {
			log.Fatalf("❌ Error stopping services: %v", err)
		}
		fmt.Println("✅ Infrastructure services stopped.")
	},
}

var dockerLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View logs of infrastructure services",
	Run: func(cmd *cobra.Command, args []string) {
		runDockerCompose("logs", "-f")
	},
}

func init() {
	dockerCmd.AddCommand(dockerSetupCmd)
	dockerCmd.AddCommand(dockerStopCmd)
	dockerCmd.AddCommand(dockerLogsCmd)
	rootCmd.AddCommand(dockerCmd)
}

func runDockerSetup() error {
	fmt.Println("🔍 Checking Docker...")
	if !isDockerInstalled() {
		fmt.Println("❌ Docker is not installed or not running.")
		openDockerDownload()
		return fmt.Errorf("Docker not found. Opening download page...")
	}

	fmt.Println("🚀 Starting infrastructure services (MongoDB, Redis, RabbitMQ)...")
	return runDockerCompose("up", "-d", "mongodb", "redis", "rabbitmq")
}

func isDockerInstalled() bool {
	_, err := exec.LookPath("docker")
	if err != nil {
		return false
	}
	// Also check if docker daemon is running
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func openDockerDownload() {
	url := "https://www.docker.com/products/docker-desktop"
	fmt.Printf("🌐 Opening Docker download page: %s\n", url)

	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	}
	if err != nil {
		fmt.Printf("👉 Please open this link manually: %s\n", url)
	}
}

func runDockerCompose(args ...string) error {
	// First, check if docker-compose.yml exists in ~/.myaaw or current dir
	composePath := findDockerComposePath()
	if composePath == "" {
		// If not found, extract the embedded one to ~/.myaaw
		home, _ := os.UserHomeDir()
		myaawDir := filepath.Join(home, ".myaaw")
		os.MkdirAll(myaawDir, 0755)
		composePath = filepath.Join(myaawDir, "docker-compose.yml")
		err := os.WriteFile(composePath, []byte(myaaw.DefaultDockerCompose), 0644)
		if err != nil {
			return fmt.Errorf("failed to create docker-compose.yml: %w", err)
		}
		fmt.Println("📄 Created docker-compose.yml in ~/.myaaw")
	}

	fullArgs := []string{"compose", "-f", composePath}
	fullArgs = append(fullArgs, args...)

	cmd := exec.Command("docker", fullArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Println("\n❌ Docker command failed.")
		fmt.Println("💡 Tip: If you see 'timeout' or 'proxyconnect' errors, check your internet connection or Docker proxy settings.")
		return err
	}
	return nil
}

func findDockerComposePath() string {
	// 1. Current Dir
	if _, err := os.Stat("docker-compose.yml"); err == nil {
		return "docker-compose.yml"
	}
	// 2. ~/.myaaw
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".myaaw", "docker-compose.yml")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}
