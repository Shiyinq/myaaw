package obs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"myaaw/internal/provider"
)

// Recorder turns a run's events into a span tree and writes it to the store
// from a background writer. Every method is safe for concurrent use, never
// blocks on I/O, and never returns an error to the caller: observability must
// not be able to break a run.
type Recorder struct {
	store  *Store
	pricer *Pricer
	now    func() time.Time

	exporterMu sync.RWMutex
	exporter   *Exporter

	maxChars     int
	storeContent bool

	mu           sync.Mutex
	run          Run
	runDirty     bool
	runSpan      *Span
	spans        []*Span
	byID         map[string]*Span
	toolSpans    map[string]*Span
	dirty        map[string]bool
	step         *Span
	llm          *Span
	llmStarted   time.Time
	firstToken   time.Time
	requestIndex int
	llmCost      float64
	ended        bool

	queueMu  sync.Mutex
	queue    []batch
	finished bool

	errMu    sync.Mutex
	writeErr error

	wake     chan struct{}
	done     chan struct{}
	closeOne sync.Once
	wg       sync.WaitGroup
}

type batch struct {
	run    *Run
	spans  []Span
	export *exportJob
}

type exportJob struct {
	run   Run
	spans []Span
}

// SetExporter attaches an OTLP exporter. Exporting happens on the writer
// goroutine, so a slow collector never delays a run.
func (r *Recorder) SetExporter(exporter *Exporter) {
	if r == nil {
		return
	}
	r.exporterMu.Lock()
	defer r.exporterMu.Unlock()
	r.exporter = exporter
}

// exporterOf returns the attached exporter, if any.
func (r *Recorder) exporterOf() *Exporter {
	r.exporterMu.RLock()
	defer r.exporterMu.RUnlock()
	return r.exporter
}

// New starts a recorder. cfg.MaxContentChars defaults to 8192 when zero.
func New(store *Store, pricer *Pricer, cfg Config) *Recorder {
	maxChars := cfg.MaxContentChars
	if maxChars <= 0 {
		maxChars = defaultMaxContentChars
	}
	if pricer == nil {
		pricer = NewPricer(nil)
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	recorder := &Recorder{
		store:        store,
		pricer:       pricer,
		now:          now,
		maxChars:     maxChars,
		storeContent: cfg.StoreContent,
		byID:         map[string]*Span{},
		toolSpans:    map[string]*Span{},
		dirty:        map[string]bool{},
		wake:         make(chan struct{}, 1),
		done:         make(chan struct{}),
	}
	recorder.wg.Add(1)
	go recorder.writer()
	return recorder
}

// RunID returns the identifier of the run in progress, or "".
func (r *Recorder) RunID() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.run.ID
}

// ToolSpanID returns the span id recorded for a tool call, or "" when the call
// has not been seen. Callers use it to nest a child run under the tool that
// started it.
func (r *Recorder) ToolSpanID(callID string) string {
	if r == nil || callID == "" {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if span, ok := r.toolSpans[callID]; ok {
		return span.ID
	}
	return ""
}

// CurrentSpanID returns the span nested work should attach to: the open step,
// or the run span when no step is open.
func (r *Recorder) CurrentSpanID() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.step != nil {
		return r.step.ID
	}
	return r.run.ID
}

// Store exposes the store so a nested run can write to the same database.
func (r *Recorder) Store() *Store {
	if r == nil {
		return nil
	}
	return r.store
}

// Start opens a run and records the system prompt and user message it began
// with. It returns the run identifier.
func (r *Recorder) Start(start RunStart) string {
	if r == nil || r.store == nil {
		return ""
	}

	now := r.now()
	id := start.RunID
	if id == "" {
		id = NewID("run")
	}
	startedAt := now.UnixMilli()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.run = Run{
		ID:             id,
		StartedAt:      startedAt,
		Channel:        start.Channel,
		UserID:         start.UserID,
		ConversationID: start.ConversationID,
		Provider:       start.Provider,
		Model:          start.Model,
		Agent:          start.Agent,
		ParentRunID:    start.ParentRunID,
		ParentSpanID:   start.ParentSpanID,
		Status:         StatusRunning,
	}
	r.runDirty = true

	r.runSpan = &Span{
		ID:        id,
		RunID:     id,
		ParentID:  start.ParentSpanID,
		Kind:      KindRun,
		Name:      firstNonEmpty(start.Agent, start.Channel, "run"),
		StartedAt: startedAt,
		Status:    StatusRunning,
	}
	r.addSpanLocked(r.runSpan)

	if prompt := start.SystemPrompt; prompt != "" {
		r.markSpanMarkerLocked(KindSystem, "system", prompt, id, startedAt)
	}
	if message := start.UserMessage; message != "" {
		r.markSpanMarkerLocked(KindUser, "user", message, id, startedAt)
	}

	r.enqueueLocked()
	return id
}

