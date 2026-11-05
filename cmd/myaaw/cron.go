package main

import (
	"fmt"
	"log"
	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/cron"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var cronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Manage scheduled jobs",
	Long:  "Manage scheduled jobs (list, add, remove, run manually).",
}

var cronListCmd = &cobra.Command{
	Use:   "list",
	Short: "List jobs, or show one job in full",
	Long: `List every scheduled job with its id, schedule, and status.

Pass a job id (or its prefix) to read that job in full: schedule, prompt,
delivery target, and timestamps. Its runs live in 'myaaw cron history <id>'.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		store := getStore(cronAgent(cmd))
		jobs, err := store.Load()
		if err != nil {
			log.Fatalf("Failed to load jobs: %v", err)
		}

		if len(jobs) == 0 {
			fmt.Println(theme.RenderMuted("No scheduled jobs found."))
			return
		}

		if len(args) == 1 {
			showCronJob(store, jobs, args[0])
			return
		}

		rows := make([][]string, 0, len(jobs))
		for _, job := range jobs {
			schedule := job.Schedule.Expr
			if job.Schedule.Kind == "every" {
				schedule = "@every " + job.Schedule.Expr
			}
			enabled := theme.RenderSuccess("Yes")
			if !job.Enabled {
				enabled = theme.RenderError("No")
			}
			rows = append(rows, []string{
				shortID(job.ID),
				job.Name,
				schedule,
				job.UpdatedAt.Format("2006-01-02 15:04"),
				enabled,
			})
		}
		fmt.Print(cron.AlignTable(headerCells("ID", "Name", "Schedule", "Last Updated", "Enabled"), rows))
	},
}

// showCronJob prints one job in full, resolving an id or prefix against the store.
func showCronJob(store *cron.Store, jobs []cron.Job, arg string) {
	matches := cron.MatchJobIDs(jobs, arg)
	switch len(matches) {
	case 0:
		fmt.Println(theme.RenderError(fmt.Sprintf("Job not found: %s", arg)))
		return
	case 1:
		job, err := store.Get(matches[0])
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to load job: %v", err)))
			return
		}
		fmt.Print(cron.JobDetail(*job))
	default:
		reportAmbiguousID(arg, matches)
	}
}

// reportAmbiguousID lists the candidates when a prefix matches more than one job.
var cronAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new scheduled job",
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		expr, _ := cmd.Flags().GetString("cron") // or every
		every, _ := cmd.Flags().GetString("every")
		at, _ := cmd.Flags().GetString("at")
		message, _ := cmd.Flags().GetString("message")
		channel, _ := cmd.Flags().GetString("channel")
		to, _ := cmd.Flags().GetString("to")
		threadID, _ := cmd.Flags().GetString("thread-id")
		sessionID, _ := cmd.Flags().GetString("session-id")
		chatType, _ := cmd.Flags().GetString("chat-type")
		tz, _ := cmd.Flags().GetString("tz")
		agentID := cronAgent(cmd)

		if name == "" || message == "" || channel == "" || to == "" {
			fmt.Println(theme.RenderError("Missing required flags: --name, --message, --channel, --to"))
			return
		}

		// chat_type only distinguishes a Discord DM from a channel. Telegram encodes
		// the room kind in the chat id, so storing it there would only mislead the
		// agent's <userinfo> block.
		chatType = strings.TrimSpace(chatType)
		if !strings.EqualFold(channel, "discord") {
			chatType = ""
		}

		// A web or terminal target has no platform room id, so an unpinned job
		// could not resolve a session at run time. Reject it here instead of
		// storing a job that can only fail.
		if (strings.EqualFold(channel, "api") || strings.EqualFold(channel, "cli")) && strings.TrimSpace(sessionID) == "" {
			fmt.Println(theme.RenderError("--session-id is required for api/cli targets"))
			return
		}

		kind := "cron"
		scheduleExpr := expr

		// Validate mutual exclusivity
		count := 0
		if expr != "" {
			count++
		}
		if every != "" {
			count++
		}
		if at != "" {
			count++
		}

		if count > 1 {
			fmt.Println(theme.RenderError("Please provide only one of: --cron, --every, --at"))
			return
		} else if count == 0 {
			fmt.Println(theme.RenderError("Missing required flag: --cron OR --every OR --at"))
			return
		}

		if every != "" {
			kind = "every"
			scheduleExpr = every
		} else if at != "" {
			kind = "at"
			// Try to parse relative time and convert to absolute immediately
			// This prevents the job from resetting its timer on every reload
			duration, err := time.ParseDuration(at)
			if err == nil {
				// It's a relative time (e.g. "10m")
				scheduleExpr = time.Now().Add(duration).Format(time.RFC3339)
			} else {
				// It's likely absolute, but let's validate/normalize it anyway if we can
				// For now, trusting user input or letting scheduler fail
				scheduleExpr = at
			}
		}

		job := cron.Job{
			ID:      uuid.New().String(),
			Name:    name,
			AgentID: agentID,
			Schedule: cron.Schedule{
				Kind: kind,
				Expr: scheduleExpr,
				Tz:   tz,
			},
			Payload: cron.Payload{
				Kind:    "agentTurn",
				Content: message,
			},
			Delivery: cron.Delivery{
				Mode:      "announce",
				Channel:   channel,
				To:        to,
				ThreadID:  threadID,
				SessionID: sessionID,
				ChatType:  chatType,
			},
			Enabled:   true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		store := getStore(agentID)
		if err := store.Add(job); err != nil {
			log.Fatalf("Failed to add job: %v", err)
		}

		fmt.Println(theme.RenderSuccess("✅ Job added successfully!"))
		fmt.Printf("ID: %s\n", job.ID)
	},
}

var cronRemoveCmd = &cobra.Command{
	Use:   "remove [id]",
	Short: "Remove a scheduled job",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		store := getStore(cronAgent(cmd))

		// If ID is short, try to find full ID
		if len(id) < 36 {
			jobs, _ := store.Load()
			for _, j := range jobs {
				if len(j.ID) >= len(id) && j.ID[:len(id)] == id {
					id = j.ID
					break
				}
			}
		}

		if err := store.Remove(id); err != nil {
			log.Fatalf("Failed to remove job: %v", err)
		}

		fmt.Println(theme.RenderSuccess("✅ Job removed successfully!"))
	},
}

var cronRunCmd = &cobra.Command{
	Use:   "run [id]",
	Short: "Manually run a scheduled job",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		agent := cronAgent(cmd)
		store := getStore(agent)

		// If ID is short, try to find full ID
		if len(id) < 36 {
			jobs, _ := store.Load()
			for _, j := range jobs {
				if len(j.ID) >= len(id) && j.ID[:len(id)] == id {
					id = j.ID
					break
				}
			}
		}

		history := getHistoryLogger(agent)
		scheduler := cron.NewScheduler(store, history)

		// Load config for MYAAWBaseURL instead of manually setting env
		config.LoadConfig()
		if err := scheduler.Start(); err != nil {
			log.Printf("Warning: scheduler start failed: %v", err)
		}
		defer scheduler.Stop()

		// Wait a bit for scheduler to initialize
		time.Sleep(100 * time.Millisecond)

		if err := scheduler.RunJob(id); err != nil {
			log.Fatalf("Failed to run job: %v", err)
		}

		fmt.Println(theme.RenderSuccess("✅ Job triggered successfully! (Check logs/history for result)"))
	},
}

var cronHistoryCmd = &cobra.Command{
	Use:   "history [id]",
	Short: "List recent runs, or show one job's runs in full",
	Long: `Show recent runs, newest first.

Without an id this is the agent-wide list, and it covers every job, including
one-time jobs that were removed after they ran. --limit shapes that list.

Pass a job id (or its prefix) to see each of that job's runs in full: timestamp,
job, status, result, prompt, and delivery target. A job has no page: every run of
it is shown, so --limit does not apply.`,
	Args: cobra.RangeArgs(0, 1),
	Run: func(cmd *cobra.Command, args []string) {
		agent := cronAgent(cmd)
		store := getStore(agent)
		history := getHistoryLogger(agent)

		if len(args) == 0 {
			limit, _ := cmd.Flags().GetInt("limit")
			showGlobalCronHistory(history, limit)
			return
		}
		showJobCronHistory(store, history, args[0])
	},
}

// showGlobalCronHistory prints the agent-wide run log: one row per run across
// every job, including the one-time jobs that were removed after they ran.
func showGlobalCronHistory(history *cron.HistoryLogger, limit int) {
	runs, _, err := history.GetGlobalHistory(limit, 0)
	if err != nil {
		fmt.Println(theme.RenderError(fmt.Sprintf("Failed to load global history: %v", err)))
		return
	}

	if len(runs) == 0 {
		fmt.Println(theme.RenderMuted("No execution history found."))
		return
	}

	fmt.Println(theme.RenderPrimary(fmt.Sprintf("Recent History (Last %d runs):", limit)))
	rows := make([][]string, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, []string{
			run.Timestamp.Format("2006-01-02 15:04:05"),
			cron.ShortID(run.JobID),
			truncateValue(strings.TrimSpace(run.Name), 28),
			renderCronStatus(run.Status),
			run.Result,
		})
	}
	fmt.Print(cron.AlignTable(headerCells("Time", "Job ID", "Job", "Status", "Result"), rows))
	fmt.Println(theme.RenderMuted("Run 'myaaw cron history <job-id>' to read a job's runs in full."))
}

// showJobCronHistory prints the runs of one job in full. The id is resolved
// against jobs.json first and then against the run log, because a one-time job is
// removed after it runs while its runs stay readable. Every run is shown: a
// single job's log is small, and paging it would only hide older runs.
func showJobCronHistory(store *cron.Store, history *cron.HistoryLogger, arg string) {
	jobs, _ := store.Load()

	id := ""
	known := false
	switch matches := cron.MatchJobIDs(jobs, arg); len(matches) {
	case 1:
		id, known = matches[0], true
	case 0:
		// A removed one-time job has no entry left in jobs.json but keeps its run
		// files, so the log is the last place an id can still resolve.
		ids, err := history.MatchRunIDs(arg)
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to load history: %v", err)))
			return
		}
		switch len(ids) {
		case 0:
			fmt.Println(theme.RenderError(fmt.Sprintf("Job not found: %s", arg)))
			return
		case 1:
			id = ids[0]
		default:
			reportAmbiguousID(arg, ids)
			return
		}
	default:
		reportAmbiguousID(arg, matches)
		return
	}

	jobName := ""
	if known {
		if job, err := store.Get(id); err == nil {
			jobName = job.Name
		}
	}

	results, err := history.GetHistory(id, 0)
	if err != nil {
		fmt.Println(theme.RenderError(fmt.Sprintf("Failed to load history: %v", err)))
		return
	}

	if len(results) == 0 {
		name := jobName
		if name == "" {
			name = cron.ShortID(id)
		}
		fmt.Println(theme.RenderMuted(fmt.Sprintf("No history found for job: %s (%s)", name, id)))
		return
	}

	// The log keeps the job name even after the job is removed, so the snapshot
	// wins; the store covers entries written before runs kept one.
	displayName := results[0].Name
	if displayName == "" {
		displayName = jobName
	}
	if displayName != "" {
		fmt.Println(theme.RenderPrimary(fmt.Sprintf("History for job: %s (%s)", displayName, id)))
	} else {
		fmt.Println(theme.RenderPrimary(fmt.Sprintf("History for removed job: %s", cron.ShortID(id))))
	}

	for i, res := range results {
		if i > 0 {
			fmt.Println()
		}
		if res.Name == "" {
			res.Name = displayName
		}
		fmt.Print(cron.RunDetail(cron.JobExecutionResult{JobID: id, ExecutionResult: res}))
	}
}

// reportAmbiguousID lists the candidates when a prefix matches more than one job.
func reportAmbiguousID(arg string, matches []string) {
	fmt.Println(theme.RenderError(fmt.Sprintf("%s matches %d jobs; use a longer id:", arg, len(matches))))
	for _, match := range matches {
		fmt.Println("  " + cron.ShortID(match))
	}
}

func renderCronStatus(status string) string {
	switch status {
	case "success":
		return theme.RenderSuccess(status)
	case "failed", "failed_schedule":
		return theme.RenderError(status)
	case "skipped":
		return theme.RenderWarning(status)
	}
	return status
}

func headerCells(names ...string) []string {
	cells := make([]string, 0, len(names))
	for _, name := range names {
		cells = append(cells, theme.RenderPrimary(name))
	}
	return cells
}

func init() {
	cronCmd.AddCommand(cronListCmd)
	cronCmd.AddCommand(cronAddCmd)
	cronCmd.AddCommand(cronRemoveCmd)
	cronCmd.AddCommand(cronRunCmd)
	cronCmd.AddCommand(cronHistoryCmd)

	cronHistoryCmd.Flags().Int("limit", 10, "Number of history entries to show")

	cronAddCmd.Flags().String("name", "", "Name of the job")
	cronAddCmd.Flags().String("cron", "", "Cron expression (e.g. '0 7 * * *')")
	cronAddCmd.Flags().String("every", "", "Interval (e.g. '30m', '1h')")
	cronAddCmd.Flags().String("at", "", "Specific time (e.g. '2023-10-27T10:00:00' or '10m')")
	cronAddCmd.Flags().String("message", "", "Message/Prompt to send")
	cronAddCmd.Flags().String("channel", "", "Target channel (telegram, discord, api, cli)")
	cronAddCmd.Flags().String("to", "", "Target id: user id (dm/api/cli), chat id (telegram), channel id (discord)")
	cronAddCmd.Flags().String("thread-id", "", "Telegram forum message_thread_id")
	cronAddCmd.Flags().String("session-id", "", "Pin the exact conversation id (required for api/cli)")
	cronAddCmd.Flags().String("chat-type", "", "dm or guild (discord, to tell a DM from a channel)")
	cronAddCmd.Flags().String("tz", "", "Timezone (e.g. Asia/Jakarta)")
	cronAddCmd.Flags().String("agent", "", "Agent ID (defaults to the active agent)")

	// Check if main.go has rootCmd and add command there
	// Since this is in package main, we can assume rootCmd is available if in same package
	rootCmd.AddCommand(cronCmd)
}

// cronAgent returns the agent a cron command operates on: the explicit --agent
// value when present, otherwise the process active agent. The store directory
// and job.AgentID both come from this value, so they can never disagree.
func cronAgent(cmd *cobra.Command) string {
	if name, _ := cmd.Flags().GetString("agent"); strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return agentdir.Active()
}

func getStore(agent string) *cron.Store {
	cronDir, err := agentdir.CronDir(agent)
	if err != nil {
		cronDir = ".myaaw-cron"
	}
	path := filepath.Join(cronDir, "jobs.json")
	return cron.NewStore(path)
}

func getHistoryLogger(agent string) *cron.HistoryLogger {
	cronDir, err := agentdir.CronDir(agent)
	if err != nil {
		cronDir = ".myaaw-cron"
	}
	return cron.NewHistoryLogger(cronDir)
}
