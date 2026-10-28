package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"myaaw/internal/obs"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	obsSummaryDays   int
	obsPruneDays     int
	obsJSON          bool
	obsRunsLimit     int
	obsSessionsLimit int
	obsBefore        string
	obsChannel       string
	obsStatus        string

	obsConversation string
)

var observabilityCmd = &cobra.Command{
	Use:     "observability",
	Aliases: []string{"obs"},
	Short:   "Inspect recorded agent traces, token usage, and cost",
	Long: `Read the local trace database (database/observability.db).

Every run is stored as a trace: one root span per inbound message (one user
turn), a step span per loop iteration, and llm/tool spans underneath. Token
usage and model cost are recorded per model, so switching models
mid-conversation bills each request correctly.`,
}

var observabilitySummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Totals, latency, cost, and per-step breakdown",
	Example: `  myaaw observability summary
  myaaw observability summary --days 30
  myaaw observability summary --json`,
	RunE: runObservabilitySummary,
}

var observabilityRunCmd = &cobra.Command{
	Use:   "run <run-id | session.type.turn>",
	Short: "Show one trace tree",
	Args:  cobra.ExactArgs(1),
	Example: `  myaaw observability run run_1a2b3c4d5e6f7a8b
  myaaw observability run 4e7371da.cli.2`,
	RunE: runObservabilityRun,
}

var observabilitySessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List sessions (conversations with recorded turns)",
	Long: `A session is the set of runs that share a conversation id. Pick one with the
arrow keys and press Enter to see its turns.`,
	RunE: runObservabilitySessions,
}

var observabilitySessionCmd = &cobra.Command{
	Use:   "session <conversation-id>",
	Short: "Show every turn of one session",
	Long: `A session is the set of runs that share a conversation id. Each user message is
one turn, stored as its own run, so a session with three chats has three runs.`,
	Args: cobra.ExactArgs(1),
	RunE: runObservabilitySession,
}

var observabilityPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete old traces",
	Example: `  myaaw observability prune --days 30
  myaaw observability prune --before 2026-01-01`,
	RunE: runObservabilityPrune,
}

var observabilityRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "List recent runs",
	Example: `  myaaw observability runs
  myaaw observability runs --limit 20 --status error`,
	RunE: runObservabilityRuns,
}

func init() {
	observabilitySummaryCmd.Flags().IntVar(&obsSummaryDays, "days", 7, "window in days")
	observabilitySummaryCmd.Flags().BoolVar(&obsJSON, "json", false, "print raw JSON")

	observabilityRunCmd.Flags().BoolVar(&obsJSON, "json", false, "print raw JSON")

	observabilityRunsCmd.Flags().IntVar(&obsRunsLimit, "limit", 20, "how many runs to show")
	observabilityRunsCmd.Flags().StringVar(&obsChannel, "channel", "", "filter by channel")
	observabilityRunsCmd.Flags().StringVar(&obsStatus, "status", "", "filter by status")
	observabilityRunsCmd.Flags().StringVar(&obsConversation, "conversation", "", "filter by conversation id (one session)")
	observabilityRunsCmd.Flags().BoolVar(&obsJSON, "json", false, "print raw JSON")

	observabilitySessionsCmd.Flags().IntVar(&obsSessionsLimit, "limit", 50, "how many sessions to list")
	observabilitySessionsCmd.Flags().BoolVar(&obsJSON, "json", false, "print raw JSON")

	observabilityPruneCmd.Flags().IntVar(&obsPruneDays, "days", 0, "delete traces older than this many days")
	observabilityPruneCmd.Flags().StringVar(&obsBefore, "before", "", "delete traces older than this date (YYYY-MM-DD)")

	observabilityCmd.AddCommand(observabilitySummaryCmd)
	observabilityCmd.AddCommand(observabilityRunsCmd)
	observabilityCmd.AddCommand(observabilitySessionsCmd)
	observabilityCmd.AddCommand(observabilitySessionCmd)
	observabilityCmd.AddCommand(observabilityRunCmd)
	observabilityCmd.AddCommand(observabilityPruneCmd)
}

func openObservabilityStore() (*obs.Store, error) {
	path, err := agentdir.ActiveObservabilityPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no traces recorded yet (%s)", path)
	}
	return obs.Open(path, obs.Options{})
}