// OnEvent applies one trace event.
func (r *Recorder) OnEvent(ev Event) {
	if r == nil || r.store == nil {
		return
	}

	at := ev.Time
	if at.IsZero() {
		at = r.now()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ended {
		return
	}

	switch ev.Type {
	case EventStepStart:
		r.startStepLocked(ev.StepIndex, at)
	case EventLLMRequestStart:
		r.startLLMLocked(ev, at)
	case EventLLMRetry:
		r.retryLLMLocked(ev, at)
	case EventCompaction:
		r.startCompactionLocked(ev, at)
	case EventFirstToken:
		if r.llm != nil && r.firstToken.IsZero() {
			r.firstToken = at
		}
	case EventLLMEnd:
		r.endLLMLocked(ev, at)
	case EventToolStart:
		r.startToolLocked(ev, at)
	case EventToolEnd:
		r.endToolLocked(ev, at)
	case EventStepEnd:
		r.closeStepLocked(at)
	case EventRunEnd:
		r.endRunLocked(ev, at)
	}

	r.enqueueLocked()
}

// Flush waits until everything queued has been written.
func (r *Recorder) Flush(ctx context.Context) error {
	if r == nil {
		return nil
	}
	for {
		r.queueMu.Lock()
		empty := len(r.queue) == 0
		r.queueMu.Unlock()
		if empty {
			return r.err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Millisecond):
		}
	}
}

// Close flushes pending spans and stops the writer. The store is owned by the
// caller and is not closed here, so one store can back many recorders.
func (r *Recorder) Close() error {
	if r == nil {
		return nil
	}
	r.closeOne.Do(func() { close(r.done) })
	r.wg.Wait()
	r.queueMu.Lock()
	r.finished = true
	r.queueMu.Unlock()

	return r.err()
}

func (r *Recorder) startStepLocked(stepIndex int, at time.Time) {
	if r.step != nil && r.step.StepIndex == stepIndex {
		return
	}
	if r.step != nil {
		r.closeStepLocked(at)
	}
	if stepIndex <= 0 {
		stepIndex = 1
	}

	step := &Span{
		ID:        NewID("sp"),
		RunID:     r.run.ID,
		ParentID:  r.run.ID,
		Kind:      KindStep,
		Name:      fmt.Sprintf("Step %d", stepIndex),
		StepIndex: stepIndex,
		StartedAt: at.UnixMilli(),
		Status:    StatusRunning,
	}
	r.addSpanLocked(step)
	r.step = step
}

func (r *Recorder) startLLMLocked(ev Event, at time.Time) {
	if r.step == nil {
		r.startStepLocked(ev.StepIndex, at)
	}
	if r.llm != nil {
		// A request that never reported an end (retry, crash): close it so the
		// next request does not overwrite it.
		r.endLLMLocked(Event{StopReason: provider.StopAborted}, at)
	}

	r.requestIndex++
	llm := &Span{
		ID:           NewID("sp"),
		RunID:        r.run.ID,
		ParentID:     r.step.ID,
		Kind:         KindLLM,
		Name:         ev.Model,
		StepIndex:    r.step.StepIndex,
		RequestIndex: r.requestIndex,
		StartedAt:    at.UnixMilli(),
		Status:       StatusRunning,
		InputJSON:    encodeMessages(ev.Messages, r.maxChars, r.storeContent),
	}
	r.addSpanLocked(llm)
	r.llm = llm
	r.llmStarted = at
	r.firstToken = time.Time{}

	if r.run.Model == "" && ev.Model != "" {
		r.run.Model = ev.Model
		r.runDirty = true
	}
}

