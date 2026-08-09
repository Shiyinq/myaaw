package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"

	"myaaw/internal/cli/theme"
)

var autoConfirm bool

func init() {
	updateCmd.Flags().BoolVar(&autoConfirm, "auto-confirm", false, "Automatically confirm prompts")
	updateCmd.Flags().MarkHidden("auto-confirm")
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install updates",
	Run: func(cmd *cobra.Command, args []string) {
		if !autoConfirm {
			fmt.Printf("%s %s...\n", theme.RenderSecondary("🔍 Checking for updates at"), githubRepo)
		}

		latestVersion, downloadURL, err := getLatestReleaseInfo()
		if err != nil {
			log.Fatalf("%s checking for updates: %v", theme.RenderError("❌ Error"), err)
		}

		if latestVersion == Version {
			if !autoConfirm {
				fmt.Printf("%s (Version: %s)\n", theme.RenderSuccess("✅ Myaaw is already up to date"), Version)
			}
			return
		}

		if !autoConfirm {
			fmt.Printf("%s: %s (Current: %s)\n", theme.RenderPrimary("🆕 New version available"), latestVersion, Version)
			if !askYesNo("Would you like to download and install it?", true) {
				return
			}
		}

		if err := performUpdate(downloadURL); err != nil {
			log.Fatalf("%s: %v", theme.RenderError("❌ Update failed"), err)
		}

		fmt.Println(theme.RenderSuccess("🚀 Update complete! Please restart Myaaw."))
	},
}

const githubRepo = "https://github.com/Shiyinq/myaaw"
const githubAPI = "https://api.github.com/repos/Shiyinq/myaaw/releases/latest"

func getLatestReleaseInfo() (version string, downloadURL string, err error) {
	client := resty.New()
	var result struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	resp, err := client.R().
		SetResult(&result).
		Get(githubAPI)

	if err != nil {
		return "", "", err
	}

	if resp.StatusCode() != http.StatusOK {
		if resp.StatusCode() == http.StatusNotFound {
			return "", "", fmt.Errorf("no releases found at %s. Please ensure you have created at least one release on GitHub", githubRepo)
		}
		return "", "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode())
	}

	version = result.TagName

	// Find asset for current platform
	expectedName := getBinaryNameForPlatform()
	for _, asset := range result.Assets {
		if strings.Contains(asset.Name, expectedName) {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return version, "", fmt.Errorf("could not find binary for your platform (%s) in release %s", expectedName, version)
	}

	return version, downloadURL, nil
}

func getBinaryNameForPlatform() string {
	osName := runtime.GOOS
	archName := runtime.GOARCH

	name := fmt.Sprintf("myaaw-%s-%s", osName, archName)
	if osName == "windows" {
		name += ".exe"
	}
	return name
}

func performUpdate(url string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	// 0. Check write permissions and elevate if needed
	dir := filepath.Dir(exe)
	testFile := filepath.Join(dir, ".myaaw_write_test")
	f, err := os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		if os.IsPermission(err) {
			if runtime.GOOS != "windows" {
				fmt.Println(theme.RenderSecondary("🔐 Administrator privileges required. Prompting for password..."))

				// Re-run the current command with sudo and --auto-confirm
				args := append([]string{exe}, os.Args[1:]...)
				args = append(args, "--auto-confirm")
				cmd := exec.Command("sudo", args...)
				cmd.Stdin = os.Stdin
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr

				if err := cmd.Run(); err != nil {
					return fmt.Errorf("sudo failed: %w", err)
				}

				// Exit gracefully since the elevated child process completed the update
				os.Exit(0)
			}
			return fmt.Errorf("permission denied. Please run your terminal as Administrator")
		}
		return err
	}
	f.Close()
	os.Remove(testFile)

	// 1. Download to temporary file
	tempFile := exe + ".tmp"
	client := resty.New()

	fmt.Print(theme.RenderSecondary("📥 Downloading..."))
	resp, err := client.R().SetOutput(tempFile).Get(url)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode())
	}
	fmt.Println(theme.RenderSuccess(" Done."))

	// 2. Make it executable
	if runtime.GOOS != "windows" {
		os.Chmod(tempFile, 0755)
	}

	// 3. Rename current binary to .old (safest for running processes)
	oldFile := exe + ".old"
	os.Remove(oldFile) // Clean up if exists
	err = os.Rename(exe, oldFile)
	if err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// 4. Move new binary to current location
	err = os.Rename(tempFile, exe)
	if err != nil {
		// Try to roll back
		os.Rename(oldFile, exe)
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	fmt.Println(theme.RenderSuccess("✅ Binary replaced successfully."))
	return nil
}
