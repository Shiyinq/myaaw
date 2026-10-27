package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/agentdir"

	tea "github.com/charmbracelet/bubbletea"
)

const updateControlConfig = `{
  "default_provider": "openai",
  "providers": {"openai": {"type": "openai", "api_key": "sk-existing", "default_model": "gpt-4o"}},
  "channels": {"telegram": {"active": true, "token": "123:abc", "mode": "polling"}},
  "heartbeat": {"active": true, "channel": "telegram", "to": "42", "every": "1h"},
  "bot": {"type": "private", "owner_ids": ["42"]}
}
`

func TestHeartbeatShownAsUnchangedWhenSkipped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-hb-keep", updateControlConfig)

	m := initialOnboardModel("agent-hb-keep")
	if !m.existingHeartbeat {
		t.Fatal("saved heartbeat was not detected")
	}
	summary := m.renderSummary()
	if !strings.Contains(summary, "unchanged (current)") {
		t.Fatalf("review does not mark the heartbeat as unchanged:\n%s", summary)
	}

	// Skip must leave the config alone.
	merged := mergeOnboardConfig(loadAgentConfig("agent-hb-keep"), m.input())
	if !merged.Heartbeat.Active || merged.Heartbeat.To != "42" {
		t.Fatalf("heartbeat changed on skip: %+v", merged.Heartbeat)
	}
}

func TestHeartbeatCanBeDisabledAndIsPrefilled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-hb-off", updateControlConfig)

	m := initialOnboardModel("agent-hb-off")
	m.setupHeartbeatChannel()
	if len(m.choices) == 0 || m.choices[m.cursor] != "Telegram" {
		t.Fatalf("cursor should start on the current channel, choices = %v", m.choices)
	}
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.step != stepHeartbeatID || m.textInput.Value() != "42" {
		t.Fatalf("step = %d value = %q, want the saved destination", m.step, m.textInput.Value())
	}
	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.step != stepHeartbeatEvery || m.textInput.Value() != "1h" {
		t.Fatalf("step = %d value = %q, want the saved interval", m.step, m.textInput.Value())
	}

	// Re-enter the channel step and pick the explicit off switch.
	m.setupHeartbeatChannel()
	cursor := -1
	for i, choice := range m.choices {
		if choice == "Disable heartbeat" {
			cursor = i
		}
	}
	if cursor < 0 {
		t.Fatalf("no disable option in %v", m.choices)
	}
	m.cursor = cursor
	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if !m.heartbeatSet || m.heartbeat {
		t.Fatalf("heartbeatSet = %v heartbeat = %v, want an explicit off", m.heartbeatSet, m.heartbeat)
	}

	merged := mergeOnboardConfig(loadAgentConfig("agent-hb-off"), m.input())
	if merged.Heartbeat.Active {
		t.Fatalf("heartbeat still active after disabling: %+v", merged.Heartbeat)
	}
	if merged.Heartbeat.To != "42" || merged.Heartbeat.Every != "1h" {
		t.Fatalf("disabling should keep the saved destination: %+v", merged.Heartbeat)
	}
}

func TestChannelsCanBeDisabledOnUpdate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-ch-off", updateControlConfig)

	m := initialOnboardModel("agent-ch-off")
	m.enterChannelExistingStep()
	if m.step != stepChannelExisting {
		t.Fatalf("step = %d, want stepChannelExisting", m.step)
	}
	cursor := -1
	for i, choice := range m.choices {
		if choice == "Disable Telegram" {
			cursor = i
		}
	}
	if cursor < 0 {
		t.Fatalf("no disable option in %v", m.choices)
	}
	m.cursor = cursor
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if !m.disableTelegram {
		t.Fatal("disableTelegram was not set")
	}
	if m.channelConfigured("telegram") {
		t.Fatal("a disabled channel must not count as configured")
	}
	if !strings.Contains(m.renderSummary(), "disabled") {
		t.Fatalf("review does not show the channel as disabled:\n%s", m.renderSummary())
	}

	merged := mergeOnboardConfig(loadAgentConfig("agent-ch-off"), m.input())
	if merged.Channels.Telegram == nil {
		t.Fatal("telegram config was dropped instead of disabled")
	}
	if merged.Channels.Telegram.Active {
		t.Fatal("telegram is still active")
	}
	if merged.Channels.Telegram.Token != "123:abc" {
		t.Fatalf("token should be kept for re-enabling, got %q", merged.Channels.Telegram.Token)
	}
}