func (r *Recorder) retryLLMLocked(ev Event, at time.Time) {
	llm := r.llm
	if llm == nil {
		return
	}

	llm.EndedAt = at.UnixMilli()
	llm.DurationMs = at.Sub(r.llmStarted).Milliseconds()
	if !r.firstToken.IsZero() {
		llm.FirstTokenMs = r.firstToken.Sub(r.llmStarted).Milliseconds()
		llm.GenerationMs = at.Sub(r.firstToken).Milliseconds()
	}
	llm.Status = StatusRetried
	llm.Error = ev.ErrorText
	r.markDirtyLocked(llm.ID)

	// Reuse the stored transcript: the retried request sends the same body, and
	// encoding it again would duplicate a large blob for every attempt.
	r.requestIndex++
	retry := &Span{
		ID:           NewID("sp"),
		RunID:        r.run.ID,
		ParentID:     llm.ParentID,
		Kind:         KindLLM,
		Name:         llm.Name,
		StepIndex:    llm.StepIndex,
		RequestIndex: r.requestIndex,
		StartedAt:    at.UnixMilli(),
		Status:       StatusRunning,
		InputJSON:    llm.InputJSON,
	}
	r.addSpanLocked(retry)
	r.llm = retry
	r.llmStarted = at
	r.firstToken = time.Time{}
}

func (r *Recorder) endLLMLocked(ev Event, at time.Time) {
	llm := r.llm
	if llm == nil {
		return
	}

	llm.EndedAt = at.UnixMilli()
	llm.DurationMs = at.Sub(r.llmStarted).Milliseconds()
	if !r.firstToken.IsZero() {
		llm.FirstTokenMs = r.firstToken.Sub(r.llmStarted).Milliseconds()
		llm.GenerationMs = at.Sub(r.firstToken).Milliseconds()
	}

	llm.InputTokens = ev.Usage.PromptTokens
	llm.OutputTokens = ev.Usage.CompletionTokens
	llm.CacheHitTokens = ev.Usage.CachedInputTokens
	if llm.InputTokens == 0 && ev.Usage.TotalTokens > 0 {
		llm.InputTokens = ev.Usage.TotalTokens - llm.OutputTokens
		if llm.InputTokens < 0 {
			llm.InputTokens = 0
		}
	}
	llm.CostUSD = r.pricer.Cost(llm.Name, ev.Usage, r.llmStarted)
	r.llmCost += llm.CostUSD

	llm.Status = StatusCompleted
	if ev.IsError || ev.StopReason == provider.StopError {
		llm.Status = StatusError
		llm.Error = ev.ErrorText
	} else if ev.StopReason == provider.StopAborted {
		llm.Status = StatusCancelled
		llm.Error = ev.ErrorText
	} else if ev.StopReason == provider.StopLength {
		llm.Status = StatusTruncated
	}

	if r.storeContent {
		llm.OutputJSON = encodeAssistant(ev.Output, ev.Thought, r.maxChars)
	}

	// Keep the run row useful while it is still open: a client that reads the
	// trace mid-run sees spend and duration so far instead of zeros.
	r.run.InputTokens += llm.InputTokens
	r.run.OutputTokens += llm.OutputTokens
	r.run.CacheHitTokens += llm.CacheHitTokens
	r.run.ModelCostUSD = r.llmCost
	r.run.TotalMs = at.Sub(time.UnixMilli(r.run.StartedAt)).Milliseconds()
	if r.run.FirstTokenMs == 0 && llm.FirstTokenMs > 0 {
		r.run.FirstTokenMs = llm.FirstTokenMs
	}
	r.runDirty = true

	r.markDirtyLocked(llm.ID)
	r.llm = nil
	r.firstToken = time.Time{}
}

func (r *Recorder) startCompactionLocked(ev Event, at time.Time) {
	if r.step == nil {
		r.startStepLocked(ev.StepIndex, at)
	}
	span := &Span{
		ID:        NewID("sp"),
		RunID:     r.run.ID,
		ParentID:  r.step.ID,
		Kind:      KindCompaction,
		Name:      "compaction",
		StepIndex: r.step.StepIndex,
		StartedAt: at.UnixMilli(),
		EndedAt:   at.UnixMilli(),
		Status:    StatusCompleted,
		InputJSON: encodeCompaction(ev),
	}
	r.addSpanLocked(span)
}

func (r *Recorder) startToolLocked(ev Event, at time.Time) {
	if r.step == nil {
		r.startStepLocked(ev.StepIndex, at)
	}

	span := &Span{
		ID:           NewID("sp"),
		RunID:        r.run.ID,
		ParentID:     r.step.ID,
		Kind:         KindTool,
		Name:         ev.ToolName,
		StepIndex:    r.step.StepIndex,
		RequestIndex: r.requestIndex,
		StartedAt:    at.UnixMilli(),
		Status:       StatusRunning,
	}
	if r.storeContent {
		span.InputJSON = TruncateContent(ev.ToolArguments, r.maxChars)
	}
	r.addSpanLocked(span)

	if ev.ToolCallID != "" {
		r.toolSpans[ev.ToolCallID] = span
	} else {
		r.toolSpans[span.ID] = span
	}
}

