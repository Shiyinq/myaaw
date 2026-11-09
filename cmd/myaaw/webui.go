package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"myaaw/internal/daemon"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
)

const (
	webuiDaemonName    = "myaaw-webui"
	webuiDefaultPort   = "5050"
	webuiDefaultAPIURL = "http://localhost:9000"
	webuiAssetPrefix   = "myaaw-webui-"
	webuiAssetSuffix   = ".tar.gz"
)

// webUIConfig is stored in ~/.myaaw/webui/config.json. It only holds the
// defaults; the browser keeps the user's last choice in localStorage.
type webUIConfig struct {
	Port    string `json:"port"`
	APIURL  string `json:"api_url"`
	Version string `json:"version"`
	// Public disables /setup and the host hints on the login page, for a
	// dashboard served to other people through a tunnel.
	Public bool `json:"public,omitempty"`
}

var (
	webuiVersion        string
	webuiRollbackTarget string
	webuiPort           string
	webuiAPIURL         string
	webuiPurge          bool
	webuiPublic         bool
)

var webuiCmd = &cobra.Command{
	Use:   "webui",
	Short: "Manage the Myaaw dashboard (WebUI)",
	Long:  "Install, run, and manage the optional Myaaw dashboard separately from the binary.",
}

var webuiInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Download and install the WebUI bundle",
	Run: func(cmd *cobra.Command, args []string) {
		if err := installWebUI(webuiVersion); err != nil {
			log.Fatalf("%s: %v", theme.RenderError("Install failed"), err)
		}
	},
}

var webuiRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the WebUI in the foreground",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runWebUI(); err != nil {
			log.Fatalf("%s: %v", theme.RenderError("WebUI failed"), err)
		}
	},
}

var webuiStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the WebUI as a background daemon",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := effectiveWebUIConfig()
		if err := startWebUI(cfg); err != nil {
			log.Fatalf("%s: %v", theme.RenderError("Failed to start WebUI"), err)
		}
	},
}

var webuiStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the WebUI daemon",
	Run: func(cmd *cobra.Command, args []string) {
		dm, err := daemon.NewGlobalManager(webuiDaemonName)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
		if err := dm.Stop(); err != nil {
			log.Fatalf("Failed to stop WebUI: %v", err)
		}
	},
}

var webuiRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the WebUI daemon",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := effectiveWebUIConfig()
		if err := restartWebUI(cfg); err != nil {
			log.Fatalf("Failed to restart WebUI: %v", err)
		}
	},
}

var webuiStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check WebUI daemon status",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := effectiveWebUIConfig()
		dm, err := daemon.NewGlobalManager(webuiDaemonName)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
		pid, running, err := dm.Status()
		if err != nil {
			log.Fatalf("Error checking status: %v", err)
		}
		if running {
			fmt.Printf("%s (PID: %d)\n", theme.RenderSuccess("✅ WebUI is running"), pid)
			fmt.Printf("   http://127.0.0.1:%s\n", cfg.Port)
			if cfg.Public {
				fmt.Println(theme.RenderMuted("   public hosting: /setup is disabled"))
			}
		} else {
			fmt.Println(theme.RenderError("❌ WebUI is stopped"))
		}
	},
}

var webuiUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the WebUI bundle to the latest release",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := effectiveWebUIConfig()
		running := isWebUIRunning()
		if err := installWebUI(webuiVersion); err != nil {
			log.Fatalf("%s: %v", theme.RenderError("Update failed"), err)
		}
		if running {
			if err := restartWebUI(cfg); err != nil {
				log.Fatalf("WebUI updated but restart failed: %v", err)
			}
		}
	},
}

var webuiRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Restore a previously installed WebUI bundle",
	Run: func(cmd *cobra.Command, args []string) {
		if err := rollbackWebUI(webuiRollbackTarget); err != nil {
			log.Fatalf("%s: %v", theme.RenderError("Rollback failed"), err)
		}
	},
}

var webuiUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop and remove the WebUI bundle",
	Run: func(cmd *cobra.Command, args []string) {
		if dm, err := daemon.NewGlobalManager(webuiDaemonName); err == nil {
			_ = dm.Stop()
		}

		root, err := webUIRoot()
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		if webuiPurge {
			credentials := filepath.Join(filepath.Dir(root), "controlplane.json")
			if askYesNo("Also remove control-plane credentials ("+credentials+")?", false) {
				if err := os.Remove(credentials); err != nil && !os.IsNotExist(err) {
					log.Fatalf("Could not remove %s: %v", credentials, err)
				}
				fmt.Println(theme.RenderMuted("Removed " + credentials))
			}
		}

		if err := os.RemoveAll(root); err != nil {
			log.Fatalf("Could not remove %s: %v", root, err)
		}
		fmt.Println(theme.RenderSuccess("🗑️  WebUI removed from " + root))
	},
}