func runObservabilitySummary(cmd *cobra.Command, args []string) error {
	store, err := openObservabilityStore()
	if err != nil {
		return err
	}
	defer store.Close()

	now := time.Now()
	summary, err := store.Summary(now.AddDate(0, 0, -obsSummaryDays).UnixMilli(), now.UnixMilli())
	if err != nil {
		return err
	}

	if obsJSON {
		return printJSON(summary)
	}

	fmt.Printf("%s\n", theme.RenderPrimary(fmt.Sprintf("Observability · last %d day(s)", obsSummaryDays)))
	fmt.Printf("Runs %d · errors %d (%.1f%%)\n", summary.Runs, summary.Errors, summary.ErrorRate*100)
	fmt.Printf("Tokens input %s · cached %s · output %s\n",
		formatTokens(summary.InputTokens), formatTokens(summary.CacheHitTokens), formatTokens(summary.OutputTokens))
	fmt.Printf("Cost %s · latency p50 %s · p95 %s · TTFT p50 %s\n",
		formatUSD(summary.CostUSD), formatMillis(summary.DurationP50),
		formatMillis(summary.DurationP95), formatMillis(summary.FirstTokenP50))

	if len(summary.PerStep) > 0 {
		fmt.Printf("\n%s\n", theme.RenderPrimary("Per step"))
		fmt.Printf("  %4s %7s %8s %8s %10s\n", "STEP", "COUNT", "AVG", "P95", "COST")
		for _, step := range summary.PerStep {
			fmt.Printf("  %4d %7d %8s %8s %10s\n", step.StepIndex, step.Count,
				formatMillis(step.AvgMs), formatMillis(step.P95Ms), formatUSD(step.CostUSD))
		}
	}

	if len(summary.ByModel) > 0 {
		fmt.Printf("\n%s\n", theme.RenderPrimary("Per model"))
		fmt.Printf("  %-28s %7s %9s %9s %11s\n", "MODEL", "CALLS", "INPUT", "OUTPUT", "COST")
		for _, model := range summary.ByModel {
			fmt.Printf("  %-28s %7d %9s %9s %11s\n",
				truncateValue(model.Model, 28), model.Calls,
				formatTokens(model.InputTokens), formatTokens(model.OutputTokens), formatUSD(model.CostUSD))
		}
	}

	if len(summary.PerTool) > 0 {
		fmt.Printf("\n%s\n", theme.RenderPrimary("Per tool"))
		fmt.Printf("  %-32s %6s %7s %9s\n", "TOOL", "CALLS", "ERRORS", "AVG")
		for _, tool := range summary.PerTool {
			fmt.Printf("  %-32s %6d %7d %9s\n",
				truncateValue(tool.Name, 32), tool.Calls, tool.Errors, formatMillis(tool.AvgMs))
		}
	}

	if summary.MostExpensiveRunID != "" || summary.SlowestRunID != "" {
		fmt.Printf("\n%s\n", theme.RenderPrimary("Highlights"))
		if summary.MostExpensiveRunID != "" {
			fmt.Printf("  %-16s %s\n", "most expensive", theme.RenderSecondary(summary.MostExpensiveRunID))
		}
		if summary.SlowestRunID != "" {
			fmt.Printf("  %-16s %s\n", "slowest", theme.RenderSecondary(summary.SlowestRunID))
		}
	}
	return nil
}

func runObservabilityRuns(cmd *cobra.Command, args []string) error {
	store, err := openObservabilityStore()
	if err != nil {
		return err
	}
	defer store.Close()

	runs, total, err := store.ListRuns(obs.RunFilter{
		Limit:          obsRunsLimit,
		Status:         obsStatus,
		Channel:        obsChannel,
		ConversationID: obsConversation,
	})
	if err != nil {
		return err
	}

	if obsJSON {
		return printJSON(runs)
	}

	if len(runs) == 0 {
		fmt.Println("No runs recorded.")
		return nil
	}

	// A terminal gets a picker; a pipe or a script keeps the plain table.
	if term.IsTerminal(int(os.Stdout.Fd())) {
		selected, ok := pickRun(runs)
		if !ok {
			return nil
		}
		return runObservabilityRun(cmd, []string{selected.ID})
	}

	fmt.Printf("%s (%d total)\n", theme.RenderPrimary("Runs"), total)
	fmt.Printf("  %-22s %-19s %-10s %-9s %-9s %s\n", "RUN", "STARTED", "CHANNEL", "STATUS", "DURATION", "COST")
	for _, run := range runs {
		fmt.Printf("  %-22s %-19s %-10s %-9s %-9s %s\n",
			run.ID, time.UnixMilli(run.StartedAt).Format("2006-01-02 15:04:05"),
			run.Channel, run.Status, formatMillis(run.TotalMs), formatUSD(run.ModelCostUSD))
	}
	return nil
}

