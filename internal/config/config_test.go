package config

import "testing"

func TestValidateAgentConfigClampsObservability(t *testing.T) {
	originalRetention, originalChars := ObsRetentionDays, ObsMaxContentChars
	t.Cleanup(func() {
		ObsRetentionDays = originalRetention
		ObsMaxContentChars = originalChars
	})

	ObsRetentionDays = -5
	ObsMaxContentChars = 0
	validateAgentConfig()
	if ObsRetentionDays != 0 {
		t.Fatalf("retention days = %d, want 0", ObsRetentionDays)
	}
	if ObsMaxContentChars != 16384 {
		t.Fatalf("max content chars = %d, want the default 16384", ObsMaxContentChars)
	}

	ObsRetentionDays = 7
	ObsMaxContentChars = 2048
	validateAgentConfig()
	if ObsRetentionDays != 7 || ObsMaxContentChars != 2048 {
		t.Fatalf("observability = %d/%d, want 7/2048 left untouched", ObsRetentionDays, ObsMaxContentChars)
	}
}

func TestEffectiveObservabilityDefaults(t *testing.T) {
	originalEnabled, originalStore := ObsEnabled, ObsStoreContent
	originalRetention, originalChars := ObsRetentionDays, ObsMaxContentChars
	t.Cleanup(func() {
		ObsEnabled, ObsStoreContent = originalEnabled, originalStore
		ObsRetentionDays, ObsMaxContentChars = originalRetention, originalChars
	})

	ObsEnabled = true
	ObsStoreContent = false
	ObsRetentionDays = 30
	ObsMaxContentChars = 16384

	settings := EffectiveObservability()
	if settings.Enabled == nil || !*settings.Enabled {
		t.Fatal("observability should be enabled")
	}
	if settings.StoreContent == nil || *settings.StoreContent {
		t.Fatal("store_content should be false")
	}
	if settings.RetentionDays != 30 || settings.MaxContentChars != 16384 {
		t.Fatalf("settings = %+v", settings)
	}
}

func TestValidateAgentConfigClampsQueueWorkers(t *testing.T) {
	original := QueueWorkers
	t.Cleanup(func() { QueueWorkers = original })

	QueueWorkers = 0
	validateAgentConfig()
	if QueueWorkers != 1 {
		t.Fatalf("queue workers = %d, want the minimum of 1", QueueWorkers)
	}

	QueueWorkers = MaxQueueWorkers + 5
	validateAgentConfig()
	if QueueWorkers != MaxQueueWorkers {
		t.Fatalf("queue workers = %d, want the cap of %d", QueueWorkers, MaxQueueWorkers)
	}

	QueueWorkers = 3
	validateAgentConfig()
	if QueueWorkers != 3 {
		t.Fatalf("queue workers = %d, want 3 left untouched", QueueWorkers)
	}
}