func TestProviderTypePrefillsSavedProviderName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-prov-name", `{
  "default_provider": "deepseek",
  "providers": {"deepseek": {"type": "openai", "base_url": "https://api.deepseek.com", "api_key": "sk-ds", "default_model": "deepseek-chat"}}
}
`)

	m := initialOnboardModel("agent-prov-name")
	m.step = stepProviderType
	m.choices = m.providerTypeChoices()
	m.cursor = 0
	for i, choice := range m.choices {
		if choice == "OpenAI (or compatible)" {
			m.cursor = i
		}
	}
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.providerName != "deepseek" {
		t.Fatalf("provider name = %q, want the saved name", m.providerName)
	}
	if m.textInput.Value() != "deepseek" {
		t.Fatalf("name input = %q, want it prefilled", m.textInput.Value())
	}

	// Pressing Enter keeps the saved name and moves on to the saved endpoint.
	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.step != stepProviderBaseURL || m.providerBaseURL != "https://api.deepseek.com" {
		t.Fatalf("step = %d baseURL = %q", m.step, m.providerBaseURL)
	}
}

// TestProviderNameMatchesTypeNotKey covers a config where the default provider
// has another type: the wizard still has to find the entry whose type matches.
func TestProviderNameMatchesTypeNotKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-multi-prov", `{
  "default_provider": "gemini",
  "providers": {
    "deepseek": {"type": "openai", "base_url": "https://api.deepseek.com", "api_key": "sk-ds", "default_model": "deepseek-chat"},
    "gemini": {"type": "gemini", "api_key": "g-key", "default_model": "gemini-2.5-flash"}
  }
}
`)

	m := initialOnboardModel("agent-multi-prov")
	if got := m.preferredProviderName("openai"); got != "deepseek" {
		t.Fatalf("openai provider = %q, want deepseek", got)
	}
	if got := m.preferredProviderName("gemini"); got != "gemini" {
		t.Fatalf("gemini provider = %q, want gemini", got)
	}
}

func TestHeadlessProviderNameReusesMatchingType(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-headless-name", `{
  "default_provider": "deepseek",
  "providers": {"deepseek": {"type": "openai", "base_url": "https://api.deepseek.com", "api_key": "sk-ds", "default_model": "deepseek-chat"}}
}
`)

	in, err := headlessInput("agent-headless-name", onboardOptions{provider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	if in.ProviderName != "deepseek" {
		t.Fatalf("provider name = %q, want deepseek", in.ProviderName)
	}
	if in.ProviderAPIKey != "sk-ds" || in.ProviderBaseURL != "https://api.deepseek.com" {
		t.Fatalf("saved provider values were not reused: %+v", in)
	}
}

func TestTemplateProviderKeyIsNotTreatedAsSaved(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-placeholder", `{
  "default_provider": "openai",
  "providers": {"openai": {"type": "openai", "api_key": "YOUR_OPENAI_API_KEY", "default_model": "gpt-4o"}}
}
`)

	m := initialOnboardModel("agent-placeholder")
	if got := m.existingAPIKey("openai"); got != "" {
		t.Fatalf("existingAPIKey = %q, want empty for a template placeholder", got)
	}
	m.providerName = "openai"
	m.step = stepProviderKey
	m.textInput.SetValue("")
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.err == nil {
		t.Fatal("an empty key must be rejected when only a placeholder is saved")
	}
}

func TestProviderReconfigureKeepsSavedKeyAndModel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-key", `{
  "default_provider": "custom",
  "providers": {"custom": {"type": "openai", "base_url": "https://api.deepseek.com", "api_key": "sk-saved", "default_model": "deepseek-chat"}}
}
`)

	m := initialOnboardModel("agent-key")
	m.providerType = "openai"
	m.step = stepProviderName
	m.textInput.SetValue("custom")
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.providerBaseURL != "https://api.deepseek.com" {
		t.Fatalf("base URL = %q, want the saved endpoint", m.providerBaseURL)
	}
	if m.textInput.Value() != "https://api.deepseek.com" {
		t.Fatalf("base URL input = %q, want it prefilled", m.textInput.Value())
	}

	m = next.(onboardModel)
	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.step != stepProviderKey {
		t.Fatalf("step = %d, want stepProviderKey", m.step)
	}
	if m.textInput.Placeholder == "" {
		t.Fatal("expected a keep-saved-key hint as the placeholder")
	}
	m.textInput.SetValue("")
	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.err != nil {
		t.Fatalf("empty key rejected: %v", m.err)
	}
	if m.llmKey != "sk-saved" {
		t.Fatalf("key = %q, want the saved key", m.llmKey)
	}

	// A failed model fetch still offers the current model as the default value.
	m = m.handleModels(modelsFetchedMsg{err: errOffline})
	if m.step != stepProviderModel || m.textInput.Value() != "deepseek-chat" {
		t.Fatalf("model input = %q (step %d), want the saved model prefilled", m.textInput.Value(), m.step)
	}
}

func TestBotTypeCanBeChanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-type", updateControlConfig)

	m := initialOnboardModel("agent-type")
	if m.botType != "private" {
		t.Fatalf("botType = %q, want private", m.botType)
	}

	// The default answer on an update is the explicit keep option.
	m.enterBotTypeStep()
	if m.step != stepBotType || m.choices[m.cursor] != keepAccessOption {
		t.Fatalf("choices = %v cursor = %d, want keep-current first", m.choices, m.cursor)
	}
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.botType != "private" {
		t.Fatalf("keeping access changed botType to %q", m.botType)
	}

	// Then pick Public explicitly.
	m.enterBotTypeStep()
	for i, choice := range m.choices {
		if choice == "Public (anyone can chat)" {
			m.cursor = i
		}
	}
	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.botType != "public" {
		t.Fatalf("botType = %q, want public", m.botType)
	}
	merged := mergeOnboardConfig(loadAgentConfig("agent-type"), m.input())
	if merged.Bot.Type != "public" {
		t.Fatalf("saved bot type = %q", merged.Bot.Type)
	}
	if strings.Contains(m.renderSummary(), "No owner IDs") {
		t.Fatal("owner warning must not appear for a public bot")
	}
}

func TestBotTypeDefaultsToPublicWhenUnset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	// Older configs may omit the type entirely, which the runtime treats as
	// public. The wizard must not silently lock such an agent down.
	writeRawConfig(t, "agent-type-empty", `{"providers": {}}`)

	m := initialOnboardModel("agent-type-empty")
	if m.botType != "public" {
		t.Fatalf("botType = %q, want public for an unset type", m.botType)
	}
}

// TestKeepCurrentLabelsAreConsistent pins the wording rule: update mode always
// offers "Keep current ...", while "Skip" only exists for a brand new agent.
func TestKeepCurrentLabelsAreConsistent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-labels", updateControlConfig)

	update := initialOnboardModel("agent-labels")
	checks := map[string]string{}

	update.step = stepProviderType
	update.choices = update.providerTypeChoices()
	checks["provider"] = update.choices[0]

	update.step = stepSkills
	update.choices = update.skillChoices()
	checks["skills"] = update.choices[0]

	update.step = stepTools
	update.choices = update.toolChoices()
	checks["tools"] = update.choices[0]

	update.enterChannelStep()
	checks["channels"] = update.choices[0]

	update.enterChannelExistingStep()
	checks["existing channels"] = update.choices[0]

	update.enterBotTypeStep()
	checks["access"] = update.choices[0]

	update.setupHeartbeatChannel()
	checks["heartbeat"] = update.choices[len(update.choices)-1]

	for section, label := range checks {
		if !strings.HasPrefix(label, "Keep current") {
			t.Fatalf("%s keep label = %q, want it to start with \"Keep current\"", section, label)
		}
	}

	// A new agent has nothing to keep, so the neutral answer is "Skip".
	fresh := initialOnboardModel("")
	fresh.enterChannelStep()
	if last := fresh.choices[len(fresh.choices)-1]; last != "Skip" {
		t.Fatalf("new agent channel choices = %v, want Skip last", fresh.choices)
	}
	fresh.setupHeartbeatChannel()
	if last := fresh.choices[len(fresh.choices)-1]; last != "Skip" {
		t.Fatalf("new agent heartbeat choices = %v, want Skip last", fresh.choices)
	}
	if len(fresh.botTypeChoices()) != 2 {
		t.Fatalf("new agent access choices = %v, want only private/public", fresh.botTypeChoices())
	}
}

func TestEscFromInitializeReturnsToAgentPicker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-picker", updateControlConfig)

	m := initialOnboardModel("")
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	m.cursor = 0
	for i, choice := range m.choices {
		if choice == "agent-picker" {
			m.cursor = i
		}
	}
	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.step != stepInitHome || !m.agentFromPicker {
		t.Fatalf("step = %d fromPicker = %v", m.step, m.agentFromPicker)
	}

	back := m.previousStep()
	if back.step != stepAgentChoice {
		t.Fatalf("step = %d, want stepAgentChoice", back.step)
	}
	if back.choices[back.cursor] != "agent-picker" {
		t.Fatalf("cursor on %q, want the agent that was picked", back.choices[back.cursor])
	}
}

func TestHelpLineMentionsEsc(t *testing.T) {
	m := initialOnboardModel("")
	m.step = stepProviderKey
	if !strings.Contains(m.helpLine(), "Esc · back") {
		t.Fatalf("help line is missing the back hint: %q", m.helpLine())
	}
	m.step = stepSkills
	m.choices = m.skillChoices()
	if !strings.Contains(m.helpLine(), "Space · toggle") {
		t.Fatalf("multi-select hint is missing: %q", m.helpLine())
	}
	m.step = stepWelcome
	if strings.Contains(m.helpLine(), "Esc · back") {
		t.Fatalf("welcome should offer cancel, not back: %q", m.helpLine())
	}
}

func TestBuiltinCapabilitiesListsEverything(t *testing.T) {
	caps := builtinCapabilities()
	if len(caps.Skills) < 9 {
		t.Fatalf("skills = %d, want at least 9", len(caps.Skills))
	}
	if len(caps.Tools) < 9 {
		t.Fatalf("tools = %d, want at least 9", len(caps.Tools))
	}
	for _, entry := range append(append([]capabilityEntry{}, caps.Skills...), caps.Tools...) {
		if entry.Name == "" {
			t.Fatalf("capability without a name: %+v", entry)
		}
	}
	if caps.Skills[0].Description == "" {
		t.Fatal("skill descriptions were not loaded")
	}
}

var errOffline = &offlineError{}

type offlineError struct{}

func (e *offlineError) Error() string { return "offline" }

func TestHeadlessUpdateWithoutProviderKeepsIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	defer func() { onboardOpts = onboardOptions{} }()

	onboardOpts = onboardOptions{yes: true, provider: "openai", apiKey: "sk-orig", model: "gpt-4o", channel: "skip"}
	if code := runOnboardHeadless("agent-keep-prov"); code != 0 {
		t.Fatalf("setup exit code = %d", code)
	}

	// Update the skills only; the provider must survive untouched.
	onboardOpts = onboardOptions{yes: true, force: true, skills: "time"}
	if code := runOnboardHeadless("agent-keep-prov"); code != 0 {
		t.Fatalf("update exit code = %d", code)
	}

	cfg := loadAgentConfig("agent-keep-prov")
	if cfg.DefaultProvider != "openai" {
		t.Fatalf("default provider = %q", cfg.DefaultProvider)
	}
	if got := cfg.Providers["openai"].APIKey; got != "sk-orig" {
		t.Fatalf("api key = %q, want the original", got)
	}
	skillsDir, err := agentdir.SkillsDir("agent-keep-prov")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "time")); err != nil {
		t.Fatalf("time skill was not installed: %v", err)
	}
}

func TestHeadlessHeartbeatAndChannelSwitches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-headless-off", updateControlConfig)

	in, err := headlessInput("agent-headless-off", onboardOptions{hbChannel: "off", disableChannels: "telegram"})
	if err != nil {
		t.Fatal(err)
	}
	if !in.HeartbeatSet || in.Heartbeat {
		t.Fatalf("heartbeatSet = %v heartbeat = %v", in.HeartbeatSet, in.Heartbeat)
	}
	if !in.DisableTelegram {
		t.Fatal("--disable-channel telegram was not applied")
	}

	merged := mergeOnboardConfig(loadAgentConfig("agent-headless-off"), in)
	if merged.Heartbeat.Active {
		t.Fatal("heartbeat is still active")
	}
	if merged.Channels.Telegram.Active {
		t.Fatal("telegram is still active")
	}
}
