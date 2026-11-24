package controlplane

import (
	"testing"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

func TestSettingsEndpoints(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	app := mustNewApp(t)

	// Unauthenticated requests are rejected.
	doJSON(t, app, "GET", "/settings", "", "", fiber.StatusUnauthorized, nil)
	doJSON(t, app, "PATCH", "/settings", "", `{"shell_mode":"bots"}`, fiber.StatusUnauthorized, nil)

	token := login(t, app)

	// GET returns defaults matching the WebUI.
	var initial service.Settings
	doJSON(t, app, "GET", "/settings", token, "", fiber.StatusOK, &initial)
	if initial != service.DefaultSettings() {
		t.Fatalf("initial settings = %+v, want %+v", initial, service.DefaultSettings())
	}

	// PATCH merges and persists changes.
	var updated service.Settings
	doJSON(
		t,
		app,
		"PATCH",
		"/settings",
		token,
		`{"shell_mode":"studio","locale":"id","theme":"obsidian","theme_mode":"dark","density":"compact"}`,
		fiber.StatusOK,
		&updated,
	)
	if updated.ShellMode != "studio" || updated.Locale != "id" || updated.Theme != "obsidian" || updated.ThemeMode != "dark" || updated.Density != "compact" {
		t.Fatalf("updated settings = %+v", updated)
	}
	// Untouched defaults remain intact.
	if !updated.AutoScroll || updated.NotifySound != "chime" {
		t.Fatalf("defaults altered unexpectedly: %+v", updated)
	}

	// A new app instance (simulating another browser/restart) reads the persisted settings.
	app2 := mustNewApp(t)
	token2 := login(t, app2)
	var fetched service.Settings
	doJSON(t, app2, "GET", "/settings", token2, "", fiber.StatusOK, &fetched)
	if fetched != updated {
		t.Fatalf("reloaded settings = %+v, want %+v", fetched, updated)
	}
}