var webuiOpenCmd = &cobra.Command{
	Use:   "open",
	Short: "Open the dashboard in your browser",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := effectiveWebUIConfig()
		if !isWebUIRunning() {
			if err := startWebUI(cfg); err != nil {
				log.Fatalf("Failed to start WebUI: %v", err)
			}
		}
		url := fmt.Sprintf("http://127.0.0.1:%s", cfg.Port)
		fmt.Printf("%s %s\n", theme.RenderSecondary("Opening"), url)
		if err := openBrowser(url); err != nil {
			log.Fatalf("Could not open a browser: %v", err)
		}
	},
}

func init() {
	webuiCmd.PersistentFlags().StringVar(&webuiPort, "port", "", "port to listen on (default "+webuiDefaultPort+")")
	webuiCmd.PersistentFlags().StringVar(&webuiAPIURL, "api-url", "", "control-plane URL (default "+webuiDefaultAPIURL+")")
	webuiCmd.PersistentFlags().BoolVar(&webuiPublic, "public", false, "public hosting: disable /setup and hide host hints")
	webuiInstallCmd.Flags().StringVar(&webuiVersion, "version", "", "install a specific release tag (default: latest)")
	webuiUpdateCmd.Flags().StringVar(&webuiVersion, "version", "", "update to a specific release tag (default: latest)")
	webuiRollbackCmd.Flags().StringVar(&webuiRollbackTarget, "version", "", "release tag to restore (default: list available versions)")
	webuiUninstallCmd.Flags().BoolVar(&webuiPurge, "purge", false, "also remove control-plane credentials")

	webuiCmd.AddCommand(webuiInstallCmd)
	webuiCmd.AddCommand(webuiRunCmd)
	webuiCmd.AddCommand(webuiStartCmd)
	webuiCmd.AddCommand(webuiStopCmd)
	webuiCmd.AddCommand(webuiRestartCmd)
	webuiCmd.AddCommand(webuiStatusCmd)
	webuiCmd.AddCommand(webuiUpdateCmd)
	webuiCmd.AddCommand(webuiRollbackCmd)
	webuiCmd.AddCommand(webuiUninstallCmd)
	webuiCmd.AddCommand(webuiOpenCmd)

	rootCmd.AddCommand(webuiCmd)
}

func webUIRoot() (string, error) {
	root, err := agentdir.MyaawRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "webui"), nil
}

func webUICurrentDir() (string, error) {
	root, err := webUIRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "current"), nil
}

func webUIConfigPath() (string, error) {
	root, err := webUIRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config.json"), nil
}

func webuiEntryPath() (string, error) {
	current, err := webUICurrentDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(current, "build", "index.js"), nil
}

func loadWebUIConfig() (webUIConfig, error) {
	cfg := webUIConfig{Port: webuiDefaultPort, APIURL: webuiDefaultAPIURL}

	path, err := webUIConfigPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Port == "" {
		cfg.Port = webuiDefaultPort
	}
	if cfg.APIURL == "" {
		cfg.APIURL = webuiDefaultAPIURL
	}
	return cfg, nil
}

func effectiveWebUIConfig() webUIConfig {
	cfg, _ := loadWebUIConfig()
	if webuiPort != "" {
		cfg.Port = webuiPort
	}
	if webuiAPIURL != "" {
		cfg.APIURL = webuiAPIURL
	}
	// Only override the saved value when the flag was actually passed, so
	// `--public=false` can turn the mode off again.
	if webuiCmd.PersistentFlags().Changed("public") {
		cfg.Public = webuiPublic
	}
	return cfg
}

