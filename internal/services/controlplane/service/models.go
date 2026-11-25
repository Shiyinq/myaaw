package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/auth/chatgpt"
	"myaaw/internal/config"
	"myaaw/internal/onboard"
	"myaaw/internal/provider"
)

type cachedModelList struct {
	fetchedAt time.Time
	view      ModelListView
}

type modelListInflight struct {
	done chan struct{}
	view ModelListView
	err  error
}

var (
	providerModelsCacheMu sync.Mutex
	providerModelsCache   = make(map[string]cachedModelList)
	providerModelsRunning = make(map[string]*modelListInflight)
	providerModelsTTL     = 5 * time.Minute
)

func cloneModelListView(v ModelListView) ModelListView {
	out := v
	if v.Models != nil {
		out.Models = append([]string(nil), v.Models...)
	}
	return out
}

func providerModelsCacheKey(providerType, baseURL, authMode, credentialID, apiKey string) string {
	root, _ := agentdir.MyaawRoot()
	sum := sha256.Sum256([]byte(strings.Join([]string{
		root,
		strings.ToLower(strings.TrimSpace(providerType)),
		strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		strings.ToLower(strings.TrimSpace(authMode)),
		strings.TrimSpace(credentialID),
		strings.TrimSpace(apiKey),
	}, "\x00")))
	return hex.EncodeToString(sum[:16])
}

// InvalidateProviderModelsCache clears cached live provider model lists.
func InvalidateProviderModelsCache() {
	providerModelsCacheMu.Lock()
	providerModelsCache = make(map[string]cachedModelList)
	providerModelsCacheMu.Unlock()
}

func fetchModelsCached(cacheKey string, refresh bool, fn func() (ModelListView, error)) (ModelListView, error) {
	providerModelsCacheMu.Lock()
	if !refresh {
		if entry, ok := providerModelsCache[cacheKey]; ok && time.Since(entry.fetchedAt) < providerModelsTTL {
			view := cloneModelListView(entry.view)
			providerModelsCacheMu.Unlock()
			return view, nil
		}
	}
	if inflight, ok := providerModelsRunning[cacheKey]; ok {
		providerModelsCacheMu.Unlock()
		<-inflight.done
		return cloneModelListView(inflight.view), inflight.err
	}
	inflight := &modelListInflight{done: make(chan struct{})}
	providerModelsRunning[cacheKey] = inflight
	providerModelsCacheMu.Unlock()

	view, err := fn()
	providerModelsCacheMu.Lock()
	if err == nil && view.Source == "live" && len(view.Models) > 0 && view.Warning == "" {
		providerModelsCache[cacheKey] = cachedModelList{
			fetchedAt: time.Now(),
			view:      cloneModelListView(view),
		}
	}
	inflight.view = cloneModelListView(view)
	inflight.err = err
	delete(providerModelsRunning, cacheKey)
	close(inflight.done)
	providerModelsCacheMu.Unlock()
	return view, err
}

// ModelListView is the response for the model discovery endpoints.
type ModelListView struct {
	Models  []string `json:"models"`
	Source  string   `json:"source" example:"live"` // "live" or "default"
	Warning string   `json:"warning,omitempty"`
}

// ModelLookupInput lets a client list models before the provider is saved, so
// the create-agent wizard can fill the model picker before calling onboard.
type ModelLookupInput struct {
	Type         string `json:"type" example:"openai"`
	BaseURL      string `json:"base_url,omitempty" example:"https://api.deepseek.com"`
	APIKey       string `json:"api_key,omitempty"`
	AuthMode     string `json:"auth_mode,omitempty"`
	CredentialID string `json:"credential_id,omitempty"`
}

// ModelsForCredentials lists models for a provider that is not saved yet.
func (s *ConfigService) ModelsForCredentials(in ModelLookupInput) (ModelListView, error) {
	return s.ModelsForCredentialsWithRefresh(in, false)
}

// ModelsForCredentialsWithRefresh lists models for an unsaved provider, using
// the 5-minute singleflight cache unless refresh is true.
func (s *ConfigService) ModelsForCredentialsWithRefresh(in ModelLookupInput, refresh bool) (ModelListView, error) {
	providerType := strings.ToLower(strings.TrimSpace(in.Type))
	if providerType == chatgpt.ProviderType {
		return lookupChatGPTModels(config.ProviderConfig{Type: providerType, BaseURL: in.BaseURL, APIKey: in.APIKey, AuthMode: in.AuthMode, CredentialID: in.CredentialID}, refresh)
	}
	if providerType == "" || !provider.IsKnownLLMProvider(providerType) {
		return ModelListView{}, fmt.Errorf("%w: unknown provider type %q", ErrInvalidConfig, in.Type)
	}
	return lookupModels(providerType, in.BaseURL, in.APIKey, refresh), nil
}

