package controlplane

import (
	"reflect"
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
	if !reflect.DeepEqual(initial, service.DefaultSettings()) {
		t.Fatalf("initial settings = %+v, want %+v", initial, service.DefaultSettings())
	}

	// PATCH merges and persists changes, including bots_sidebar custom sections.
	var updated service.Settings
	doJSON(
		t,
		app,
		"PATCH",
		"/settings",
		token,
		`{"shell_mode":"studio","locale":"id","theme":"obsidian","theme_mode":"dark","density":"compact","bots_sidebar":{"section_order":["work","agents","groups"],"sections":[{"id":"work","name":"Work","items":[{"kind":"agent","id":"myaagy"},{"kind":"group","id":"grp-1"}]}]}}`,
		fiber.StatusOK,
		&updated,
	)
	if updated.ShellMode != "studio" || updated.Locale != "id" || updated.Theme != "obsidian" || updated.ThemeMode != "dark" || updated.Density != "compact" {
		t.Fatalf("updated settings = %+v", updated)
	}
	if !reflect.DeepEqual(updated.BotsSidebar.SectionOrder, []string{"work", "agents", "groups"}) || len(updated.BotsSidebar.Sections) != 1 || len(updated.BotsSidebar.Sections[0].Items) != 2 {
		t.Fatalf("unexpected bots_sidebar = %+v", updated.BotsSidebar)
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
	if !reflect.DeepEqual(fetched, updated) {
		t.Fatalf("reloaded settings = %+v, want %+v", fetched, updated)
	}
}