func (r *Recorder) endToolLocked(ev Event, at time.Time) {
	span := r.toolSpans[ev.ToolCallID]
	if span == nil {
		span = r.lastRunningToolLocked()
	}
	if span == nil {
		return
	}
	delete(r.toolSpans, ev.ToolCallID)

	span.EndedAt = at.UnixMilli()
	span.DurationMs = at.Sub(time.UnixMilli(span.StartedAt)).Milliseconds()
	if ev.ToolDurationMs > 0 {
		span.DurationMs = ev.ToolDurationMs
	}
	span.Status = StatusCompleted
	if ev.IsError {
		span.Status = StatusError
		span.Error = ev.ErrorText
	}
	if r.storeContent {
		span.OutputJSON = TruncateContent(ev.ToolOutput, r.maxChars)
	}
	r.markDirtyLocked(span.ID)
}

func (r *Recorder) closeStepLocked(at time.Time) {
	if r.llm != nil {
		r.endLLMLocked(Event{StopReason: provider.StopAborted}, at)
	}
	for key, span := range r.toolSpans {
		span.EndedAt = at.UnixMilli()
		span.DurationMs = at.Sub(time.UnixMilli(span.StartedAt)).Milliseconds()
		span.Status = StatusCancelled
		r.markDirtyLocked(span.ID)
		delete(r.toolSpans, key)
	}

	step := r.step
	if step == nil {
		return
	}
	step.EndedAt = at.UnixMilli()
	step.DurationMs = at.Sub(time.UnixMilli(step.StartedAt)).Milliseconds()
	step.Status = StatusCompleted
	for _, span := range r.spans {
		if span.ParentID != step.ID {
			continue
		}
		if span.Status == StatusError {
			step.Status = StatusError
			step.Error = span.Error
			break
		}
	}
	r.markDirtyLocked(step.ID)
	r.step = nil
}

func (r *Recorder) endRunLocked(ev Event, at time.Time) {
	r.closeStepLocked(at)

	status := ev.Status
	if status == "" {
		var reasonErr error
		if ev.Err != "" {
			reasonErr = fmt.Errorf("%s", ev.Err)
		}
		status = StatusFromStopReason(ev.StopReason, reasonErr)
	}

	r.run.EndedAt = at.UnixMilli()
	r.run.TotalMs = at.Sub(time.UnixMilli(r.run.StartedAt)).Milliseconds()
	if ev.Usage.TotalTokens > 0 || ev.Usage.PromptTokens > 0 {
		r.run.InputTokens = ev.Usage.PromptTokens
		r.run.OutputTokens = ev.Usage.CompletionTokens
		r.run.CacheHitTokens = ev.Usage.CachedInputTokens
	}
	if r.run.InputTokens == 0 && ev.Usage.TotalTokens > 0 {
		r.run.InputTokens = ev.Usage.TotalTokens - r.run.OutputTokens
		if r.run.InputTokens < 0 {
			r.run.InputTokens = 0
		}
	}
	r.run.ModelCostUSD = r.llmCost
	r.run.Status = status
	r.run.Error = ev.Err
	r.run.FirstTokenMs = r.runFirstTokenLocked()
	r.runDirty = true

	if r.runSpan != nil {
		r.runSpan.EndedAt = r.run.EndedAt
		r.runSpan.DurationMs = r.run.TotalMs
		r.runSpan.Status = status
		r.runSpan.Error = r.run.Error
		r.runSpan.FirstTokenMs = r.run.FirstTokenMs
		r.runSpan.InputTokens = r.run.InputTokens
		r.runSpan.OutputTokens = r.run.OutputTokens
		r.runSpan.CacheHitTokens = r.run.CacheHitTokens
		r.runSpan.CostUSD = r.run.ModelCostUSD
		r.markDirtyLocked(r.runSpan.ID)
	}

	r.ended = true

	if exporter := r.exporterOf(); exporter != nil && exporter.Enabled() {
		snapshots := make([]Span, 0, len(r.spans))
		for _, span := range r.spans {
			snapshots = append(snapshots, *span)
		}
		r.enqueue(batch{export: &exportJob{run: r.run, spans: snapshots}})
	}
}

