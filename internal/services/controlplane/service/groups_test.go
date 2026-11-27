package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"myaaw/internal/channel"
)

func newTestGroupStore(t *testing.T) *GroupStore {
	t.Helper()
	store, err := NewGroupStoreAt(filepath.Join(t.TempDir(), "controlplane.db"))
	if err != nil {
		t.Fatalf("NewGroupStoreAt: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

type mockAgentDirectory struct {
	mu     sync.RWMutex
	agents map[string]AgentInfo
}

func newMockAgentDirectory(agents ...AgentInfo) *mockAgentDirectory {
	m := &mockAgentDirectory{agents: make(map[string]AgentInfo)}
	for _, a := range agents {
		m.agents[a.Name] = a
	}
	return m
}

func (m *mockAgentDirectory) Get(name string) (AgentInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info, ok := m.agents[name]
	if !ok {
		return AgentInfo{}, ErrAgentNotFound
	}
	return info, nil
}

func (m *mockAgentDirectory) BaseURL(name string) (string, int, error) {
	info, err := m.Get(name)
	if err != nil {
		return "", 0, err
	}
	return info.BaseURL, info.Port, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func sseHTTPResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func jsonHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func waitForRunEnd(t *testing.T, ch <-chan GroupSSEEvent, timeout time.Duration) ([]GroupSSEEvent, map[string]any) {
	t.Helper()
	var collected []GroupSSEEvent
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("event channel closed before run_end; got %d events", len(collected))
			}
			collected = append(collected, ev)
			if ev.Name == "run_end" {
				data, _ := ev.Data.(map[string]any)
				return collected, data
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for run_end; collected %d events: %+v", len(collected), collected)
		}
	}
}

func TestGroupStoreValidationAndCRUD(t *testing.T) {
	store := newTestGroupStore(t)

	for _, bad := range []string{"", "../etc/passwd", "a/b", "bad id", strings.Repeat("a", 65)} {
		if err := ValidateGroupID(bad); err == nil {
			t.Fatalf("ValidateGroupID(%q) = nil, want error", bad)
		}
		if _, err := store.Get(bad); err != ErrInvalidGroupID {
			t.Fatalf("store.Get(%q) = %v, want ErrInvalidGroupID", bad, err)
		}
	}

	if _, err := store.Get("missing-group"); err != ErrGroupNotFound {
		t.Fatalf("store.Get(missing) = %v, want ErrGroupNotFound", err)
	}

	sg := &storedGroup{
		Group: Group{
			ID:             "grp-1",
			Name:           "Council",
			Members:        []GroupMember{{Name: "alice"}},
			DiscussionMode: DiscussionModeAutonomous,
			MaxTurns:       8,
			CreatedAt:      "2026-11-21T07:00:00Z",
			UpdatedAt:      "2026-11-21T07:00:00Z",
		},
		Messages: []GroupMessage{},
	}
	if err := store.Create(sg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	loaded, err := store.Get("grp-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.Group.Name != "Council" || len(loaded.Group.Members) != 1 || loaded.Messages == nil {
		t.Fatalf("unexpected loaded group: %+v", loaded)
	}

	if err := store.Delete("grp-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete("grp-1"); err != ErrGroupNotFound {
		t.Fatalf("second Delete = %v, want ErrGroupNotFound", err)
	}
}

func TestExtractMentionsAndDoneSignal(t *testing.T) {
	members := []GroupMember{
		{Name: "alice"},
		{Name: "bob_dev"},
		{Name: "charlie-qa"},
	}

	text := "Hey @Alice, ignore `@bob_dev` and ```\n@charlie-qa\n``` but ping (@bob_dev) and @alice again."
	got, hasAll := extractMentions(text, members)
	if hasAll {
		t.Fatalf("hasAll = true, want false")
	}
	if len(got) != 2 || got[0] != "alice" || got[1] != "bob_dev" {
		t.Fatalf("mentions = %v, want [alice bob_dev]", got)
	}

	_, hasAll = extractMentions("Let's ask @ALL to weigh in.", members)
	if !hasAll {
		t.Fatalf("expected @ALL to set hasAll=true")
	}

	// Markdown-wrapped and quoted mentions match; emails and URLs stay ignored.
	mdText := `Please check **@alice**, "@bob_dev", —@charlie-qa; ignore a@b.com, mail@alice.dev, and https://example.com/@alice.`
	mdGot, mdAll := extractMentions(mdText, members)
	if mdAll || len(mdGot) != 3 || mdGot[0] != "alice" || mdGot[1] != "bob_dev" || mdGot[2] != "charlie-qa" {
		t.Fatalf("markdown mentions = (%v, %v), want [alice bob_dev charlie-qa], false", mdGot, mdAll)
	}
	emailOnly, _ := extractMentions("Email a@b.com or contact@alice.com or https://x.com/@alice", members)
	if len(emailOnly) != 0 {
		t.Fatalf("email/URL should not match mentions, got %v", emailOnly)
	}

	cleaned, done := extractDoneSignal("Everything looks great! [DONE]")
	if !done || cleaned != "Everything looks great!" {
		t.Fatalf("extractDoneSignal = (%q, %v)", cleaned, done)
	}

	cleaned, done = extractDoneSignal("All set [DONE].")
	if !done || cleaned != "All set" {
		t.Fatalf("extractDoneSignal with trailing period = (%q, %v)", cleaned, done)
	}

	cleaned, done = extractDoneSignal("Use `[DONE]` in docs. [DONE].")
	if !done || cleaned != "Use `[DONE]` in docs." {
		t.Fatalf("extractDoneSignal mixed backtick + real = (%q, %v)", cleaned, done)
	}

	cleaned, done = extractDoneSignal("Use `[DONE]` in docs without stopping.")
	if done || !strings.Contains(cleaned, "`[DONE]`") {
		t.Fatalf("code-wrapped [DONE] should not trigger done: (%q, %v)", cleaned, done)
	}
}

func TestGroupAutonomousHandoffDeltaReplyQuotesAndSessionReuse(t *testing.T) {
	dir := newMockAgentDirectory(
		AgentInfo{Name: "alice", AgentColor: "bleu", Running: true, BaseURL: "http://agent-alice"},
		AgentInfo{Name: "bob", AgentColor: "vert", Running: true, BaseURL: "http://agent-bob"},
	)

	var (
		mu       sync.Mutex
		requests []struct {
			Host string
			Body agentChatStreamRequest
		}
	)

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var body agentChatStreamRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				return jsonHTTPResponse(http.StatusBadRequest, `{"error":"bad json"}`), nil
			}
			mu.Lock()
			requests = append(requests, struct {
				Host string
				Body agentChatStreamRequest
			}{Host: r.URL.Host, Body: body})
			callNum := len(requests)
			mu.Unlock()

			switch r.URL.Host {
			case "agent-alice":
				if callNum == 1 {
					sse := strings.Join([]string{
						`event: conversation`,
						`data: {"conversation_id":"conv-alice-1"}`,
						``,
						`event: thought`,
						`data: {"text":"Checking design"}`,
						``,
						`event: tool`,
						`data: {"id":"call_1","name":"read_file","arguments":"{\"path\":\"a.go\"}"}`,
						``,
						`event: tool_result`,
						`data: {"id":"call_1","name":"read_file","result":"package main","is_error":false}`,
						``,
						`event: chunk`,
						`data: {"text":"Looks good, @bob please review."}`,
						``,
						`event: done`,
						`data: {"ok":true,"text":"Looks good, @bob please review.","conversation_id":"conv-alice-1"}`,
						``,
					}, "\n")
					return sseHTTPResponse(sse), nil
				}
				// Second user message reply to alice's earlier message
				sse := "event: done\ndata: {\"ok\":true,\"text\":\"Got your reply! [DONE]\",\"conversation_id\":\"conv-alice-1\"}\n\n"
				return sseHTTPResponse(sse), nil

			case "agent-bob":
				sse := strings.Join([]string{
					`event: conversation`,
					`data: {"conversation_id":"conv-bob-1"}`,
					``,
					`event: chunk`,
					`data: {"text":"Approved! [DONE]"}`,
					``,
					`event: done`,
					`data: {"ok":true,"text":"Approved! [DONE]","conversation_id":"conv-bob-1"}`,
					``,
				}, "\n")
				return sseHTTPResponse(sse), nil
			}
			return jsonHTTPResponse(http.StatusNotFound, `{"error":"unknown host"}`), nil
		}),
	}

	svc := NewGroupServiceWithStore(newTestGroupStore(t), dir, client)

	group, err := svc.Create(CreateGroupRequest{
		Name:           "Arch <Council>",
		Description:    "Design reviews",
		Members:        []string{"alice", "bob", "alice"}, // duplicate should dedupe
		DiscussionMode: DiscussionModeAutonomous,
		MaxTurns:       8,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(group.Members) != 2 {
		t.Fatalf("members = %+v, want 2 deduplicated members", group.Members)
	}

	ch, _, unsub, err := svc.Subscribe(group.ID)
	defer unsub()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Send first user message addressing @alice with an image and file upload.
	userMsg, state, err := svc.SendMessage(group.ID, 42, SendGroupMessageRequest{
		Text:   "@alice review the API contract",
		Images: []string{"aW1hZ2U="},
		Files:  []channel.FileUpload{{Name: "spec.md", MimeType: "text/markdown", DataBase64: "IyBTcGVj"}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !state.Running || len(state.Queue) != 1 || state.Queue[0] != "alice" {
		t.Fatalf("initial state = %+v", state)
	}

	events, endData := waitForRunEnd(t, ch, 5*time.Second)
	if endData["reason"] != "completed" {
		t.Fatalf("run_end reason = %v, want completed", endData["reason"])
	}

	// Verify normalized SSE event names were emitted.
	seenEvents := make(map[string]bool)
	for _, ev := range events {
		seenEvents[ev.Name] = true
	}
	for _, want := range []string{"turn_start", "thinking", "tool_start", "tool_end", "chunk", "message", "turn_end", "run_end"} {
		if !seenEvents[want] {
			t.Fatalf("missing SSE event %q; saw %v", want, seenEvents)
		}
	}

	detail, err := svc.Get(group.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Expect 3 messages: User, alice, bob (with [DONE] stripped).
	if len(detail.Messages) != 3 {
		t.Fatalf("messages count = %d, want 3: %+v", len(detail.Messages), detail.Messages)
	}
	aliceMsg := detail.Messages[1]
	if aliceMsg.SenderName != "alice" || aliceMsg.AgentColor != "bleu" || len(aliceMsg.Timeline) < 2 {
		t.Fatalf("unexpected alice message: %+v", aliceMsg)
	}
	bobMsg := detail.Messages[2]
	if bobMsg.SenderName != "bob" || bobMsg.Content != "Approved!" {
		t.Fatalf("unexpected bob message (should strip [DONE]): %+v", bobMsg)
	}

	// Verify conversation_id and last_seen_message_id persisted on members.
	if detail.Group.Members[0].ConversationID != "conv-alice-1" || detail.Group.Members[0].LastSeenMessageID != aliceMsg.ID {
		t.Fatalf("alice member state = %+v", detail.Group.Members[0])
	}
	if detail.Group.Members[1].ConversationID != "conv-bob-1" || detail.Group.Members[1].LastSeenMessageID != bobMsg.ID {
		t.Fatalf("bob member state = %+v", detail.Group.Members[1])
	}

	// Now send a WhatsApp-style Reply to alice's message + text-selection Quote!
	// Because no @mention is in text, autonomous mode should route directly to alice (repliedAgent).
	startOff, endOff := 0, 10
	replyMsg, _, err := svc.SendMessage(group.ID, 42, SendGroupMessageRequest{
		Text:      "Can you double check this part?",
		ReplyToID: aliceMsg.ID,
		Quotes: []AskQuote{
			{
				ID:          "quote-1",
				Text:        "Looks good </selection>",
				Source:      "answer",
				SourceID:    aliceMsg.ID,
				StartOffset: &startOff,
				EndOffset:   &endOff,
			},
		},
	})
	if err != nil {
		t.Fatalf("SendMessage reply: %v", err)
	}
	if replyMsg.ReplyPreview == nil || replyMsg.ReplyPreview.SenderName != "alice" || len(replyMsg.Quotes) != 1 {
		t.Fatalf("replyMsg metadata missing: %+v", replyMsg)
	}

	_, endData2 := waitForRunEnd(t, ch, 5*time.Second)
	if endData2["reason"] != "completed" {
		t.Fatalf("second run_end reason = %v, want completed", endData2["reason"])
	}

	// Inspect the 3 HTTP calls made to agents.
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 3 {
		t.Fatalf("requests count = %d, want 3", len(requests))
	}
	// Call 1 (alice first turn): conversation_id == "", full rules, sanitized group name, image & file attached.
	r1 := requests[0].Body
	if r1.ConversationID != "" || r1.UserID != 42 || len(r1.Images) != 1 || len(r1.Files) != 1 {
		t.Fatalf("req 1 = %+v", r1)
	}
	if !strings.HasPrefix(r1.Text, "<group>\n<group_context>\n") ||
		!strings.Contains(r1.Text, "Group: Arch Council") ||
		!strings.Contains(r1.Text, "Group Session ID: "+group.ID) ||
		!strings.Contains(r1.Text, "Rules:") ||
		!strings.Contains(r1.Text, "</group_context>\n<chats>\n") ||
		!strings.HasSuffix(r1.Text, "\n</chats>\n</group>") {
		t.Fatalf("req 1 missing wrapped first-turn <group>/<group_context>/<chats>:\n%s", r1.Text)
	}
	// Call 2 (bob first turn): sees both userMsg and aliceMsg, and gets the image & file since userMsg is in bob's unseen delta.
	r2 := requests[1].Body
	if r2.ConversationID != "" || !strings.Contains(r2.Text, "[alice]") || len(r2.Images) != 1 || len(r2.Files) != 1 {
		t.Fatalf("req 2 = %+v", r2)
	}
	// Call 3 (alice second turn): reuses conv-alice-1, compact header, no stale image/file, sees bob's message + reply + escaped selection quote, does NOT re-see userMsg!
	r3 := requests[2].Body
	if r3.ConversationID != "conv-alice-1" || len(r3.Images) != 0 || len(r3.Files) != 0 {
		t.Fatalf("req 3 session/images/files = %+v", r3)
	}
	if !strings.Contains(r3.Text, "Group Session ID: "+group.ID) || strings.Contains(r3.Text, "conv-alice-1") {
		t.Fatalf("req 3 should expose Group Session ID (and not internal conversation id) in compact header:\n%s", r3.Text)
	}
	if strings.Contains(r3.Text, "Rules:") {
		t.Fatalf("req 3 should use compact header, got:\n%s", r3.Text)
	}
	if strings.Contains(r3.Text, userMsg.Content) {
		t.Fatalf("req 3 should NOT re-send already-seen first user message:\n%s", r3.Text)
	}
	if !strings.Contains(r3.Text, "[bob]") || !strings.Contains(r3.Text, `replying to alice:`) || !strings.Contains(r3.Text, `<\/selection>`) {
		t.Fatalf("req 3 missing delta/reply/quote context:\n%s", r3.Text)
	}
}

func TestGroupSelfHealing404PassMaxTurnsContinueAndOfflineSkip(t *testing.T) {
	dir := newMockAgentDirectory(
		AgentInfo{Name: "alpha", AgentColor: "rouge", Running: true, BaseURL: "http://agent-alpha"},
		AgentInfo{Name: "beta", AgentColor: "ambre", Running: true, BaseURL: "http://agent-beta"},
		AgentInfo{Name: "offline", AgentColor: "gris", Running: false, BaseURL: "http://agent-offline"},
	)

	var (
		mu             sync.Mutex
		alpha404Inject = true
		alphaCalls     = 0
		betaCalls      = 0
	)

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var body agentChatStreamRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			defer mu.Unlock()

			switch r.URL.Host {
			case "agent-alpha":
				alphaCalls++
				// Simulate 404 when a stale conversation_id is presented.
				if body.ConversationID == "stale-conv" && alpha404Inject {
					alpha404Inject = false
					return jsonHTTPResponse(http.StatusNotFound, `{"error":"conversation not found"}`), nil
				}
				return sseHTTPResponse("event: done\ndata: {\"ok\":true,\"text\":\"Alpha ping @beta\",\"conversation_id\":\"fresh-alpha\"}\n\n"), nil
			case "agent-beta":
				betaCalls++
				if betaCalls == 1 {
					return sseHTTPResponse("event: done\ndata: {\"ok\":true,\"text\":\"Beta ping @alpha\",\"conversation_id\":\"conv-beta\"}\n\n"), nil
				}
				// On continued turn, beta replies [PASS].
				return sseHTTPResponse("event: done\ndata: {\"ok\":true,\"text\":\"[PASS]\",\"conversation_id\":\"conv-beta\"}\n\n"), nil
			}
			return jsonHTTPResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}),
	}

	store := newTestGroupStore(t)
	svc := NewGroupServiceWithStore(store, dir, client)

	group, err := svc.Create(CreateGroupRequest{
		Name:           "Loop Guard",
		Members:        []string{"offline", "alpha", "beta"},
		DiscussionMode: DiscussionModeAutonomous,
		MaxTurns:       2,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Seed a stale conversation_id on alpha to test self-healing 404 fallback.
	_, err = store.Update(group.ID, func(sg *storedGroup) error {
		for i := range sg.Group.Members {
			if sg.Group.Members[i].Name == "alpha" {
				sg.Group.Members[i].ConversationID = "stale-conv"
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed stale conv: %v", err)
	}

	ch, _, unsub, err := svc.Subscribe(group.ID)
	defer unsub()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Continuing before paused must return ErrGroupNotPaused.
	if _, err := svc.Continue(group.ID, 1, 5); err != ErrGroupNotPaused {
		t.Fatalf("Continue while idle = %v, want ErrGroupNotPaused", err)
	}

	// Trigger @all: offline should be skipped WITHOUT consuming a turn; alpha (after 404 retry) + beta consume turns 1 & 2, then pause at max_turns=2 with alpha still in queue!
	if _, _, err := svc.SendMessage(group.ID, 1, SendGroupMessageRequest{Text: "@all start"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	events, endData := waitForRunEnd(t, ch, 5*time.Second)
	if endData["reason"] != "max_turns" || endData["paused"] != true {
		t.Fatalf("expected max_turns pause, got: %+v", endData)
	}
	sawOfflineErr := false
	for _, ev := range events {
		if ev.Name == "error" {
			m, _ := ev.Data.(map[string]any)
			if m["agent"] == "offline" {
				sawOfflineErr = true
			}
		}
	}
	if !sawOfflineErr {
		t.Fatalf("expected error event for skipped offline agent")
	}

	// Continue for +2 turns: alpha runs (turn 3), mentions beta, beta runs (turn 4) and replies [PASS] -> completed!
	contState, err := svc.Continue(group.ID, 1, 2)
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if !contState.Running || contState.MaxTurns != 4 {
		t.Fatalf("continued state = %+v", contState)
	}

	events2, endData2 := waitForRunEnd(t, ch, 5*time.Second)
	if endData2["reason"] != "completed" {
		t.Fatalf("after continue reason = %v, want completed", endData2["reason"])
	}
	sawPass := false
	for _, ev := range events2 {
		if ev.Name == "turn_end" {
			m, _ := ev.Data.(map[string]any)
			if m["agent"] == "beta" && m["passed"] == true {
				sawPass = true
			}
		}
	}
	if !sawPass {
		t.Fatalf("expected beta [PASS] turn_end event")
	}

	detail, err := svc.Get(group.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Verify alpha's conversation_id healed to "fresh-alpha".
	for _, m := range detail.Group.Members {
		if m.Name == "alpha" && m.ConversationID != "fresh-alpha" {
			t.Fatalf("alpha conversation_id = %q, want fresh-alpha", m.ConversationID)
		}
	}
	// Messages: User + alpha #1 + beta #1 + alpha #2 (beta #2 was [PASS] so not persisted) = 4 messages.
	if len(detail.Messages) != 4 {
		t.Fatalf("messages count = %d, want 4", len(detail.Messages))
	}
}

func TestGroupStopCallsAgentStopEndpointAndUserInterrupts(t *testing.T) {
	dir := newMockAgentDirectory(
		AgentInfo{Name: "slow", AgentColor: "violet", Running: true, BaseURL: "http://agent-slow"},
	)

	startedCh := make(chan string, 2)
	stopCalledCh := make(chan string, 2)

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/api/chat/stop" {
				var p struct {
					RequestID string `json:"request_id"`
				}
				_ = json.NewDecoder(r.Body).Decode(&p)
				stopCalledCh <- p.RequestID
				return jsonHTTPResponse(http.StatusOK, `{"stopped":true}`), nil
			}

			var req agentChatStreamRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			startedCh <- req.RequestID

			pr, pw := io.Pipe()
			go func() {
				defer pw.Close()
				_, _ = pw.Write([]byte("event: chunk\ndata: {\"text\":\"working...\"}\n\n"))
				<-r.Context().Done()
			}()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       pr,
			}, nil
		}),
	}

	svc := NewGroupServiceWithStore(newTestGroupStore(t), dir, client)
	group, err := svc.Create(CreateGroupRequest{
		Name:    "Stop Test",
		Members: []string{"slow"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, _, unsub, err := svc.Subscribe(group.ID)
	defer unsub()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if _, _, err := svc.SendMessage(group.ID, 7, SendGroupMessageRequest{Text: "start long task"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	var firstReqID string
	select {
	case firstReqID = <-startedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("agent never started streaming")
	}

	stopped, state, err := svc.Stop(group.ID, 7)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !stopped || state.Running {
		t.Fatalf("Stop result = (%v, %+v)", stopped, state)
	}

	select {
	case gotStopID := <-stopCalledCh:
		if gotStopID != firstReqID {
			t.Fatalf("stop request_id = %q, want %q", gotStopID, firstReqID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("agent /api/chat/stop was not called")
	}

	_, endData := waitForRunEnd(t, ch, 3*time.Second)
	if endData["reason"] != "stopped" {
		t.Fatalf("run_end reason = %v, want stopped", endData["reason"])
	}

	// Test user interruption cancels prior run cleanly.
	if _, _, err := svc.SendMessage(group.ID, 7, SendGroupMessageRequest{Text: "task 2"}); err != nil {
		t.Fatalf("SendMessage 2: %v", err)
	}
	var secondReqID string
	select {
	case secondReqID = <-startedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("second run never started")
	}

	if _, _, err := svc.SendMessage(group.ID, 7, SendGroupMessageRequest{Text: "interrupting task 2"}); err != nil {
		t.Fatalf("SendMessage interrupt: %v", err)
	}

	select {
	case gotStopID := <-stopCalledCh:
		if gotStopID != secondReqID {
			t.Fatalf("interrupt stop request_id = %q, want %q", gotStopID, secondReqID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("interrupt did not call /api/chat/stop")
	}

	_, interruptEnd := waitForRunEnd(t, ch, 3*time.Second)
	if interruptEnd["reason"] != "interrupted" {
		t.Fatalf("interrupt run_end reason = %v, want interrupted", interruptEnd["reason"])
	}

	// Clean up running goroutine before test exits.
	_, _, _ = svc.Stop(group.ID, 7)
}

func TestGroupNoOnlineMembersAndMentionModeAndClear(t *testing.T) {
	dir := newMockAgentDirectory(
		AgentInfo{Name: "offline1", AgentColor: "gris", Running: false, BaseURL: "http://offline1"},
	)

	svc := NewGroupServiceWithStore(newTestGroupStore(t), dir, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("dial tcp: connection refused")
		}),
	})

	// Mention mode with no mention ends immediately with completed.
	gMention, err := svc.Create(CreateGroupRequest{
		Name:           "Quiet Room",
		Members:        []string{"offline1"},
		DiscussionMode: DiscussionModeMention,
	})
	if err != nil {
		t.Fatalf("Create mention: %v", err)
	}
	_, st, err := svc.SendMessage(gMention.ID, 1, SendGroupMessageRequest{Text: "just a note"})
	if err != nil {
		t.Fatalf("SendMessage mention: %v", err)
	}
	if st.Running {
		t.Fatalf("mention mode without @mention should not run, got %+v", st)
	}

	// Explicit mention of offline member yields no_online_members.
	ch, _, unsub, err := svc.Subscribe(gMention.ID)
	defer unsub()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if _, _, err := svc.SendMessage(gMention.ID, 1, SendGroupMessageRequest{Text: "@offline1 hello"}); err != nil {
		t.Fatalf("SendMessage offline: %v", err)
	}
	_, endData := waitForRunEnd(t, ch, 3*time.Second)
	if endData["reason"] != "no_online_members" {
		t.Fatalf("reason = %v, want no_online_members", endData["reason"])
	}

	// Clear messages resets transcript and member cursors.
	if err := svc.ClearMessages(gMention.ID, 1); err != nil {
		t.Fatalf("ClearMessages: %v", err)
	}
	detail, err := svc.Get(gMention.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(detail.Messages) != 0 {
		t.Fatalf("messages after clear = %d, want 0", len(detail.Messages))
	}

	// Ensure bytes import isn't unused if edited; sanity check JSON encoding of GroupDetail has non-null slices.
	raw, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	if bytes.Contains(raw, []byte(`"messages":null`)) || bytes.Contains(raw, []byte(`"queue":null`)) {
		t.Fatalf("detail JSON contains null array: %s", raw)
	}
}

func TestGroupDialUnreachableEmitsTurnEndAndStatusReset(t *testing.T) {
	// Agent reports Running=true at discovery time, then fails to dial during /api/chat/stream.
	dir := newMockAgentDirectory(
		AgentInfo{Name: "flaky", AgentColor: "rouge", Running: true, BaseURL: "http://agent-flaky"},
	)

	svc := NewGroupServiceWithStore(newTestGroupStore(t), dir, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("dial tcp: connection refused")
		}),
	})

	group, err := svc.Create(CreateGroupRequest{
		Name:    "Flaky Dial",
		Members: []string{"flaky"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, _, unsub, err := svc.Subscribe(group.ID)
	defer unsub()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if _, _, err := svc.SendMessage(group.ID, 1, SendGroupMessageRequest{Text: "hello"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	events, endData := waitForRunEnd(t, ch, 3*time.Second)
	if endData["reason"] != "no_online_members" {
		t.Fatalf("reason = %v, want no_online_members", endData["reason"])
	}

	var sawTurnStart, sawError, sawTurnEnd, sawResetStatus bool
	for _, ev := range events {
		switch ev.Name {
		case "turn_start":
			sawTurnStart = true
		case "error":
			sawError = true
		case "turn_end":
			m, _ := ev.Data.(map[string]any)
			if m["agent"] == "flaky" && m["passed"] == false {
				sawTurnEnd = true
			}
		case "status":
			m, _ := ev.Data.(map[string]any)
			if rs, ok := m["run_state"].(GroupRunState); ok && sawTurnEnd && rs.TurnCount == 0 && rs.CurrentAgent == "" {
				sawResetStatus = true
			}
		}
	}
	if !sawTurnStart || !sawError || !sawTurnEnd || !sawResetStatus {
		t.Fatalf("missing expected unreachable cleanup events: turn_start=%v error=%v turn_end=%v reset_status=%v",
			sawTurnStart, sawError, sawTurnEnd, sawResetStatus)
	}
}

func TestGroupMentionOverridesDoneAndPreservesUnspokenInitialMembers(t *testing.T) {
	dir := newMockAgentDirectory(
		AgentInfo{Name: "default", AgentColor: "encre", Running: true, BaseURL: "http://agent-default"},
		AgentInfo{Name: "bob", AgentColor: "rose", Running: true, BaseURL: "http://agent-bob"},
		AgentInfo{Name: "charlie", AgentColor: "bleu", Running: true, BaseURL: "http://agent-charlie"},
	)

	var (
		mu      sync.Mutex
		order   []string
		prompts = make(map[string][]string)
	)

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var body agentChatStreamRequest
			_ = json.NewDecoder(r.Body).Decode(&body)

			mu.Lock()
			host := strings.TrimPrefix(r.URL.Host, "agent-")
			order = append(order, host)
			prompts[host] = append(prompts[host], body.Text)
			callCount := len(prompts[host])
			mu.Unlock()

			switch host {
			case "default":
				if callCount == 1 {
					// Run 1 (initial broadcast): default replies normally.
					return sseHTTPResponse("event: done\ndata: {\"ok\":true,\"text\":\"Default checking in.\"}\n\n"), nil
				}
				// Run 2 (explicit @default -> hands off to **@charlie** WITH [DONE]):
				return sseHTTPResponse("event: done\ndata: {\"ok\":true,\"text\":\"Passing to **@charlie** to verify! [DONE]\"}\n\n"), nil
			case "bob":
				// Run 1: bob outputs [DONE] while charlie has NOT spoken yet!
				return sseHTTPResponse("event: done\ndata: {\"ok\":true,\"text\":\"All good from me! [DONE]\"}\n\n"), nil
			case "charlie":
				return sseHTTPResponse("event: done\ndata: {\"ok\":true,\"text\":\"Charlie here, verified! [DONE]\"}\n\n"), nil
			}
			return jsonHTTPResponse(http.StatusNotFound, `{"error":"unknown"}`), nil
		}),
	}

	svc := NewGroupServiceWithStore(newTestGroupStore(t), dir, client)
	group, err := svc.Create(CreateGroupRequest{
		Name:           "Trio",
		Members:        []string{"default", "bob", "charlie"},
		DiscussionMode: DiscussionModeAutonomous,
		MaxTurns:       8,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, _, unsub, err := svc.Subscribe(group.ID)
	defer unsub()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Run 1: broadcast to all 3 members; bob (#2) outputs [DONE], yet unspoken charlie (#3) MUST still run!
	if _, _, err := svc.SendMessage(group.ID, 1, SendGroupMessageRequest{Text: "can everyone check this?"}); err != nil {
		t.Fatalf("SendMessage run 1: %v", err)
	}
	_, end1 := waitForRunEnd(t, ch, 5*time.Second)
	if end1["reason"] != "completed" {
		t.Fatalf("run 1 reason = %v, want completed", end1["reason"])
	}

	mu.Lock()
	run1Order := append([]string(nil), order...)
	order = nil
	mu.Unlock()
	if len(run1Order) != 3 || run1Order[0] != "default" || run1Order[1] != "bob" || run1Order[2] != "charlie" {
		t.Fatalf("run 1 order = %v, want [default bob charlie]", run1Order)
	}

	// Run 2: @default replies with BOTH **@charlie** AND [DONE] -> @mention MUST take precedence over [DONE]!
	if _, _, err := svc.SendMessage(group.ID, 1, SendGroupMessageRequest{Text: "@default hand this off"}); err != nil {
		t.Fatalf("SendMessage run 2: %v", err)
	}
	_, end2 := waitForRunEnd(t, ch, 5*time.Second)
	if end2["reason"] != "completed" {
		t.Fatalf("run 2 reason = %v, want completed", end2["reason"])
	}

	mu.Lock()
	run2Order := append([]string(nil), order...)
	charliePrompts := append([]string(nil), prompts["charlie"]...)
	mu.Unlock()
	if len(run2Order) != 2 || run2Order[0] != "default" || run2Order[1] != "charlie" {
		t.Fatalf("run 2 order = %v, want [default charlie]", run2Order)
	}
	// On run 2, charlie was directly mentioned by default, so its prompt must include the direct-address directive.
	if len(charliePrompts) < 2 || !strings.Contains(charliePrompts[1], "directly mentioned or addressed") {
		t.Fatalf("charlie run 2 prompt missing direct-address note:\n%v", charliePrompts)
	}
}

func TestGroupPlainNameAddressingPrioritizesQueueAndPrompt(t *testing.T) {
	dir := newMockAgentDirectory(
		AgentInfo{Name: "default", AgentColor: "encre", Running: true, BaseURL: "http://agent-default"},
		AgentInfo{Name: "bob", AgentColor: "rose", Running: true, BaseURL: "http://agent-bob"},
		AgentInfo{Name: "charlie", AgentColor: "bleu", Running: true, BaseURL: "http://agent-charlie"},
	)

	var (
		mu      sync.Mutex
		order   []string
		prompts = make(map[string]string)
	)

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var body agentChatStreamRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			host := strings.TrimPrefix(r.URL.Host, "agent-")

			mu.Lock()
			order = append(order, host)
			prompts[host] = body.Text
			mu.Unlock()

			return sseHTTPResponse("event: done\ndata: {\"ok\":true,\"text\":\"ok from " + host + "\"}\n\n"), nil
		}),
	}

	svc := NewGroupServiceWithStore(newTestGroupStore(t), dir, client)
	group, err := svc.Create(CreateGroupRequest{
		Name:           "Plain Name Room",
		Members:        []string{"default", "bob", "charlie"},
		DiscussionMode: DiscussionModeAutonomous,
		MaxTurns:       8,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, _, unsub, err := svc.Subscribe(group.ID)
	defer unsub()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	_, state, err := svc.SendMessage(group.ID, 1, SendGroupMessageRequest{Text: "charlie can you answer this"})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if len(state.Queue) != 3 || state.Queue[0] != "charlie" || state.Queue[1] != "default" || state.Queue[2] != "bob" {
		t.Fatalf("initial queue = %v, want [charlie default bob]", state.Queue)
	}

	_, endData := waitForRunEnd(t, ch, 5*time.Second)
	if endData["reason"] != "completed" {
		t.Fatalf("reason = %v, want completed", endData["reason"])
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 || order[0] != "charlie" || order[1] != "default" || order[2] != "bob" {
		t.Fatalf("execution order = %v, want [charlie default bob]", order)
	}
	if !strings.Contains(prompts["charlie"], "directly mentioned or addressed") {
		t.Fatalf("charlie prompt should contain direct-address directive:\n%s", prompts["charlie"])
	}
	if strings.Contains(prompts["default"], "directly mentioned or addressed") {
		t.Fatalf("default prompt should NOT contain direct-address directive:\n%s", prompts["default"])
	}
	if !strings.Contains(prompts["charlie"], "Never append [DONE] when you @mention a teammate") {
		t.Fatalf("prompt missing stronger [DONE] guardrail:\n%s", prompts["charlie"])
	}
}

func TestGroupStoreAndServicePagination(t *testing.T) {
	dir := newMockAgentDirectory(
		AgentInfo{Name: "alice", AgentColor: "bleu", Running: false, BaseURL: "http://agent-alice"},
	)
	store := newTestGroupStore(t)
	svc := NewGroupServiceWithStore(store, dir, nil)

	group, err := svc.Create(CreateGroupRequest{
		Name:           "Paged Room",
		Members:        []string{"alice"},
		DiscussionMode: DiscussionModeMention,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Seed 25 messages chronologically.
	if _, err := store.Update(group.ID, func(sg *storedGroup) error {
		for i := 1; i <= 25; i++ {
			sg.Messages = append(sg.Messages, GroupMessage{
				ID:         fmt.Sprintf("msg-%02d", i),
				GroupID:    group.ID,
				SenderType: "user",
				SenderName: "User",
				Content:    fmt.Sprintf("Message %02d", i),
				CreatedAt:  "2026-11-21T07:00:00Z",
			})
		}
		return nil
	}); err != nil {
		t.Fatalf("seed messages: %v", err)
	}

	// 1. Unpaged (limit=0) returns all 25 messages.
	all, err := svc.GetPage(group.ID, 0, 0, "")
	if err != nil {
		t.Fatalf("GetPage unpaged: %v", err)
	}
	if len(all.Messages) != 25 || all.TotalMessages != 25 || all.Offset != 0 || all.Limit != 0 || all.HasMore {
		t.Fatalf("unpaged = len=%d total=%d offset=%d limit=%d hasMore=%v",
			len(all.Messages), all.TotalMessages, all.Offset, all.Limit, all.HasMore)
	}

	// 2. Tail page (limit=10, offset=-1) returns msg-16..msg-25 with offset=15, has_more=true.
	tail, err := svc.GetPage(group.ID, 10, -1, "")
	if err != nil {
		t.Fatalf("GetPage tail: %v", err)
	}
	if len(tail.Messages) != 10 || tail.TotalMessages != 25 || tail.Offset != 15 || tail.Limit != 10 || !tail.HasMore {
		t.Fatalf("tail = len=%d total=%d offset=%d limit=%d hasMore=%v",
			len(tail.Messages), tail.TotalMessages, tail.Offset, tail.Limit, tail.HasMore)
	}
	if tail.Messages[0].ID != "msg-16" || tail.Messages[9].ID != "msg-25" {
		t.Fatalf("tail bounds = [%s .. %s], want [msg-16 .. msg-25]", tail.Messages[0].ID, tail.Messages[9].ID)
	}

	// 3. Backward page via before_id returns msg-06..msg-15 chronologically with offset=5, has_more=true.
	prev1, err := svc.GetPage(group.ID, 10, 0, tail.Messages[0].ID)
	if err != nil {
		t.Fatalf("GetPage before_id 1: %v", err)
	}
	if len(prev1.Messages) != 10 || prev1.Offset != 5 || !prev1.HasMore {
		t.Fatalf("prev1 = len=%d offset=%d hasMore=%v", len(prev1.Messages), prev1.Offset, prev1.HasMore)
	}
	if prev1.Messages[0].ID != "msg-06" || prev1.Messages[9].ID != "msg-15" {
		t.Fatalf("prev1 bounds = [%s .. %s], want [msg-06 .. msg-15]", prev1.Messages[0].ID, prev1.Messages[9].ID)
	}

	// 4. Simulate concurrent appends at the tail while scrolling up.
	if _, err := store.Update(group.ID, func(sg *storedGroup) error {
		for i := 26; i <= 28; i++ {
			sg.Messages = append(sg.Messages, GroupMessage{
				ID:         fmt.Sprintf("msg-%02d", i),
				GroupID:    group.ID,
				SenderType: "user",
				SenderName: "User",
				Content:    fmt.Sprintf("Message %02d", i),
				CreatedAt:  "2026-11-21T07:05:00Z",
			})
		}
		return nil
	}); err != nil {
		t.Fatalf("append concurrent messages: %v", err)
	}

	// Next before_id page is immune to concurrent appends: exact msg-01..msg-05, offset=0, has_more=false.
	prev2, err := svc.GetPage(group.ID, 10, 0, prev1.Messages[0].ID)
	if err != nil {
		t.Fatalf("GetPage before_id 2: %v", err)
	}
	if len(prev2.Messages) != 5 || prev2.TotalMessages != 28 || prev2.Offset != 0 || prev2.HasMore {
		t.Fatalf("prev2 = len=%d total=%d offset=%d hasMore=%v",
			len(prev2.Messages), prev2.TotalMessages, prev2.Offset, prev2.HasMore)
	}
	if prev2.Messages[0].ID != "msg-01" || prev2.Messages[4].ID != "msg-05" {
		t.Fatalf("prev2 bounds = [%s .. %s], want [msg-01 .. msg-05]", prev2.Messages[0].ID, prev2.Messages[4].ID)
	}

	// 5. Clamping & edge cases: limit > 500 clamped to 500, out-of-range offset clamped to total, unknown before_id empty.
	clamped, err := svc.GetPage(group.ID, 999, 0, "")
	if err != nil {
		t.Fatalf("GetPage clamped limit: %v", err)
	}
	if clamped.Limit != MaxGroupMessagePageLimit || len(clamped.Messages) != 28 || clamped.HasMore {
		t.Fatalf("clamped = limit=%d len=%d hasMore=%v", clamped.Limit, len(clamped.Messages), clamped.HasMore)
	}

	pastEnd, err := svc.GetPage(group.ID, 10, 999, "")
	if err != nil {
		t.Fatalf("GetPage past end: %v", err)
	}
	if len(pastEnd.Messages) != 0 || pastEnd.Offset != 28 || !pastEnd.HasMore {
		t.Fatalf("pastEnd = len=%d offset=%d hasMore=%v", len(pastEnd.Messages), pastEnd.Offset, pastEnd.HasMore)
	}

	missingCursor, err := svc.GetPage(group.ID, 10, 0, "does-not-exist")
	if err != nil {
		t.Fatalf("GetPage missing cursor: %v", err)
	}
	if len(missingCursor.Messages) != 0 || missingCursor.Offset != 0 || missingCursor.HasMore {
		t.Fatalf("missingCursor = len=%d offset=%d hasMore=%v", len(missingCursor.Messages), missingCursor.Offset, missingCursor.HasMore)
	}
}

func TestGroupStreamForwardsScopeAndApprovals(t *testing.T) {
	dir := newMockAgentDirectory(
		AgentInfo{Name: "mikaela", AgentColor: "amber", Running: true, BaseURL: "http://agent-mikaela"},
	)

	var gotScope string
	releaseApproval := make(chan struct{})

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var req agentChatStreamRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			gotScope = req.Scope

			pr, pw := io.Pipe()
			go func() {
				defer pw.Close()
				_, _ = io.WriteString(pw, "event: conversation\ndata: {\"conversation_id\":\"conv-grp-1\"}\n\n")
				_, _ = io.WriteString(pw, "event: approval_required\ndata: {\"approval_id\":\"appr-1\",\"tool_name\":\"bash\",\"action\":\"run\",\"args_summary\":\"sleep 5\",\"args\":\"{\\\"command\\\":\\\"sleep 5\\\"}\"}\n\n")
				<-releaseApproval
				_, _ = io.WriteString(pw, "event: approval_resolved\ndata: {\"approval_id\":\"appr-1\",\"decision\":\"allow_once\"}\n\n")
				_, _ = io.WriteString(pw, "event: chunk\ndata: {\"text\":\"Done!\"}\n\n")
				_, _ = io.WriteString(pw, "event: done\ndata: {\"ok\":true,\"text\":\"Done!\",\"conversation_id\":\"conv-grp-1\"}\n\n")
			}()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       pr,
			}, nil
		}),
	}

	svc := NewGroupServiceWithStore(newTestGroupStore(t), dir, client)
	group, err := svc.Create(CreateGroupRequest{
		Name:           "Ops",
		Members:        []string{"mikaela"},
		DiscussionMode: DiscussionModeAll,
		MaxTurns:       2,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, _, unsub, err := svc.Subscribe(group.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	if _, _, err := svc.SendMessage(group.ID, 1, SendGroupMessageRequest{Text: "run sleep 5"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Wait until approval_required arrives on the group SSE stream.
	deadline := time.After(5 * time.Second)
	var sawRequired bool
	for !sawRequired {
		select {
		case ev := <-ch:
			if ev.Name == "approval_required" {
				sawRequired = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for approval_required")
		}
	}

	if wantScope := "group:" + group.ID; gotScope != wantScope {
		t.Fatalf("agent request scope = %q, want %q", gotScope, wantScope)
	}

	// While paused on approval, Get() and List() must surface pending approval state.
	detail, err := svc.Get(group.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(detail.RunState.PendingApprovals) != 1 || detail.RunState.PendingApprovals[0].ApprovalID != "appr-1" {
		t.Fatalf("PendingApprovals = %+v, want [appr-1]", detail.RunState.PendingApprovals)
	}
	summaries, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 1 || summaries[0].PendingApprovalCount != 1 {
		t.Fatalf("List PendingApprovalCount = %+v, want 1", summaries)
	}

	// Release approval and wait for run completion.
	close(releaseApproval)
	events, endData := waitForRunEnd(t, ch, 5*time.Second)
	if endData["reason"] != "completed" {
		t.Fatalf("run_end reason = %v, want completed", endData["reason"])
	}
	sawResolved := false
	for _, ev := range events {
		if ev.Name == "approval_resolved" {
			sawResolved = true
		}
	}
	if !sawResolved {
		t.Fatal("expected approval_resolved event on group stream")
	}

	after, err := svc.Get(group.ID)
	if err != nil {
		t.Fatalf("Get after: %v", err)
	}
	if len(after.RunState.PendingApprovals) != 0 {
		t.Fatalf("PendingApprovals after resolve = %+v, want empty", after.RunState.PendingApprovals)
	}
}

func TestGroupUploadsAndAttachments(t *testing.T) {
	dir := newMockAgentDirectory(
		AgentInfo{Name: "alice", AgentColor: "bleu", Running: true, BaseURL: "http://agent-alice"},
	)
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return sseHTTPResponse(
				"event: done\ndata: {\"ok\":true,\"text\":\"Got it!\",\"conversation_id\":\"conv-1\",\"attachments\":[{\"file_name\":\"photo.jpg\",\"mime_type\":\"image/jpeg\",\"path\":\"/home/workspace/photo.jpg\",\"kind\":\"image\",\"size\":1234}]}\n\n",
			), nil
		}),
	}

	svc := NewGroupServiceWithStore(newTestGroupStore(t), dir, client)
	group, err := svc.Create(CreateGroupRequest{
		Name:           "Files",
		Members:        []string{"alice"},
		DiscussionMode: DiscussionModeAll,
		MaxTurns:       2,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, _, unsub, err := svc.Subscribe(group.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	// 1x1 valid PNG (base64) + plain text PDF/doc upload.
	pngB64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	docB64 := "aGVsbG8gZ3JvdXAgZmlsZQ==" // "hello group file"

	userMsg, _, err := svc.SendMessage(group.ID, 1, SendGroupMessageRequest{
		Text:   "check these",
		Images: []string{pngB64},
		Files:  []channel.FileUpload{{Name: "notes.txt", DataBase64: docB64}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	waitForRunEnd(t, ch, 5*time.Second)

	if len(userMsg.Attachments) != 2 {
		t.Fatalf("Attachments len = %d, want 2", len(userMsg.Attachments))
	}
	if userMsg.Attachments[0].Kind != "image" || userMsg.Attachments[1].Kind != "file" || userMsg.Attachments[1].FileName != "notes.txt" {
		t.Fatalf("unexpected attachments = %+v", userMsg.Attachments)
	}

	// Verify both user uploads AND agent @attach(...) deliverables persist across GetPage & ListAttachments.
	page, err := svc.GetPage(group.ID, 10, -1, "")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if len(page.Messages) != 2 || len(page.Messages[0].Attachments) != 2 || len(page.Messages[1].Attachments) != 1 {
		t.Fatalf("persisted message attachments = %+v", page.Messages)
	}
	if page.Messages[1].Attachments[0].Agent != "alice" || page.Messages[1].Attachments[0].Path != "/home/workspace/photo.jpg" {
		t.Fatalf("agent attachment = %+v", page.Messages[1].Attachments[0])
	}

	allAtts, err := svc.ListAttachments(group.ID, "", 50, 0)
	if err != nil || allAtts.Total != 3 || len(allAtts.Items) != 3 {
		t.Fatalf("ListAttachments all = %+v, err=%v", allAtts, err)
	}
	imgAtts, err := svc.ListAttachments(group.ID, "image", 50, 0)
	if err != nil || imgAtts.Total != 2 || imgAtts.Items[0].Kind != "image" {
		t.Fatalf("ListAttachments image = %+v, err=%v", imgAtts, err)
	}

	// Verify file exists on disk and AttachmentFile resolves safely.
	absPath, origName, mimeType, err := svc.AttachmentFile(group.ID, userMsg.ID, userMsg.Attachments[1].StoredName)
	if err != nil || origName != "notes.txt" || mimeType == "" {
		t.Fatalf("AttachmentFile = (%q, %q, %q, %v)", absPath, origName, mimeType, err)
	}
	if _, _, _, err := svc.AttachmentFile(group.ID, userMsg.ID, "../secret"); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}

	// Clearing messages removes on-disk uploads.
	if err := svc.ClearMessages(group.ID, 1); err != nil {
		t.Fatalf("ClearMessages: %v", err)
	}
	if _, _, _, err := svc.AttachmentFile(group.ID, userMsg.ID, userMsg.Attachments[1].StoredName); err == nil {
		t.Fatal("expected attachment to be deleted after ClearMessages")
	}
}

func TestGroupServiceGlobalUpdatesAndReplayOnDisconnect(t *testing.T) {
	store := newTestGroupStore(t)
	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat/stream" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "event: tool\ndata: {\"id\":\"t1\",\"name\":\"bash\",\"arguments\":\"{\\\"command\\\":\\\"sleep 5\\\"}\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		<-release
		fmt.Fprint(w, "event: tool_result\ndata: {\"id\":\"t1\",\"name\":\"bash\",\"result\":\"ok\",\"is_error\":false}\n\n")
		fmt.Fprint(w, "event: chunk\ndata: {\"text\":\"Done sleeping.\"}\n\n")
		fmt.Fprint(w, "event: done\ndata: {\"ok\":true,\"text\":\"Done sleeping.\",\"conversation_id\":\"conv-g1\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	dir := newMockAgentDirectory(
		AgentInfo{Name: "default", Running: true, BaseURL: srv.URL},
	)
	svc := NewGroupServiceWithStore(store, dir, srv.Client())
	group, err := svc.Create(CreateGroupRequest{
		Name:           "Replay Room",
		Members:        []string{"default"},
		DiscussionMode: DiscussionModeAutonomous,
		MaxTurns:       8,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	globalCh, unsubGlobal := svc.SubscribeGlobal()
	defer unsubGlobal()

	// Start turn.
	if _, _, err := svc.SendMessage(group.ID, 1, SendGroupMessageRequest{Text: "sleep 5"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Wait until tool_start is buffered (simulating mid-turn refresh).
	ch1, init1, unsub1, err := svc.Subscribe(group.ID)
	if err != nil {
		t.Fatalf("Subscribe #1: %v", err)
	}
	if !init1.Running {
		t.Fatalf("expected running=true on Subscribe #1")
	}
	sawTool := false
	deadline := time.After(3 * time.Second)
	for !sawTool {
		select {
		case ev := <-ch1:
			if ev.Name == "tool_start" {
				sawTool = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for tool_start")
		}
	}
	// Disconnect group stream (simulating refresh + switching to another agent).
	unsub1()

	// Re-subscribe mid-turn to verify replay buffer has turn_start + tool_start.
	ch2, _, unsub2, err := svc.Subscribe(group.ID)
	if err != nil {
		t.Fatalf("Subscribe #2: %v", err)
	}
	replayedNames := map[string]bool{}
	for len(replayedNames) < 2 {
		select {
		case ev := <-ch2:
			replayedNames[ev.Name] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("expected replayed turn_start+tool_start, got %v", replayedNames)
		}
	}
	// Disconnect again (user switched to another agent before agent finished).
	unsub2()

	// Let the agent finish while ZERO group subscribers are connected.
	close(release)

	// Global subscriber (used by GET /events) must receive both message and status(running=false).
	sawAgentMsg := false
	sawStopped := false
	globalDeadline := time.After(3 * time.Second)
	for !(sawAgentMsg && sawStopped) {
		select {
		case ev := <-globalCh:
			data, _ := ev.Data.(map[string]any)
			if ev.Name == "message" {
				if msg, ok := data["message"].(GroupMessage); ok && msg.SenderType == "agent" && msg.Content == "Done sleeping." {
					sawAgentMsg = true
				}
			}
			if ev.Name == "status" {
				if rs, ok := data["run_state"].(GroupRunState); ok && !rs.Running {
					sawStopped = true
				}
			}
		case <-globalDeadline:
			t.Fatalf("global stream missing events: sawAgentMsg=%v sawStopped=%v", sawAgentMsg, sawStopped)
		}
	}

	// Persisted group must contain the agent's reply and running=false.
	detail, err := svc.Get(group.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail.RunState.Running {
		t.Fatalf("expected RunState.Running=false after completion")
	}
	if len(detail.Messages) != 2 || detail.Messages[1].Content != "Done sleeping." {
		t.Fatalf("unexpected persisted messages: %+v", detail.Messages)
	}
	// Agent reply arrived after user's message, so persistent unread_count must be 1
	// and first_unread_message_id must point to that agent reply.
	if detail.UnreadCount != 1 || detail.FirstUnreadMessageID != detail.Messages[1].ID {
		t.Fatalf("unread = (%d, %q), want (1, %q)", detail.UnreadCount, detail.FirstUnreadMessageID, detail.Messages[1].ID)
	}
	// List() must report the exact same persistent unread count.
	list, err := svc.List()
	if err != nil || len(list) != 1 || list[0].UnreadCount != 1 || list[0].FirstUnreadMessageID != detail.Messages[1].ID {
		t.Fatalf("List unread = %+v, err=%v", list, err)
	}
	// MarkRead persists the watermark and clears unread_count across subsequent reads.
	if _, _, err := svc.MarkRead(group.ID, ""); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	afterRead, err := svc.Get(group.ID)
	if err != nil {
		t.Fatalf("Get after MarkRead: %v", err)
	}
	if afterRead.UnreadCount != 0 || afterRead.FirstUnreadMessageID != "" {
		t.Fatalf("after MarkRead unread = (%d, %q), want (0, \"\")", afterRead.UnreadCount, afterRead.FirstUnreadMessageID)
	}
}

func TestGroupWorkingDirAttachLockResolveAndDispatch(t *testing.T) {
	projectDir := t.TempDir()
	otherDir := t.TempDir()
	t.Setenv("MYAAW_ALLOW_HOST_DIRS", "1")
	t.Setenv("MYAAW_WORKSPACE_ROOTS", filepath.Dir(projectDir))

	var (
		mu             sync.Mutex
		seenWorkingDir string
		seenPrompt     string
	)
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat/stream" {
			http.NotFound(w, r)
			return
		}
		var req agentChatStreamRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		seenWorkingDir = req.WorkingDir
		seenPrompt = req.Text
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: conversation\ndata: {\"conversation_id\":\"conv-wd-1\"}\n\n")
		fmt.Fprint(w, "event: done\ndata: {\"text\":\"Checked project! [DONE]\",\"conversation_id\":\"conv-wd-1\"}\n\n")
	}))
	defer agentServer.Close()

	dir := newMockAgentDirectory(
		AgentInfo{Name: "coder", AgentColor: "bleu", Running: true, BaseURL: agentServer.URL},
	)
	svc := NewGroupServiceWithStore(newTestGroupStore(t), dir, agentServer.Client())

	// Create group without a project first, then attach working_dir via PATCH/Update.
	created, err := svc.Create(CreateGroupRequest{
		Name:    "Project Team",
		Members: []string{"coder"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.WorkingDir != "" {
		t.Fatalf("initial WorkingDir = %q, want empty", created.WorkingDir)
	}

	updated, err := svc.Update(created.ID, UpdateGroupRequest{WorkingDir: &projectDir})
	if err != nil {
		t.Fatalf("Update attach working_dir: %v", err)
	}
	if updated.WorkingDir == "" || !updated.WorkingDirExists {
		t.Fatalf("expected hydrated WorkingDir & WorkingDirExists=true, got %+v", updated)
	}

	// Immutable once attached (same rule as DM chats).
	if _, err := svc.Update(created.ID, UpdateGroupRequest{WorkingDir: &otherDir}); !errors.Is(err, ErrWorkingDirLocked) {
		t.Fatalf("expected ErrWorkingDirLocked on change, got %v", err)
	}
	empty := ""
	if _, err := svc.Update(created.ID, UpdateGroupRequest{WorkingDir: &empty}); !errors.Is(err, ErrWorkingDirLocked) {
		t.Fatalf("expected ErrWorkingDirLocked on clear, got %v", err)
	}

	// ConversationService resolves "group:<id>" transparently for shared /fs, /git, and /terminal.
	convSvc := NewConversationService()
	convSvc.SetGroupWorkingDirResolver(svc.ResolveWorkingDir)
	resolved, err := convSvc.ResolveWorkingDir("coder", "group:"+created.ID, 1)
	if err != nil {
		t.Fatalf("ResolveWorkingDir(group): %v", err)
	}
	if resolved != updated.WorkingDir {
		t.Fatalf("ResolveWorkingDir = %q, want %q", resolved, updated.WorkingDir)
	}
	// Fiber path params pass percent-encoded colons ("group%3A<id>") on /git/* routes.
	resolvedEscaped, err := convSvc.ResolveWorkingDir("coder", "group%3A"+created.ID, 1)
	if err != nil {
		t.Fatalf("ResolveWorkingDir(escaped group): %v", err)
	}
	if resolvedEscaped != updated.WorkingDir {
		t.Fatalf("ResolveWorkingDir(escaped) = %q, want %q", resolvedEscaped, updated.WorkingDir)
	}

	// Sending a group message forwards working_dir to member agents' /api/chat/stream.
	ch, _, unsub, err := svc.Subscribe(created.ID)
	defer unsub()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if _, _, err := svc.SendMessage(created.ID, 1, SendGroupMessageRequest{Text: "check repo"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	waitForRunEnd(t, ch, 3*time.Second)

	mu.Lock()
	gotWD := seenWorkingDir
	gotPrompt := seenPrompt
	mu.Unlock()
	if gotWD != updated.WorkingDir {
		t.Fatalf("agent stream working_dir = %q, want %q", gotWD, updated.WorkingDir)
	}
	if !strings.Contains(gotPrompt, "Shared project working directory: "+updated.WorkingDir) {
		t.Fatalf("group prompt missing shared working_dir context:\n%s", gotPrompt)
	}
}
