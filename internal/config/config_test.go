package config

import "testing"

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
