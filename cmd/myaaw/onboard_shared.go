package main

// Thin bindings between the CLI/TUI and the shared onboard core. The core does
// the actual work; these keep the local call sites (and tests) unchanged so the
// interactive wizard, headless CLI, and control-plane API stay in sync.

import (
	"myaaw/internal/config"
	"myaaw/internal/onboard"
)

// onboardInput is the same structure the core applies, so the TUI, headless
// flags, and the API all describe a configuration the same way.
type onboardInput = onboard.Input

func initializeAgent(name string) (error, error) { return onboard.Initialize(name, true) }

func applyOnboarding(name string, in onboardInput, port int) (*config.Config, []string, error) {
	return onboard.Apply(name, in, port)
}

func mergeOnboardConfig(base *config.Config, in onboardInput) *config.Config {
	return onboard.MergeConfig(base, in)
}

func sanitizeOnboardConfig(cfg *config.Config) { onboard.SanitizeConfig(cfg) }

func isTemplateValue(value string) bool { return onboard.IsTemplateValue(value) }

func validateTelegramToken(token string) error { return onboard.ValidateTelegramToken(token) }

func validateWebhookURL(value string) error { return onboard.ValidateWebhookURL(value) }

func validateChatID(value string) error { return onboard.ValidateChatID(value) }

func parseOwnerIDs(raw string) ([]string, error) { return onboard.ParseOwnerIDs(raw) }

func parseOnboardSelection(raw string, known []string, flag string) ([]string, error) {
	return onboard.ParseOnboardSelection(raw, known, flag)
}

func mergeOwnerIDs(groups ...[]string) []string { return onboard.MergeOwnerIDs(groups...) }

func installSelectedSkills(name string, selected []string) error {
	return onboard.InstallSkills(name, selected)
}

func saveBuiltinToolSelection(name string, disabled []string) error {
	return onboard.SaveToolSelection(name, disabled)
}

func existingDisabledTools(agentName string) []string {
	return onboard.ExistingDisabledTools(agentName)
}

func canonicalToolName(name string) string { return onboard.CanonicalToolName(name) }

func builtinSkillNames() []string { return onboard.BuiltinSkillNames() }

func builtinToolNames() []string { return onboard.BuiltinToolNames() }

func onboardSkillDescriptions() map[string]string { return onboard.SkillDescriptions() }

func onboardToolDescriptions() map[string]string { return onboard.ToolDescriptions() }

func onboardBuiltinCapabilities() onboard.Capabilities { return onboard.BuiltinCapabilities() }

func loadConfigFromFile(path string) (*config.Config, error) { return onboard.LoadConfig(path) }

func prepareAgentEnv(name string, desiredPort int) error {
	return onboard.PrepareEnv(name, desiredPort)
}

func readEnvValue(path, key string) string { return onboard.ReadEnvValue(path, key) }

func registerTelegramWebhook(botToken, baseURL string) error {
	return onboard.RegisterTelegramWebhook(botToken, baseURL)
}
