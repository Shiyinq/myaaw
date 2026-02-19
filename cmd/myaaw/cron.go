package main

import (
	"fmt"
	"log"
	"myaaw/internal/cli/theme"
	"myaaw/internal/cron"
	"os"
	"path/filepath"
	"text/tabwriter"
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
	Short: "List all scheduled jobs",
	Run: func(cmd *cobra.Command, args []string) {
		store := getStore()
		jobs, err := store.Load()
		if err != nil {
			log.Fatalf("Failed to load jobs: %v", err)
		}

		if len(jobs) == 0 {
			fmt.Println(theme.RenderMuted("No scheduled jobs found."))
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, theme.RenderPrimary("ID\tName\tSchedule\tLast Updated\tEnabled"))
		for _, job := range jobs {
			schedule := job.Schedule.Expr
			if job.Schedule.Kind == "every" {
				schedule = "@every " + job.Schedule.Expr
			}
			enabled := theme.RenderSuccess("Yes")
			if !job.Enabled {
				enabled = theme.RenderError("No")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				job.ID[:8],
				job.Name,
				schedule,
				job.UpdatedAt.Format("2006-01-02 15:04"),
				enabled,
			)
		}
		w.Flush()
	},
}

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
		tz, _ := cmd.Flags().GetString("tz")
		agentID, _ := cmd.Flags().GetString("agent")

		if name == "" || message == "" || channel == "" || to == "" {
			fmt.Println(theme.RenderError("Missing required flags: --name, --message, --channel, --to"))
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
				Mode:    "announce",
				Channel: channel,
				To:      to,
			},
			Enabled:   true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		store := getStore()
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
		store := getStore()

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
		store := getStore()

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

		history := getHistoryLogger()
		scheduler := cron.NewScheduler(store, history)

		// Manually set baseURL for CLI
		os.Setenv("MYAAW_BASE_URL", "http://localhost:8080") // Default if not set
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

func init() {
	cronCmd.AddCommand(cronListCmd)
	cronCmd.AddCommand(cronAddCmd)
	cronCmd.AddCommand(cronRemoveCmd)
	cronCmd.AddCommand(cronRunCmd)

	cronAddCmd.Flags().String("name", "", "Name of the job")
	cronAddCmd.Flags().String("cron", "", "Cron expression (e.g. '0 7 * * *')")
	cronAddCmd.Flags().String("every", "", "Interval (e.g. '30m', '1h')")
	cronAddCmd.Flags().String("at", "", "Specific time (e.g. '2023-10-27T10:00:00' or '10m')")
	cronAddCmd.Flags().String("message", "", "Message/Prompt to send")
	cronAddCmd.Flags().String("channel", "", "Target channel (telegram, discord)")
	cronAddCmd.Flags().String("to", "", "Target chat ID or channel ID")
	cronAddCmd.Flags().String("tz", "", "Timezone (e.g. Asia/Jakarta)")
	cronAddCmd.Flags().String("agent", "main", "Agent ID")

	// Check if main.go has rootCmd and add command there
	// Since this is in package main, we can assume rootCmd is available if in same package
	rootCmd.AddCommand(cronCmd)
}

func getStore() *cron.Store {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".myaaw", "cron", "jobs.json")
	return cron.NewStore(path)
}

func getHistoryLogger() *cron.HistoryLogger {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".myaaw", "cron")
	return cron.NewHistoryLogger(path)
}