func saveWebUIConfig(cfg webUIConfig) error {
	path, err := webUIConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func isWebUIRunning() bool {
	dm, err := daemon.NewGlobalManager(webuiDaemonName)
	if err != nil {
		return false
	}
	_, running, _ := dm.Status()
	return running
}

func startWebUI(cfg webUIConfig) error {
	entry, err := webuiEntryPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(entry); err != nil {
		return fmt.Errorf("WebUI is not installed; run 'myaaw webui install' first")
	}

	dm, err := daemon.NewGlobalManager(webuiDaemonName)
	if err != nil {
		return err
	}
	args := []string{"webui", "run", "--port", cfg.Port, "--api-url", cfg.APIURL}
	if err := dm.Start(args); err != nil {
		return err
	}
	// Keep the config in sync with what is actually running so `status` and the
	// next start agree on the port and control-plane URL.
	_ = saveWebUIConfig(cfg)
	fmt.Printf("%s http://127.0.0.1:%s\n", theme.RenderSuccess("✅ WebUI started"), cfg.Port)
	return nil
}

func restartWebUI(cfg webUIConfig) error {
	dm, err := daemon.NewGlobalManager(webuiDaemonName)
	if err != nil {
		return err
	}
	if _, running, _ := dm.Status(); running {
		fmt.Println(theme.RenderSecondary("🔄 Stopping WebUI..."))
		_ = dm.Stop()
		time.Sleep(1 * time.Second)
	}
	fmt.Println(theme.RenderPrimary("🚀 Starting WebUI..."))
	return startWebUI(cfg)
}

func runWebUI() error {
	cfg := effectiveWebUIConfig()

	entry, err := webuiEntryPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(entry); err != nil {
		return fmt.Errorf("WebUI is not installed; run 'myaaw webui install' first")
	}

	node, err := exec.LookPath("node")
	if err != nil {
		return fmt.Errorf("Node.js 20+ is required but was not found in PATH")
	}

	// The current WebUI still proxies /api server-side, so keep it pointed at the
	// control plane. When the browser-side connection option lands, this becomes
	// just the default the UI seeds its "Server URL" field with.
	env := append(os.Environ(),
		"PORT="+cfg.Port,
		"HOST=127.0.0.1",
		"MYAAW_API_BASE_URL="+cfg.APIURL,
	)
	if cfg.Public {
		env = append(env, "MYAAW_WEBUI_PUBLIC=1")
	}

	if err := os.Chdir(filepath.Dir(filepath.Dir(entry))); err != nil {
		return err
	}
	return launchNode(node, entry, env)
}

func installWebUI(version string) error {
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("Node.js 20+ is required but was not found in PATH")
	}

	root, err := webUIRoot()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}

	rel, err := webuiRelease(version)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", theme.RenderSecondary("Downloading WebUI"), rel.Tag)

	tmpFile, err := os.CreateTemp(root, "webui-*.tar.gz")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpName)

	if err := downloadFile(rel.DownloadURL, tmpName); err != nil {
		return err
	}
	if err := verifyBundleChecksum(tmpName, rel); err != nil {
		return err
	}

	extractDir, err := os.MkdirTemp(root, "current-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)

	if err := extractTarGz(tmpName, extractDir); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(extractDir, "build", "index.js")); err != nil {
		return fmt.Errorf("downloaded bundle is missing build/index.js")
	}

	current := filepath.Join(root, "current")
	oldCfg, _ := loadWebUIConfig()
	backup, err := backupCurrentBundle(current, oldCfg.Version)
	if err != nil {
		return err
	}
	if err := os.Rename(extractDir, current); err != nil {
		_ = restoreBundle(current, backup)
		return err
	}

	cfg := effectiveWebUIConfig()
	cfg.Version = rel.Tag
	if err := saveWebUIConfig(cfg); err != nil {
		return err
	}
	pruneVersionBackups()

	fmt.Printf("%s %s\n", theme.RenderSuccess("✅ WebUI installed"), rel.Tag)
	fmt.Printf("   %s\n", current)
	return nil
}

type webuiReleaseInfo struct {
	Tag         string
	AssetName   string
	DownloadURL string
	ChecksumURL string
}

