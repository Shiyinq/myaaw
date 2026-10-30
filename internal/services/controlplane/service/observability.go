package service

import (
	"errors"
	"os"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/obs"
)

// ErrObservabilityNotFound reports that a run id is unknown.
var ErrObservabilityNotFound = errors.New("run not found")

// ObservabilityService exposes recorded traces for one agent.
type ObservabilityService struct{}

func NewObservabilityService() *ObservabilityService {
	return &ObservabilityService{}
}

// SessionTurn is one turn of a session, with its step count and, when
// requested, the full span list the trajectory UI renders.
type SessionTurn struct {
	obs.Run
	Steps int        `json:"steps"`
	Spans []obs.Span `json:"spans,omitempty"`
}

// SessionDetail is one session with every turn in order, oldest first.
type SessionDetail struct {
	ConversationID string        `json:"conversation_id"`
	Channel        string        `json:"channel,omitempty"`
	Turns          int           `json:"turns"`
	StartedAt      int64         `json:"started_at"`
	LastActiveAt   int64         `json:"last_active_at"`
	InputTokens    int           `json:"input_tokens"`
	OutputTokens   int           `json:"output_tokens"`
	CostUSD        float64       `json:"cost_usd"`
	ErrorCount     int           `json:"error_count"`
	Runs           []SessionTurn `json:"runs"`
}

// RunDetail is one run with its spans, flat and nested.
type RunDetail struct {
	Run   obs.Run    `json:"run"`
	Spans []obs.Span `json:"spans"`
	Tree  *SpanNode  `json:"tree,omitempty"`
}