func runObservabilityRun(cmd *cobra.Command, args []string) error {
	store, err := openObservabilityStore()
	if err != nil {
		return err
	}
	defer store.Close()

	runID := args[0]
	if !strings.HasPrefix(runID, "run_") {
		resolved, err := resolveTurnRef(store, runID)
		if err != nil {
			return err
		}
		runID = resolved.ID
	}

	run, spans, err := store.GetRun(runID)
	if err != nil {
		return err
	}

	if obsJSON {
		return printJSON(map[string]any{"run": run, "spans": spans})
	}

	status := string(run.Status)
	if run.Status == obs.StatusRunning {
		status = "running (in progress)"
	}
	fmt.Printf("%s %s\n", theme.RenderPrimary("Run"), run.ID)
	fmt.Printf("  %s · %s · %s\n", run.Channel, run.Model, status)
	duration := formatMillis(run.TotalMs)
	if run.Status == obs.StatusRunning && run.TotalMs > 0 {
		duration += " so far"
	}
	fmt.Printf("  duration %s · TTFT %s · tokens %s in / %s out · cost %s\n",
		duration, formatMillis(run.FirstTokenMs),
		formatTokens(run.InputTokens), formatTokens(run.OutputTokens), costLabel(run))
	if run.ModelCostUSD == 0 && (run.InputTokens > 0 || run.OutputTokens > 0) {
		fmt.Printf("  cost missing: %q had no price when this run was recorded\n", run.Model)
	}
	if run.Error != "" {
		fmt.Printf("  error: %s\n", run.Error)
	}
	if run.ConversationID != "" {
		fmt.Printf("  conversation %s · user %s\n", run.ConversationID, run.UserID)
	}
	steps, requests, tools := spanCounts(run, spans)
	fmt.Printf("  %d step(s) · %d model request(s) · %d tool call(s)\n", steps, requests, tools)
	fmt.Println()

	children := map[string][]obs.Span{}
	roots := make([]obs.Span, 0, 4)
	for _, span := range spans {
		if span.ID == run.ID {
			continue
		}
		if span.ParentID == "" || span.ParentID == run.ID {
			roots = append(roots, span)
			continue
		}
		children[span.ParentID] = append(children[span.ParentID], span)
	}

	var printSpan func(span obs.Span, depth int)
	printSpan = func(span obs.Span, depth int) {
		indent := strings.Repeat("  ", depth+1)
		label := spanLabel(span)
		line := fmt.Sprintf("%s%-40s %9s", indent, truncate(label, 40), spanDuration(run, span))
		if span.FirstTokenMs > 0 {
			line += fmt.Sprintf("  ttft %s", formatMillis(span.FirstTokenMs))
		}
		if span.InputTokens > 0 || span.OutputTokens > 0 {
			line += fmt.Sprintf("  %s/%s tok", formatTokens(span.InputTokens), formatTokens(span.OutputTokens))
		}
		if span.CostUSD > 0 {
			line += fmt.Sprintf("  %s", formatUSD(span.CostUSD))
		}
		if span.Status != "" && span.Status != obs.StatusCompleted {
			line += fmt.Sprintf("  [%s]", span.Status)
		}
		fmt.Println(line)
		for _, child := range children[span.ID] {
			printSpan(child, depth+1)
		}
	}

	for _, root := range roots {
		printSpan(root, 0)
	}
	return nil
}