// webuiRelease resolves the WebUI bundle asset for a release. The bundle is
// platform-independent (pure JS), so there is exactly one asset per release.
func webuiRelease(version string) (webuiReleaseInfo, error) {
	api := githubAPI
	if v := strings.TrimSpace(version); v != "" && v != "latest" {
		api = "https://api.github.com/repos/Shiyinq/myaaw/releases/tags/" + v
	}

	client := resty.New()
	var result struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	resp, err := client.R().SetResult(&result).Get(api)
	if err != nil {
		return webuiReleaseInfo{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		if resp.StatusCode() == http.StatusNotFound {
			return webuiReleaseInfo{}, fmt.Errorf("no release found at %s", api)
		}
		return webuiReleaseInfo{}, fmt.Errorf("GitHub API returned status %d", resp.StatusCode())
	}

	info := webuiReleaseInfo{Tag: result.TagName}
	for _, asset := range result.Assets {
		switch {
		case strings.HasPrefix(asset.Name, webuiAssetPrefix) && strings.HasSuffix(asset.Name, webuiAssetSuffix):
			info.AssetName = asset.Name
			info.DownloadURL = asset.BrowserDownloadURL
		case asset.Name == "SHA256SUMS":
			info.ChecksumURL = asset.BrowserDownloadURL
		}
	}
	if info.DownloadURL == "" {
		return webuiReleaseInfo{}, fmt.Errorf("no WebUI bundle found in release %s (expected %s*%s)", result.TagName, webuiAssetPrefix, webuiAssetSuffix)
	}
	return info, nil
}

func verifyBundleChecksum(filePath string, rel webuiReleaseInfo) error {
	if rel.ChecksumURL == "" {
		return nil
	}
	expected, err := fetchChecksum(rel.ChecksumURL, rel.AssetName)
	if err != nil {
		return err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(sum.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s", rel.AssetName)
	}
	return nil
}

func fetchChecksum(url, assetName string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum download failed with status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == assetName {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("SHA256SUMS does not list %s", assetName)
}

func webUIVersionsDir() (string, error) {
	root, err := webUIRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "versions"), nil
}

func sanitizeVersion(version string) string {
	v := strings.TrimSpace(version)
	v = strings.ReplaceAll(v, "/", "_")
	v = strings.ReplaceAll(v, "\\", "_")
	if v == "" || v == "." || v == ".." {
		return ""
	}
	return v
}

// backupCurrentBundle moves the installed bundle into versions/<version> so it
// can be restored with `webui rollback`. It returns the backup path (empty when
// there was nothing to back up).
func backupCurrentBundle(current, version string) (string, error) {
	if _, err := os.Stat(current); err != nil {
		return "", nil
	}
	versions, err := webUIVersionsDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(versions, 0o755); err != nil {
		return "", err
	}

	name := sanitizeVersion(version)
	if name == "" {
		name = "previous"
	}
	target := filepath.Join(versions, name)
	_ = os.RemoveAll(target)
	if err := os.Rename(current, target); err != nil {
		return "", err
	}
	return target, nil
}

func restoreBundle(current, backup string) error {
	_ = os.RemoveAll(current)
	if backup == "" {
		return nil
	}
	return os.Rename(backup, current)
}

func listWebUIVersions() ([]string, error) {
	versions, err := webUIVersionsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(versions)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func pruneVersionBackups() {
	names, err := listWebUIVersions()
	if err != nil || len(names) <= 3 {
		return
	}
	versions, err := webUIVersionsDir()
	if err != nil {
		return
	}
	for _, name := range names[:len(names)-3] {
		_ = os.RemoveAll(filepath.Join(versions, name))
	}
}

func rollbackWebUI(version string) error {
	names, err := listWebUIVersions()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no previous versions available")
	}
	if version == "" {
		fmt.Println(theme.RenderPrimary("Available versions:"))
		for _, name := range names {
			fmt.Println("  " + name)
		}
		return nil
	}

	target := sanitizeVersion(version)
	versions, err := webUIVersionsDir()
	if err != nil {
		return err
	}
	backup := filepath.Join(versions, target)
	if _, err := os.Stat(backup); err != nil {
		return fmt.Errorf("version %q is not available", version)
	}

	current, err := webUICurrentDir()
	if err != nil {
		return err
	}
	cfg, _ := loadWebUIConfig()
	if _, err := os.Stat(current); err == nil {
		keep := sanitizeVersion(cfg.Version)
		if keep == "" || keep == target {
			keep = "current-" + time.Now().Format("20060102-150405")
		}
		keepPath := filepath.Join(versions, keep)
		_ = os.RemoveAll(keepPath)
		if err := os.Rename(current, keepPath); err != nil {
			return err
		}
	}
	if err := os.Rename(backup, current); err != nil {
		return err
	}

	cfg.Version = target
	if err := saveWebUIConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("%s %s\n", theme.RenderSuccess("✅ Rolled back to"), target)
	if isWebUIRunning() {
		return restartWebUI(cfg)
	}
	return nil
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// extractTarGz safely extracts a .tar.gz archive into dest, rejecting entries
// that escape the destination directory.
func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	cleanDest := filepath.Clean(dest)
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		name := filepath.Clean(hdr.Name)
		if name == "." {
			continue
		}
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("unexpected path in archive: %q", hdr.Name)
		}
		target := filepath.Join(cleanDest, name)
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("unexpected path in archive: %q", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			// .bin symlinks are optional, and Windows needs privileges to create
			// them; a failure must not abort the install.
			_ = os.Symlink(hdr.Linkname, target)
		case tar.TypeLink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			linkName := filepath.Clean(hdr.Linkname)
			if filepath.IsAbs(linkName) || linkName == ".." || strings.HasPrefix(linkName, ".."+string(os.PathSeparator)) {
				return fmt.Errorf("unexpected link target in archive: %q", hdr.Linkname)
			}
			_ = os.Link(filepath.Join(cleanDest, linkName), target)
		}
	}
	return nil
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
