package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"myaaw/internal/agent/approvals"
	"myaaw/internal/channel"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestApprovalKey(t *testing.T) {
	cases := map[string]approvals.Decision{
		"y":   approvals.AllowOnce,
		"Y":   approvals.AllowOnce,
		"s":   approvals.AllowSession,
		"a":   approvals.AllowAlways,
		"n":   approvals.Deny,
		"esc": approvals.Deny,
	}
	for key, want := range cases {
		got, ok := approvalKey(key)
		if !ok || got != want {
			t.Errorf("approvalKey(%q) = %q, %v; want %q", key, got, ok, want)
		}
	}
	for _, key := range []string{"x", "enter", ""} {
		if _, ok := approvalKey(key); ok {
			t.Errorf("approvalKey(%q) should not be recognized", key)
		}
	}
}

func TestRenderApprovalPromptHasNoHintLine(t *testing.T) {
	// The key hints live in the bottom status bar; repeating them inside the
	// prompt rendered the same sentence twice on short terminals.
	out := renderApprovalPrompt(approvals.Request{
		ToolName:    "bash",
		ArgsSummary: `{"command":"ls"}`,
	}, 80, 0, false, 0, approvalArgsWindow)
	if strings.Contains(out, "Enter confirm") || strings.Contains(out, "←/→ select") {
		t.Errorf("approval prompt repeats the footer hints:\n%s", out)
	}
	if !strings.Contains(out, "Allow once") {
		t.Errorf("approval prompt is missing its options:\n%s", out)
	}
}

func TestRenderApprovalPromptMarksSubAgent(t *testing.T) {
	out := renderApprovalPrompt(approvals.Request{
		ToolName: "bash",
		SubAgent: "download-x-photo",
	}, 80, 0, false, 0, approvalArgsWindow)
	if !strings.Contains(out, "from sub-agent download-x-photo") {
		t.Errorf("prompt does not identify the origin sub-agent:\n%s", out)
	}

	main := renderApprovalPrompt(approvals.Request{ToolName: "bash"}, 80, 0, false, 0, approvalArgsWindow)
	if strings.Contains(main, "from sub-agent") {
		t.Errorf("main-agent prompt should not show a sub-agent label:\n%s", main)
	}
}

func TestRenderApprovalPromptExpandedArgs(t *testing.T) {
	req := approvals.Request{
		ToolName: "bash",
		Args:     `{"command":"rm -rf /tmp/x","timeout":60}`,
	}
	collapsed := renderApprovalPrompt(req, 80, 0, false, 0, approvalArgsWindow)
	if strings.Contains(collapsed, "\"timeout\"") {
		t.Errorf("collapsed prompt should not dump the full arguments:\n%s", collapsed)
	}

	expanded := renderApprovalPrompt(req, 80, 0, true, 0, approvalArgsWindow)
	for _, want := range []string{"args 1-", "\"command\"", "\"timeout\"", "Ctrl+O to collapse"} {
		if !strings.Contains(expanded, want) {
			t.Errorf("expanded prompt missing %q:\n%s", want, expanded)
		}
	}
}

func TestRenderApprovalPromptSanitizesArgs(t *testing.T) {
	req := approvals.Request{
		ToolName: "bash",
		Args:     "{\"command\":\"echo \x1b[2Jpwn\"}",
	}
	out := renderApprovalPrompt(req, 80, 0, true, 0, approvalArgsWindow)
	if strings.Contains(out, "\x1b") {
		t.Errorf("approval prompt leaked a terminal escape sequence:\n%q", out)
	}
}

func TestApprovalArgsToggleByClick(t *testing.T) {
	m := model{
		state:           stateApproval,
		pendingApproval: &approvals.Request{ID: "a1", ToolName: "bash", ArgsSummary: `{"command":"ls"}`},
		textInput:       textinput.New(),
		viewport:        viewport.New(80, 20),
		width:           80,
		height:          40,
	}

	y := m.approvalArgsToggleY()
	if y < 0 {
		t.Fatal("args toggle line not found")
	}
	updated, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: y})
	got := updated.(model)
	if !got.approvalArgsOpen {
		t.Fatal("clicking the args line did not expand it")
	}

	y = got.approvalArgsToggleY()
	updated, _ = got.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: y})
	if updated.(model).approvalArgsOpen {
		t.Fatal("clicking the args line again did not collapse it")
	}
}

