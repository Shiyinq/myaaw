package service

import (
	"context"
	"log"

	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/obs"
	"myaaw/internal/obs/agenttrace"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
)

// observability lazily opens the per-agent trace database. Opening is deferred
// so commands and tests that never run the agent do not create a database.
func (r *BotServiceImpl) observability() (*obs.Store, *obs.Pricer, obs.Config) {
	r.obsOnce.Do(func() {
		settings := config.EffectiveObservability()
		if settings.Enabled == nil || !*settings.Enabled {
			return
		}

		path, err := agentdir.ObservabilityPath(activeAgentName())
		if err != nil {
			log.Printf("⚠️  Observability disabled: %v", err)
			return
		}

		retentionDays := 0
		if settings.RetentionDays != nil {
			retentionDays = *settings.RetentionDays
		}
		store, err := obs.Open(path, obs.Options{RetentionDays: retentionDays})
		if err != nil {
			log.Printf("⚠️  Observability disabled: %v", err)
			return
		}

		exporter, err := obs.NewExporter(context.Background())
		if err != nil {
			log.Printf("⚠️  OTLP export disabled: %v", err)
		} else if exporter.Enabled() {
			log.Printf("📡 OTLP trace export is enabled")
		}

		r.obsStore = store
		r.obsExporter = exporter
		r.obsPricer = obs.NewPricer(obsPrices(settings.ModelPrices))
		r.obsConfig = obs.Config{
			MaxContentChars: settings.MaxContentChars,
			StoreContent:    settings.StoreContent == nil || *settings.StoreContent,
		}
	})
	return r.obsStore, r.obsPricer, r.obsConfig
}

// traceRun starts a trace for one inbound message. It returns nil when
// observability is off.
func (r *BotServiceImpl) traceRun(
	msg *channel.IncomingMessage,
	user *model.User,
	messages []provider.Message,
) *agenttrace.Trace {
	// Heartbeats are internal liveness runs; keeping them out of the trace keeps
	// the summary about real user work.
	if msg.TriggerType == "heartbeat" {
		return nil
	}

	store, pricer, cfg := r.observability()
	if store == nil {
		return nil
	}

	trace := agenttrace.Start(store, pricer, cfg, obs.RunStart{
		RunID:          obs.NewID("run"),
		Channel:        msg.Channel,
		UserID:         user.Id,
		ConversationID: msg.ConversationID,
		Provider:       config.LLMProviderName,
		Model:          user.Model,
		Agent:          activeAgentName(),
		UserMessage:    msg.Text,
		SystemPrompt:   agenttrace.SystemPrompt(messages),
	})
	trace.SetExporter(r.obsExporter)
	return trace
}

// obsPrices converts configured prices into the pricing table type.
func obsPrices(prices map[string]config.ModelPriceConfig) map[string]obs.Price {
	if len(prices) == 0 {
		return nil
	}
	converted := make(map[string]obs.Price, len(prices))
	for model, price := range prices {
		converted[model] = obsPrice(price)
	}
	return converted
}

func obsPrice(price config.ModelPriceConfig) obs.Price {
	result := obs.Price{
		Input:       price.Input,
		CacheHit:    price.CacheHit,
		Output:      price.Output,
		PeakWindows: price.PeakWindows,
	}
	if price.OffPeak != nil {
		offPeak := obsPrice(*price.OffPeak)
		result.OffPeak = &offPeak
	}
	return result
}

func activeAgentName() string {
	return agentdir.Active()
}