// ProviderModels lists models for an already saved provider, using its stored
// type, base URL, and API key.
func (s *ConfigService) ProviderModels(agentName, id string) (ModelListView, error) {
	return s.ProviderModelsWithRefresh(agentName, id, false)
}

// ProviderModelsWithRefresh lists models for a saved provider, serving cached
// live lists across requests and coalescing concurrent lookups unless refresh is true.
func (s *ConfigService) ProviderModelsWithRefresh(agentName, id string, refresh bool) (ModelListView, error) {
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return ModelListView{}, err
	}
	saved, ok := cfg.Providers[id]
	if !ok {
		return ModelListView{}, ErrProviderNotFound
	}
	if saved.Type == chatgpt.ProviderType {
		return lookupChatGPTModels(saved, refresh)
	}
	apiKey := saved.APIKey
	if onboard.IsTemplateValue(apiKey) {
		apiKey = ""
	}
	return lookupModels(strings.ToLower(saved.Type), saved.BaseURL, apiKey, refresh), nil
}

// WarmupProviderModels pre-warms live provider model lists concurrently in the
// background on control-plane startup so the first model picker open is instant.
func (s *ConfigService) WarmupProviderModels(ctx context.Context) {
	if s == nil {
		return
	}
	go func() {
		names, err := agentdir.ListAgents()
		if err != nil || len(names) == 0 {
			return
		}
		seen := make(map[string]config.ProviderConfig)
		for _, name := range names {
			if ctx.Err() != nil {
				return
			}
			cfg, err := loadAgentConfig(name)
			if err != nil || cfg == nil {
				continue
			}
			// Skip external CLI agents (they use runtime options, not native providers).
			if rt := config.MergeRuntime(cfg.Runtime); rt.IsExternal() {
				continue
			}
			for _, p := range cfg.Providers {
				apiKey := p.APIKey
				if onboard.IsTemplateValue(apiKey) {
					apiKey = ""
				}
				key := providerModelsCacheKey(p.Type, p.BaseURL, p.AuthMode, p.CredentialID, apiKey)
				if _, exists := seen[key]; !exists {
					pCopy := p
					pCopy.APIKey = apiKey
					seen[key] = pCopy
				}
			}
		}
		var wg sync.WaitGroup
		for _, p := range seen {
			if ctx.Err() != nil {
				break
			}
			prov := p
			wg.Add(1)
			go func() {
				defer wg.Done()
				if strings.EqualFold(strings.TrimSpace(prov.Type), chatgpt.ProviderType) {
					_, _ = lookupChatGPTModels(prov, false)
					return
				}
				_ = lookupModels(strings.ToLower(strings.TrimSpace(prov.Type)), prov.BaseURL, prov.APIKey, false)
			}()
		}
		wg.Wait()
	}()
}

func lookupChatGPTModels(cfg config.ProviderConfig, refresh bool) (ModelListView, error) {
	if err := onboard.NormalizeProviderAuth(&cfg); err != nil {
		return ModelListView{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	key := providerModelsCacheKey(cfg.Type, cfg.BaseURL, cfg.AuthMode, cfg.CredentialID, "")
	return fetchModelsCached(key, refresh, func() (ModelListView, error) {
		models, err := provider.ModelsForConfig(cfg)
		if err != nil {
			return ModelListView{}, fmt.Errorf("%w: ChatGPT Subscription model discovery failed; check the connection and plan permissions", ErrInvalidConfig)
		}
		if models == nil {
			models = []string{}
		}
		return ModelListView{Models: models, Source: "live"}, nil
	})
}

// lookupModels tries the live provider list and falls back to the built-in
// default model so the wizard can always continue.
func lookupModels(providerType, baseURL, apiKey string, refresh bool) ModelListView {
	key := providerModelsCacheKey(providerType, baseURL, "", "", apiKey)
	view, _ := fetchModelsCached(key, refresh, func() (ModelListView, error) {
		models, err := provider.ModelsFor(providerType, baseURL, apiKey)
		if err == nil && len(models) > 0 {
			return ModelListView{Models: models, Source: "live"}, nil
		}

		fallbackView := ModelListView{Source: "default"}
		if err != nil {
			fallbackView.Warning = err.Error()
		}
		if fallback := provider.DefaultModel(providerType); fallback != "" {
			fallbackView.Models = []string{fallback}
		} else {
			fallbackView.Models = []string{}
		}
		return fallbackView, nil
	})
	return view
}