func TestApprovalArgsToggleAndScroll(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"command":"ls"`)
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, `,"k%02d":"value"`, i)
	}
	b.WriteString("}")

	m := model{
		state:           stateApproval,
		pendingApproval: &approvals.Request{ID: "a1", ToolName: "bash", Args: b.String()},
		textInput:       textinput.New(),
		viewport:        viewport.New(80, 20),
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	got := updated.(model)
	if !got.approvalArgsOpen {
		t.Fatal("Ctrl+O did not expand the arguments")
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	if got.approvalArgsOffset != 1 {
		t.Fatalf("offset = %d; want 1", got.approvalArgsOffset)
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	max := approvalArgsMaxOffset(*got.pendingApproval, approvalArgsWindow)
	if max == 0 {
		t.Fatal("test arguments should wrap into more than one window")
	}
	if got.approvalArgsOffset > max {
		t.Fatalf("offset = %d exceeds max %d", got.approvalArgsOffset, max)
	}
}

func TestPawTickKeepsRunningDuringApproval(t *testing.T) {
	// The approval prompt suspends the animation, but the tick loop must stay
	// scheduled: answering the prompt restores stateWaiting, and a dead loop
	// would leave the paw frozen for the rest of the response.
	m := model{state: stateApproval}
	if _, cmd := m.Update(pawTickMsg{}); cmd == nil {
		t.Fatal("paw tick stopped while waiting for approval")
	}

	// Once the run is idle there is nothing to animate, so the loop stops.
	m = model{state: stateInput}
	if _, cmd := m.Update(pawTickMsg{}); cmd != nil {
		t.Fatal("paw tick should stop when no run is active")
	}
}

func TestTUIApprovalRequesterIgnoresStaleDecision(t *testing.T) {
	requests := make(chan approvals.Request, 1)
	decisions := make(chan approvalDecision, 4)
	cancelled := make(chan string, 1)
	r := &tuiApprovalRequester{requests: requests, decisions: decisions, cancelled: cancelled}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct {
		decision approvals.Decision
		err      error
	}, 1)
	go func() {
		decision, err := r.Request(ctx, approvals.Request{ID: "new"})
		done <- struct {
			decision approvals.Decision
			err      error
		}{decision, err}
	}()

	if req := <-requests; req.ID != "new" {
		t.Fatalf("request ID = %q", req.ID)
	}
	// An answer left over from an expired prompt must not approve this call.
	decisions <- approvalDecision{ID: "expired", Decision: approvals.AllowAlways}
	decisions <- approvalDecision{ID: "new", Decision: approvals.AllowOnce}

	select {
	case got := <-done:
		if got.err != nil || got.decision != approvals.AllowOnce {
			t.Fatalf("decision = %q, err = %v; want allow_once", got.decision, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("requester never consumed the matching decision")
	}
}

func TestTUIApprovalRequesterReportsCancellation(t *testing.T) {
	requests := make(chan approvals.Request, 1)
	decisions := make(chan approvalDecision, 1)
	cancelled := make(chan string, 1)
	r := &tuiApprovalRequester{requests: requests, decisions: decisions, cancelled: cancelled}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_, _ = r.Request(ctx, approvals.Request{ID: "expired"})
	}()

	if req := <-requests; req.ID != "expired" {
		t.Fatalf("request ID = %q", req.ID)
	}
	cancel()

	select {
	case id := <-cancelled:
		if id != "expired" {
			t.Fatalf("cancelled ID = %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("requester did not report the cancellation")
	}
}

func TestChunkWhileApprovalPendingClearsStalePrompt(t *testing.T) {
	m := model{
		state:               stateApproval,
		stateBeforeApproval: stateWaiting,
		pendingApproval:     &approvals.Request{ID: "a1", ToolName: "bash"},
		sub:                 make(chan nextChunkMsg, 1),
		textInput:           textinput.New(),
	}

	updated, cmd := m.Update(nextChunkMsg{chunk: channel.StreamChunk{Kind: "text", Text: "hello"}})
	got := updated.(model)
	if got.state != stateWaiting {
		t.Fatalf("state = %v; want stateWaiting after stale prompt was cleared", got.state)
	}
	if got.pendingApproval != nil {
		t.Fatal("stale approval prompt was not cleared")
	}
	if cmd == nil {
		t.Fatal("chunk reader stopped while the run was still active")
	}
}

func TestChunkKeepsSubAgentApprovalPrompt(t *testing.T) {
	// The parent run keeps streaming while a sub-agent waits for approval, so a
	// parent chunk must not dismiss the prompt.
	m := model{
		state:               stateApproval,
		stateBeforeApproval: stateWaiting,
		pendingApproval:     &approvals.Request{ID: "a1", ToolName: "bash", SubAgent: "download-x-photo"},
		sub:                 make(chan nextChunkMsg, 1),
		textInput:           textinput.New(),
	}

	updated, cmd := m.Update(nextChunkMsg{chunk: channel.StreamChunk{Kind: "text", Text: "hello"}})
	got := updated.(model)
	if got.state != stateApproval || got.pendingApproval == nil {
		t.Fatal("sub-agent prompt was dismissed by parent output")
	}
	if cmd == nil {
		t.Fatal("chunk reader stopped while the run was still active")
	}
}

func TestDoneKeepsSubAgentApprovalPrompt(t *testing.T) {
	m := model{
		state:               stateApproval,
		stateBeforeApproval: stateWaiting,
		pendingApproval:     &approvals.Request{ID: "a1", ToolName: "bash", SubAgent: "download-x-photo"},
		streamingItems:      []timelineItem{{Kind: "text", Text: "done"}},
		viewport:            viewport.New(80, 20),
		textInput:           textinput.New(),
	}

	updated, _ := m.Update(nextChunkMsg{done: true, finalText: "done"})
	got := updated.(model)
	if len(got.messages) != 1 {
		t.Fatalf("messages = %d; want the final answer to be kept", len(got.messages))
	}
	if got.state != stateApproval || got.pendingApproval == nil {
		t.Fatal("sub-agent prompt should survive the end of the parent run")
	}
	if got.stateBeforeApproval != stateInput {
		t.Fatalf("stateBeforeApproval = %v; want stateInput once the run is over", got.stateBeforeApproval)
	}
}

func TestConcurrentApprovalsAreQueued(t *testing.T) {
	m := model{
		state:               stateApproval,
		stateBeforeApproval: stateWaiting,
		pendingApproval:     &approvals.Request{ID: "a1", ToolName: "bash"},
		approvalRequests:    make(chan approvals.Request, 1),
		viewport:            viewport.New(80, 20),
		textInput:           textinput.New(),
	}

	updated, _ := m.Update(approvalRequestMsg{req: approvals.Request{ID: "a2", ToolName: "filesystem", SubAgent: "child"}})
	got := updated.(model)
	if got.pendingApproval == nil || got.pendingApproval.ID != "a1" {
		t.Fatal("the visible prompt was replaced by a concurrent request")
	}
	if len(got.approvalQueue) != 1 || got.approvalQueue[0].ID != "a2" {
		t.Fatalf("approval queue = %+v", got.approvalQueue)
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	got = updated.(model)
	if got.pendingApproval == nil || got.pendingApproval.ID != "a2" {
		t.Fatal("the queued approval was not shown after answering the first one")
	}
	if got.state != stateApproval {
		t.Fatalf("state = %v; want stateApproval", got.state)
	}
}

func TestDoneWhileApprovalPendingFinalizes(t *testing.T) {
	m := model{
		state:               stateApproval,
		stateBeforeApproval: stateWaiting,
		pendingApproval:     &approvals.Request{ID: "a1", ToolName: "bash"},
		streamingItems:      []timelineItem{{Kind: "text", Text: "done"}},
		viewport:            viewport.New(80, 20),
		textInput:           textinput.New(),
	}

	updated, _ := m.Update(nextChunkMsg{done: true, finalText: "done"})
	got := updated.(model)
	if got.state != stateInput {
		t.Fatalf("state = %v; want stateInput after the run finished", got.state)
	}
	if got.pendingApproval != nil {
		t.Fatal("approval prompt survived the finished run")
	}
	if len(got.messages) != 1 {
		t.Fatalf("messages = %d; want the final answer to be kept", len(got.messages))
	}
}