func runObservabilitySessions(cmd *cobra.Command, args []string) error {
	store, err := openObservabilityStore()
	if err != nil {
		return err
	}
	defer store.Close()

	sessions, err := store.ListSessions(obsSessionsLimit)
	if err != nil {
		return err
	}

	if obsJSON {
		return printJSON(sessions)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions recorded.")
		return nil
	}

	if term.IsTerminal(int(os.Stdout.Fd())) {
		selected, ok := pickSession(sessions)
		if !ok {
			return nil
		}
		return runObservabilitySession(cmd, []string{selected.ConversationID})
	}

	fmt.Printf("%s (%d shown)\n", theme.RenderPrimary("Sessions"), len(sessions))
	fmt.Printf("  %-38s %-6s %-19s %-14s %s\n", "CONVERSATION", "TURNS", "LAST ACTIVITY", "TOKENS", "COST")
	for _, session := range sessions {
		fmt.Printf("  %-38s %-6d %-19s %-14s %s\n",
			session.ConversationID, session.Turns,
			time.UnixMilli(session.LastActiveAt).Format("2006-01-02 15:04:05"),
			fmt.Sprintf("%s/%s", formatTokens(session.InputTokens), formatTokens(session.OutputTokens)),
			formatUSD(session.CostUSD))
	}
	return nil
}

func runObservabilitySession(cmd *cobra.Command, args []string) error {
	store, err := openObservabilityStore()
	if err != nil {
		return err
	}
	defer store.Close()

	conversationID := args[0]
	runs, _, err := store.ListRuns(obs.RunFilter{ConversationID: conversationID, Limit: 500})
	if err != nil {
		return err
	}

	if obsJSON {
		return printJSON(runs)
	}

	if len(runs) == 0 {
		fmt.Printf("No runs recorded for conversation %s.\n", conversationID)
		return nil
	}

	// ListRuns returns newest first; a session reads in turn order.
	for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
		runs[i], runs[j] = runs[j], runs[i]
	}

	fmt.Printf("%s %s · %d turn(s)\n", theme.RenderPrimary("Session"), conversationID, len(runs))
	fmt.Printf("  %-4s %-8s %-10s %-22s %-22s %-5s %-8s %-8s %-13s %-10s %s\n",
		"TURN", "TYPE", "STARTED", "RUN", "MODEL", "STEPS", "DUR", "TTFT", "TOKENS", "COST", "STATUS")

	totalCost := 0.0
	for i, run := range runs {
		totalCost += run.ModelCostUSD
		fmt.Printf("  %-4d %-8s %-10s %-22s %-22s %-5d %-8s %-8s %-13s %-10s %s\n",
			i+1,
			firstNonEmptyString(run.Channel, "agent"),
			time.UnixMilli(run.StartedAt).Format("15:04:05"),
			run.ID,
			truncateValue(firstNonEmptyString(run.Model, "unknown model"), 22),
			countSpans(store, run, obs.KindStep),
			formatMillis(run.TotalMs),
			formatMillis(run.FirstTokenMs),
			fmt.Sprintf("%s/%s", formatTokens(run.InputTokens), formatTokens(run.OutputTokens)),
			costLabel(run), run.Status)
		if run.Error != "" {
			fmt.Printf("        %s\n", theme.RenderMuted("error: "+run.Error))
		}
	}

	fmt.Printf("\n%d turn(s) · %s total\n", len(runs), formatUSD(totalCost))
	example := formatTurnRef(conversationID, runs[0].Channel, 1)
	fmt.Printf("Open a turn:  %s  or  %s\n",
		theme.RenderSecondary("myaaw obs run <run-id>"),
		theme.RenderSecondary("myaaw obs run "+example))
	fmt.Printf("All runs:     %s\n", theme.RenderSecondary("myaaw obs runs"))
	return nil
}

// formatTurnRef is the short way to address one turn of a session:
// <session prefix>.<channel>.<turn>. The run id stays available, but this reads
// out of a table.
func formatTurnRef(sessionID, channel string, turn int) string {
	return fmt.Sprintf("%s.%s.%d", shortID(sessionID), firstNonEmptyString(channel, "agent"), turn)
}