// RunList is one page of recorded runs.
type RunList struct {
	Runs   []obs.Run `json:"runs"`
	Total  int       `json:"total"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}

// SpanNode nests a span under its parent.
type SpanNode struct {
	obs.Span
	Children []*SpanNode `json:"children,omitempty"`
}

// Summary aggregates the last `days` days. It returns an empty summary instead
// of an error when the agent has never recorded anything.
func (s *ObservabilityService) Summary(agentName string, days int) (obs.Summary, error) {
	store, err := s.open(agentName)
	if err != nil {
		return obs.Summary{}, err
	}
	if store == nil {
		now := time.Now()
		return obs.Summary{
			From:    now.AddDate(0, 0, -clampDays(days)).UnixMilli(),
			To:      now.UnixMilli(),
			PerStep: []obs.StepStat{},
			ByModel: []obs.ModelStat{},
			PerTool: []obs.ToolStat{},
		}, nil
	}
	defer store.Close()

	now := time.Now()
	to := now.UnixMilli()
	from := now.AddDate(0, 0, -clampDays(days)).UnixMilli()
	return store.Summary(from, to)
}

// Runs lists recorded runs newest first.
func (s *ObservabilityService) Runs(agentName string, filter obs.RunFilter) ([]obs.Run, int, error) {
	store, err := s.open(agentName)
	if err != nil {
		return nil, 0, err
	}
	if store == nil {
		return []obs.Run{}, 0, nil
	}
	defer store.Close()

	return store.ListRuns(filter)
}

// Sessions groups recorded runs by conversation, newest activity first.
func (s *ObservabilityService) Sessions(agentName string, limit int) ([]obs.Session, error) {
	store, err := s.open(agentName)
	if err != nil {
		return nil, err
	}
	if store == nil {
		return []obs.Session{}, nil
	}
	defer store.Close()

	return store.ListSessions(limit)
}

// Session returns one session with all of its turns, oldest first. When
// includeSpans is set, every turn also carries its spans so a client can render
// the whole trajectory in one request.
func (s *ObservabilityService) Session(agentName, conversationID string, includeSpans bool) (SessionDetail, error) {
	if conversationID == "" {
		return SessionDetail{}, ErrObservabilityNotFound
	}

	store, err := s.open(agentName)
	if err != nil {
		return SessionDetail{}, err
	}
	if store == nil {
		return SessionDetail{}, ErrObservabilityNotFound
	}
	defer store.Close()

	runs, _, err := store.ListRuns(obs.RunFilter{ConversationID: conversationID, Limit: 500})
	if err != nil {
		return SessionDetail{}, err
	}
	if len(runs) == 0 {
		return SessionDetail{}, ErrObservabilityNotFound
	}

	// ListRuns is newest first; a session reads in turn order.
	for left, right := 0, len(runs)-1; left < right; left, right = left+1, right-1 {
		runs[left], runs[right] = runs[right], runs[left]
	}

	ids := make([]string, len(runs))
	for i, run := range runs {
		ids[i] = run.ID
	}
	steps, err := store.StepCounts(ids)
	if err != nil {
		return SessionDetail{}, err
	}

	spans := map[string][]obs.Span{}
	if includeSpans {
		spans, err = store.SpansForRuns(ids)
		if err != nil {
			return SessionDetail{}, err
		}
	}

	detail := SessionDetail{
		ConversationID: conversationID,
		Turns:          len(runs),
		StartedAt:      runs[0].StartedAt,
		Runs:           make([]SessionTurn, 0, len(runs)),
	}
	for _, run := range runs {
		detail.LastActiveAt = run.StartedAt
		detail.InputTokens += run.InputTokens
		detail.OutputTokens += run.OutputTokens
		detail.CostUSD += run.ModelCostUSD
		if run.Status == obs.StatusError || run.Status == obs.StatusInterrupted {
			detail.ErrorCount++
		}
		if run.Channel != "" {
			detail.Channel = run.Channel
		}
		detail.Runs = append(detail.Runs, SessionTurn{
			Run:   run,
			Steps: steps[run.ID],
			Spans: spans[run.ID],
		})
	}
	return detail, nil
}

// Run returns one run and its span tree.
func (s *ObservabilityService) Run(agentName, runID string) (RunDetail, error) {
	store, err := s.open(agentName)
	if err != nil {
		return RunDetail{}, err
	}
	if store == nil {
		return RunDetail{}, ErrObservabilityNotFound
	}
	defer store.Close()

	run, spans, err := store.GetRun(runID)
	if errors.Is(err, obs.ErrNotFound) {
		return RunDetail{}, ErrObservabilityNotFound
	}
	if err != nil {
		return RunDetail{}, err
	}
	return RunDetail{Run: run, Spans: spans, Tree: buildTree(run, spans)}, nil
}

// DeleteBefore removes runs that started before the given time.
func (s *ObservabilityService) DeleteBefore(agentName string, before time.Time) (int64, error) {
	store, err := s.open(agentName)
	if err != nil {
		return 0, err
	}
	if store == nil {
		return 0, nil
	}
	defer store.Close()

	return store.DeleteBefore(before.UnixMilli())
}

// open returns a store, or nil when the agent has no trace database yet. It
// never creates the file from a read path.
func (s *ObservabilityService) open(agentName string) (*obs.Store, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return nil, err
	}
	path, err := agentdir.ObservabilityPath(agentName)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return obs.Open(path, obs.Options{})
}

func clampDays(days int) int {
	if days <= 0 {
		return 7
	}
	if days > 365 {
		return 365
	}
	return days
}

// buildTree nests spans under their parent. A span whose parent is missing
// stays a root, so a partial trace still renders.
func buildTree(run obs.Run, spans []obs.Span) *SpanNode {
	nodes := make(map[string]*SpanNode, len(spans))
	order := make([]*SpanNode, 0, len(spans))
	for _, span := range spans {
		node := &SpanNode{Span: span}
		nodes[span.ID] = node
		order = append(order, node)
	}

	root := &SpanNode{Span: obs.Span{
		ID:           run.ID,
		RunID:        run.ID,
		Kind:         obs.KindRun,
		Name:         run.Agent,
		StartedAt:    run.StartedAt,
		EndedAt:      run.EndedAt,
		DurationMs:   run.TotalMs,
		Status:       run.Status,
		Error:        run.Error,
		InputTokens:  run.InputTokens,
		OutputTokens: run.OutputTokens,
		CostUSD:      run.ModelCostUSD,
		FirstTokenMs: run.FirstTokenMs,
	}}

	for _, node := range order {
		if node.ID == run.ID {
			// The synthetic root below already represents the run span.
			continue
		}
		switch {
		case node.ParentID == "" || node.ParentID == run.ID:
			root.Children = append(root.Children, node)
		case nodes[node.ParentID] != nil:
			parent := nodes[node.ParentID]
			parent.Children = append(parent.Children, node)
		default:
			root.Children = append(root.Children, node)
		}
	}
	return root
}

// SessionList is one page of sessions.
type SessionList struct {
	Sessions []obs.Session `json:"sessions"`
}