func (r *Recorder) runFirstTokenLocked() int64 {
	for _, span := range r.spans {
		if span.Kind == KindLLM && span.FirstTokenMs > 0 {
			return span.FirstTokenMs
		}
	}
	return 0
}

func (r *Recorder) markSpanMarkerLocked(kind SpanKind, name, content, parentID string, at int64) {
	span := &Span{
		ID:        NewID("sp"),
		RunID:     r.run.ID,
		ParentID:  parentID,
		Kind:      kind,
		Name:      name,
		StartedAt: at,
		EndedAt:   at,
		Status:    StatusCompleted,
	}
	if r.storeContent {
		span.InputJSON = TruncateContent(content, r.maxChars)
	}
	r.addSpanLocked(span)
}

func (r *Recorder) addSpanLocked(span *Span) {
	r.spans = append(r.spans, span)
	r.byID[span.ID] = span
	r.markDirtyLocked(span.ID)
}

func (r *Recorder) markDirtyLocked(id string) {
	r.dirty[id] = true
}

func (r *Recorder) lastRunningToolLocked() *Span {
	var fallback *Span
	for _, span := range r.spans {
		if span.Kind != KindTool || span.Status != StatusRunning {
			continue
		}
		fallback = span
	}
	return fallback
}

func (r *Recorder) enqueueLocked() {
	if len(r.dirty) == 0 && !r.runDirty {
		return
	}

	spans := make([]Span, 0, len(r.dirty))
	for id := range r.dirty {
		if span, ok := r.byID[id]; ok {
			spans = append(spans, *span)
		}
	}
	r.dirty = map[string]bool{}

	b := batch{spans: spans}
	if r.runDirty {
		runCopy := r.run
		b.run = &runCopy
		r.runDirty = false
	}
	r.enqueue(b)
}

func (r *Recorder) enqueue(b batch) {
	r.queueMu.Lock()
	if r.finished {
		r.queueMu.Unlock()
		return
	}
	r.queue = append(r.queue, b)
	r.queueMu.Unlock()

	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Recorder) writer() {
	defer r.wg.Done()
	for {
		select {
		case <-r.done:
			r.drain()
			return
		case <-r.wake:
			r.drain()
		}
	}
}

// drain writes every queued batch while holding the queue lock, so a Flush that
// acquires the lock and sees an empty queue knows everything is on disk.
func (r *Recorder) drain() {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()

	for len(r.queue) > 0 {
		b := r.queue[0]
		r.queue = r.queue[1:]

		if b.run != nil {
			if err := r.store.UpsertRun(*b.run); err != nil {
				r.recordErr(err)
			}
		}
		if len(b.spans) > 0 {
			if err := r.store.UpsertSpans(b.spans); err != nil {
				r.recordErr(err)
			}
		}
		if b.export != nil {
			if exporter := r.exporterOf(); exporter != nil {
				exporter.Export(b.export.run, b.export.spans)
			}
		}
	}
}

func (r *Recorder) recordErr(err error) {
	r.errMu.Lock()
	first := r.writeErr == nil
	if first {
		r.writeErr = err
	}
	r.errMu.Unlock()

	// One line per run, not one per batch: a broken database would otherwise
	// flood the log for the whole run.
	if first {
		log.Printf("[obs] write failed: %v", err)
	}
}

func (r *Recorder) err() error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	return r.writeErr
}

type messageView struct {
	Role       string   `json:"role"`
	Name       string   `json:"name,omitempty"`
	Content    string   `json:"content,omitempty"`
	Thought    string   `json:"thought,omitempty"`
	ToolCallID string   `json:"tool_call_id,omitempty"`
	ToolCalls  []string `json:"tool_calls,omitempty"`
}

type messageEnvelope struct {
	Dropped  int           `json:"dropped_messages,omitempty"`
	Messages []messageView `json:"messages"`
}

// capturePolicy is one attempt at fitting the transcript into the content cap.
// Divisors are relative to the cap; zero keeps that value whole. Escalation
// sacrifices tool output first, then older turns, and only then the system
// prompt, which is the piece most traces are read for.
type capturePolicy struct {
	system         int
	user           int
	assistant      int
	thought        int
	tool           int
	dropThought    bool
	dropToolOutput bool
}

var capturePolicies = []capturePolicy{
	{assistant: 2, thought: 2, tool: 8},
	{user: 2, assistant: 4, thought: 2, tool: 16},
	{user: 4, assistant: 8, dropThought: true, dropToolOutput: true},
	{system: 2, user: 8, assistant: 8, dropThought: true, dropToolOutput: true},
	{system: 4, user: 8, assistant: 16, dropThought: true, dropToolOutput: true},
}