// resolveTurnRef turns "4e7371da.cli.1" into the run it names.
func resolveTurnRef(store *obs.Store, ref string) (obs.Run, error) {
	parts := strings.Split(ref, ".")
	if len(parts) != 3 {
		return obs.Run{}, fmt.Errorf("%q is not a turn reference (expected <session>.<type>.<turn>)", ref)
	}
	turn, err := strconv.Atoi(parts[2])
	if err != nil || turn < 1 {
		return obs.Run{}, fmt.Errorf("%q has an invalid turn number", ref)
	}

	sessions, err := store.ListSessions(500)
	if err != nil {
		return obs.Run{}, err
	}
	match := ""
	for _, session := range sessions {
		if !strings.HasPrefix(session.ConversationID, parts[0]) {
			continue
		}
		if match != "" {
			return obs.Run{}, fmt.Errorf("session prefix %q matches more than one session; use more characters", parts[0])
		}
		match = session.ConversationID
	}
	if match == "" {
		return obs.Run{}, fmt.Errorf("no session starts with %q", parts[0])
	}

	runs, _, err := store.ListRuns(obs.RunFilter{ConversationID: match, Limit: 500})
	if err != nil {
		return obs.Run{}, err
	}
	// ListRuns is newest first; turns count from the oldest.
	for left, right := 0, len(runs)-1; left < right; left, right = left+1, right-1 {
		runs[left], runs[right] = runs[right], runs[left]
	}
	if turn > len(runs) {
		return obs.Run{}, fmt.Errorf("session %s has %d turn(s), not %d", shortID(match), len(runs), turn)
	}
	return runs[turn-1], nil
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// truncateValue keeps table columns aligned. It counts runes so a non-ASCII
// name cannot be cut mid-character.
func truncateValue(value string, max int) string {
	if max <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max-1]) + "…"
}

// countSpans returns how many spans of a kind belong to one run.
func countSpans(store *obs.Store, run obs.Run, kind obs.SpanKind) int {
	_, spans, err := store.GetRun(run.ID)
	if err != nil {
		return 0
	}
	count := 0
	for _, span := range spans {
		if span.RunID == run.ID && span.Kind == kind {
			count++
		}
	}
	return count
}

func runObservabilityPrune(cmd *cobra.Command, args []string) error {
	store, err := openObservabilityStore()
	if err != nil {
		return err
	}
	defer store.Close()

	var before time.Time
	switch {
	case obsBefore != "":
		parsed, err := time.Parse("2006-01-02", obsBefore)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, obsBefore)
		}
		if err != nil {
			return fmt.Errorf("invalid --before value %q: use YYYY-MM-DD", obsBefore)
		}
		before = parsed
	case obsPruneDays > 0:
		before = time.Now().AddDate(0, 0, -obsPruneDays)
	default:
		return fmt.Errorf("pass --days N or --before YYYY-MM-DD")
	}

	removed, err := store.DeleteBefore(before.UnixMilli())
	if err != nil {
		return err
	}
	fmt.Printf("Removed %d run(s) started before %s.\n", removed, before.Format("2006-01-02 15:04"))
	return nil
}

type runItem struct {
	run obs.Run
}

func (i runItem) Title() string {
	status := string(i.run.Status)
	if i.run.Status == obs.StatusRunning {
		status = "running"
	}
	return fmt.Sprintf("%s · %s · %s",
		time.UnixMilli(i.run.StartedAt).Format("2006-01-02 15:04:05"),
		firstNonEmptyString(i.run.Channel, "agent"), status)
}

func (i runItem) Description() string {
	detail := fmt.Sprintf("%s · %s · %s in / %s out · %s",
		i.run.ID, firstNonEmptyString(i.run.Model, "unknown model"),
		formatTokens(i.run.InputTokens), formatTokens(i.run.OutputTokens), costLabel(i.run))
	if i.run.TotalMs > 0 {
		detail += " · " + formatMillis(i.run.TotalMs)
	}
	return detail
}

func (i runItem) FilterValue() string { return i.run.ID + " " + i.run.Channel + " " + i.run.Model }

type sessionItem struct {
	session obs.Session
}

func (i sessionItem) Title() string {
	return fmt.Sprintf("%s · %d turn(s) · %s",
		time.UnixMilli(i.session.LastActiveAt).Format("2006-01-02 15:04"),
		i.session.Turns, firstNonEmptyString(i.session.Channel, "agent"))
}

func (i sessionItem) Description() string {
	return fmt.Sprintf("%s · %s in / %s out · %s",
		i.session.ConversationID, formatTokens(i.session.InputTokens),
		formatTokens(i.session.OutputTokens), formatUSD(i.session.CostUSD))
}

func (i sessionItem) FilterValue() string { return i.session.ConversationID }

type runPickerModel struct {
	list   list.Model
	chosen int
	quit   bool
}

func (m runPickerModel) Init() tea.Cmd { return nil }

func (m runPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.chosen = -1
			m.quit = true
			return m, tea.Quit
		case "enter":
			m.chosen = m.list.Index()
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width-4, msg.Height-6)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m runPickerModel) View() string {
	if m.quit {
		return ""
	}
	return m.list.View()
}

