package middleware

import (
	"net/http/httptest"
	"testing"

	"myaaw/internal/config"

	"github.com/gofiber/fiber/v3"
)

func newCORSTestApp(t *testing.T) *fiber.App {
	t.Helper()

	app := fiber.New()
	app.Use(SetupCORS())
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	return app
}

func corsOrigin(t *testing.T, app *fiber.App, origin string) string {
	t.Helper()

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", origin)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	return resp.Header.Get("Access-Control-Allow-Origin")
}

func TestSetupCORSLoopbackByDefault(t *testing.T) {
	old := config.AllowedOrigins
	config.AllowedOrigins = ""
	t.Cleanup(func() { config.AllowedOrigins = old })

	app := newCORSTestApp(t)
	if got := corsOrigin(t, app, "http://localhost:5050"); got != "http://localhost:5050" {
		t.Fatalf("localhost origin = %q", got)
	}
	if got := corsOrigin(t, app, "https://evil.example"); got != "" {
		t.Fatalf("remote origin must opt in, got %q", got)
	}
}

func TestSetupCORSExplicitOrigins(t *testing.T) {
	old := config.AllowedOrigins
	config.AllowedOrigins = "https://a.example, https://b.example"
	t.Cleanup(func() { config.AllowedOrigins = old })

	app := newCORSTestApp(t)
	if got := corsOrigin(t, app, "https://b.example"); got != "https://b.example" {
		t.Fatalf("listed origin = %q", got)
	}
	if got := corsOrigin(t, app, "https://localhost:5050"); got != "" {
		t.Fatalf("loopback should not be implicit once origins are listed, got %q", got)
	}
}

func TestSetupCORSWildcard(t *testing.T) {
	old := config.AllowedOrigins
	config.AllowedOrigins = "*"
	t.Cleanup(func() { config.AllowedOrigins = old })

	app := newCORSTestApp(t)
	if got := corsOrigin(t, app, "https://anywhere.example"); got != "*" {
		t.Fatalf("wildcard origin = %q", got)
	}
}