// encodeMessages renders the request transcript, dropping the oldest messages
// until the JSON fits the content cap. The leading system message is never
// dropped, and it is the last thing shortened: a reader has to be able to see
// the prompt that was actually sent.
func encodeMessages(messages []provider.Message, max int, storeContent bool) string {
	if !storeContent || max <= 0 || len(messages) == 0 {
		return ""
	}

	for _, policy := range capturePolicies {
		if encoded, ok := fitMessages(messages, policy, max); ok {
			return encoded
		}
	}
	return `{"messages":[]}`
}

func fitMessages(messages []provider.Message, policy capturePolicy, max int) (string, bool) {
	views := make([]messageView, 0, len(messages))
	sizes := make([]int, 0, len(messages))
	for _, message := range messages {
		view := messageView{
			Role:       message.Role,
			Name:       message.Name,
			ToolCallID: message.ToolCallID,
		}
		text := provider.MessageText(message)
		switch strings.ToLower(message.Role) {
		case "system":
			view.Content = TruncateContent(text, captureLimit(max, policy.system))
		case "tool":
			if !policy.dropToolOutput {
				view.Content = TruncateContent(text, captureLimit(max, policy.tool))
			}
		case "assistant":
			view.Content = TruncateContent(text, captureLimit(max, policy.assistant))
			if !policy.dropThought {
				view.Thought = TruncateContent(message.Thought, captureLimit(max, policy.thought))
			}
		default:
			view.Content = TruncateContent(text, captureLimit(max, policy.user))
		}
		for _, call := range message.ToolCalls {
			view.ToolCalls = append(view.ToolCalls,
				fmt.Sprintf("%s(%s)", call.Function.Name, argsText(call.Function.Arguments)))
		}
		encoded, err := json.Marshal(view)
		if err != nil {
			continue
		}
		views = append(views, view)
		sizes = append(sizes, len(encoded)+1)
	}

	total := 32
	for _, size := range sizes {
		total += size
	}

	// The system message sits at position 0 in every request; trimming starts
	// after it so the snapshot keeps the same shape as what was sent.
	protected := 0
	if len(views) > 0 && strings.EqualFold(views[0].Role, "system") {
		protected = 1
	}

	start := protected
	for total > max && start < len(views)-1 {
		total -= sizes[start]
		start++
	}

	kept := make([]messageView, 0, protected+len(views)-start)
	kept = append(kept, views[:protected]...)
	kept = append(kept, views[start:]...)

	envelope := messageEnvelope{Dropped: start - protected, Messages: kept}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", false
	}
	if len(encoded) <= max {
		return string(encoded), true
	}
	return "", false
}

// captureLimit turns a policy divisor into a byte cap. Zero keeps the value
// whole, which is how the system prompt survives the first policies.
func captureLimit(max, divisor int) int {
	if divisor <= 0 {
		return 0
	}
	if limit := max / divisor; limit > 0 {
		return limit
	}
	return 1
}

func encodeCompaction(ev Event) string {
	// The numbers are not content, so they are kept even when capture is off:
	// they explain why the model stopped seeing older turns.
	view := struct {
		TokensBefore    int `json:"tokens_before"`
		TokensAfter     int `json:"tokens_after"`
		Budget          int `json:"budget,omitempty"`
		DroppedMessages int `json:"messages_dropped"`
		DroppedTokens   int `json:"tokens_dropped"`
	}{
		TokensBefore:    ev.TokensBefore,
		TokensAfter:     ev.TokensAfter,
		Budget:          ev.Budget,
		DroppedMessages: ev.DroppedMessages,
		DroppedTokens:   ev.DroppedTokens,
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func encodeAssistant(output, thought string, max int) string {
	if max <= 0 {
		return ""
	}
	view := messageView{
		Role:    "assistant",
		Content: TruncateContent(output, max/2),
		Thought: TruncateContent(thought, max/2),
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		return ""
	}
	if len(encoded) <= max {
		return string(encoded)
	}

	view.Thought = ""
	encoded, err = json.Marshal(view)
	if err != nil {
		return ""
	}
	if len(encoded) <= max {
		return string(encoded)
	}

	view.Content = ""
	encoded, err = json.Marshal(view)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func argsText(value interface{}) string {
	if text, ok := value.(string); ok {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