// pickRun lets the user walk the list with the arrow keys and open one run with
// Enter.
func pickRun(runs []obs.Run) (obs.Run, bool) {
	items := make([]list.Item, 0, len(runs))
	for _, run := range runs {
		items = append(items, runItem{run: run})
	}
	index, ok := pickFromList("Runs · ↑/↓ to move, Enter to open", items)
	if !ok {
		return obs.Run{}, false
	}
	return runs[index], true
}

// pickSession lets the user choose a session with the arrow keys.
func pickSession(sessions []obs.Session) (obs.Session, bool) {
	items := make([]list.Item, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, sessionItem{session: session})
	}
	index, ok := pickFromList("Sessions · ↑/↓ to move, Enter to open", items)
	if !ok {
		return obs.Session{}, false
	}
	return sessions[index], true
}

// pickFromList runs the interactive picker and returns the chosen index.
func pickFromList(title string, items []list.Item) (int, bool) {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = theme.HighlightStyle.
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color(theme.ColorPrimary)).
		Foreground(lipgloss.Color(theme.ColorPrimary)).
		Padding(0, 0, 0, 1)
	delegate.Styles.SelectedDesc = theme.SecondaryStyle.
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color(theme.ColorPrimary)).
		Foreground(lipgloss.Color(theme.ColorSecondary)).
		Padding(0, 0, 0, 1)

	picker := list.New(items, delegate, 100, 20)
	picker.Title = title
	picker.Styles.Title = theme.HeaderStyle
	picker.SetFilteringEnabled(false)
	picker.SetShowStatusBar(false)

	final, err := tea.NewProgram(runPickerModel{list: picker, chosen: -1}).Run()
	if err != nil {
		return 0, false
	}
	state, ok := final.(runPickerModel)
	if !ok || state.chosen < 0 || state.chosen >= len(items) {
		return 0, false
	}
	return state.chosen, true
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func printJSON(value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

// spanCounts counts the run's own spans: a turn is one loop iteration, and a
// request or tool is a child of one. Nested runs are excluded because their
// spans belong to the child run.
func spanCounts(run obs.Run, spans []obs.Span) (steps, requests, tools int) {
	for _, span := range spans {
		if span.RunID != run.ID {
			continue
		}
		switch span.Kind {
		case obs.KindStep:
			steps++
		case obs.KindLLM:
			requests++
		case obs.KindTool:
			tools++
		}
	}
	return steps, requests, tools
}

// spanLabel avoids "step · Step 1" when the span name already reads as a label.
// A step is always named by its index, which also normalises traces recorded
// before the turn-to-step rename.
func spanLabel(span obs.Span) string {
	if span.Kind == obs.KindStep && span.StepIndex > 0 {
		return fmt.Sprintf("Step %d", span.StepIndex)
	}
	if span.Name == "" {
		return string(span.Kind)
	}
	if strings.HasPrefix(strings.ToLower(span.Name), string(span.Kind)) {
		return span.Name
	}
	return string(span.Kind) + " · " + span.Name
}

// spanDuration shows a running span's elapsed time instead of an empty dash, so
// a live trace stays readable.
func spanDuration(run obs.Run, span obs.Span) string {
	if span.Status == obs.StatusRunning && run.Status == obs.StatusRunning {
		elapsed := time.Now().UnixMilli() - span.StartedAt
		if elapsed > 0 {
			return formatMillis(elapsed) + " so far"
		}
	}
	return formatMillis(span.DurationMs)
}

func formatMillis(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.2fs", float64(ms)/1000)
}

func formatTokens(tokens int) string {
	switch {
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	case tokens >= 1_000:
		return fmt.Sprintf("%.1fk", float64(tokens)/1_000)
	default:
		return strconv.Itoa(tokens)
	}
}

func formatUSD(value float64) string {
	if value <= 0 {
		return "-"
	}
	if value < 0.01 {
		return fmt.Sprintf("$%.6f", value)
	}
	return fmt.Sprintf("$%.4f", value)
}

// costLabel marks an unpriced model instead of showing a misleading zero.
func costLabel(run obs.Run) string {
	if run.ModelCostUSD == 0 && (run.InputTokens > 0 || run.OutputTokens > 0) {
		return "unpriced"
	}
	return formatUSD(run.ModelCostUSD)
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max-1] + "…"
}
