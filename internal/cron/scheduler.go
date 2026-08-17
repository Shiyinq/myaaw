package cron

import (
	"fmt"
	"log"
	"myaaw/internal/config"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-resty/resty/v2"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	store   *Store
	cron    *cron.Cron
	history *HistoryLogger
	baseURL string
	timers  map[string]*time.Timer
	mu      sync.Mutex
}

func NewScheduler(store *Store, history *HistoryLogger) *Scheduler {
	// Use a parser that supports standard cron (5 fields) AND seconds (6 fields)
	parser := cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)

	return &Scheduler{
		store:   store,
		cron:    cron.New(cron.WithParser(parser)),
		history: history,
		timers:  make(map[string]*time.Timer),
	}
}

// NewDefaultScheduler creates a scheduler with default paths (~/.myaaw/cron)
func NewDefaultScheduler() (*Scheduler, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home dir: %w", err)
	}

	cronDir := filepath.Join(home, ".myaaw", "cron")
	if err := os.MkdirAll(cronDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cron directory: %w", err)
	}

	store := NewStore(filepath.Join(cronDir, "jobs.json"))
	history := NewHistoryLogger(cronDir)

	return NewScheduler(store, history), nil
}

func (s *Scheduler) Start() error {
	s.baseURL = config.MYAAWBaseURL
	if s.baseURL == "" {
		s.baseURL = "http://localhost" + config.PORT
	}

	if !config.CronActive {
		log.Println("Cron scheduler is disabled in config")
		return nil
	}

	if err := s.Reload(); err != nil {
		return err
	}

	s.cron.Start()
	log.Println("Cron scheduler started")
	return nil
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
	log.Println("Cron scheduler stopped")
}

func (s *Scheduler) ReloadConfig() {
	if config.CronActive {
		log.Println("Reloading Cron Config: Active=true")

		s.Stop()
		if err := s.Start(); err != nil {
			log.Printf("Failed to restart scheduler: %v", err)
		}
	} else {
		log.Println("Reloading Cron Config: Active=false")
		s.Stop()
	}
}

func (s *Scheduler) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Remove all existing cron jobs
	entries := s.cron.Entries()
	for _, entry := range entries {
		s.cron.Remove(entry.ID)
	}

	// 2. Stop and clear all existing "at" timers
	for id, timer := range s.timers {
		if timer != nil {
			timer.Stop()
		}
		delete(s.timers, id)
	}

	// 3. Check if scheduler is globally disabled
	if !config.CronActive {
		// We already cleared everything, so just return
		return nil
	}

	jobs, err := s.store.Load()
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if !job.Enabled {
			continue
		}

		// Closure to capture job
		j := job

		// Determine schedule specification
		spec := j.Schedule.Expr
		if j.Schedule.Kind == "every" {
			spec = "@every " + j.Schedule.Expr
		} else if j.Schedule.Kind == "at" {
			// Handle "at" jobs (one-time)
			targetTime, err := parseAtTime(j.Schedule.Expr, j.Schedule.Tz)
			if err != nil {
				log.Printf("Failed to parse time for job %s: %v", j.Name, err)
				s.history.Log(j.ID, "failed_schedule", fmt.Sprintf("Failed to parse time: %v", err))
				continue
			}

			delay := time.Until(targetTime)
			if delay <= 0 {
				log.Printf("Job %s is in the past detailed from %s. Skipping execution and removing...", j.Name, targetTime.Format(time.RFC3339))

				// Log to history as skipped
				if err := s.history.Log(j.ID, "skipped", fmt.Sprintf("Job expired (scheduled at %s)", targetTime.Format(time.RFC3339))); err != nil {
					log.Printf("Failed to log history for skipped job %s: %v", j.Name, err)
				}

				// Remove immediately without executing
				if err := s.store.Remove(j.ID); err != nil {
					log.Printf("Failed to remove expired job %s: %v", j.Name, err)
				}
				continue
			}

			log.Printf("Scheduled one-time job: %s at %s (in %v)", j.Name, targetTime.Format(time.RFC3339), delay)

			// Schedule timer and store reference
			timer := time.AfterFunc(delay, func() {
				s.executeJob(j)
				// Remove after execution
				if err := s.store.Remove(j.ID); err != nil {
					log.Printf("Failed to auto-remove job %s: %v", j.Name, err)
				} else {
					log.Printf("Auto-removed one-time job: %s", j.Name)
				}

				// Clean up timer map entry
				s.mu.Lock()
				delete(s.timers, j.ID)
				s.mu.Unlock()
			})
			s.timers[j.ID] = timer
			continue
		}

		// If timezone is specified and it's a cron expression, we might need to handle it.
		// For simplicity, we assume server time or that expr handles it if necessary.
		// configuring specific timezone with robfig/cron requires CRON_TZ=... in expr or WithLocation.
		// For now, let's keep it simple. If TZ is provided, we can prepend CRON_TZ
		if j.Schedule.Kind == "cron" && j.Schedule.Tz != "" {
			spec = "CRON_TZ=" + j.Schedule.Tz + " " + spec
		}

		_, err := s.cron.AddFunc(spec, func() {
			s.executeJob(j)
		})

		if err != nil {
			log.Printf("Failed to schedule job %s: %v", j.Name, err)
			s.history.Log(j.ID, "failed_schedule", fmt.Sprintf("Failed to schedule: %v", err))
		} else {
			log.Printf("Scheduled job: %s (%s)", j.Name, spec)
		}
	}

	return nil
}

func (s *Scheduler) executeJob(job Job) {
	log.Printf("Executing job: %s", job.Name)

	client := resty.New()
	client.SetTimeout(120 * time.Second)

	// Construct HeartbeatRequest
	payload := map[string]interface{}{
		"prompt":  job.Payload.Content,
		"to":      job.Delivery.To,
		"channel": job.Delivery.Channel,
		"trigger": "cron",
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(fmt.Sprintf("%s/heartbeat", s.baseURL))

	status := "success"
	result := "Job executed successfully"

	if err != nil {
		status = "failed"
		result = fmt.Sprintf("Request failed: %v", err)
		log.Printf("Job %s failed: %v", job.Name, err)
	} else if resp.IsError() {
		status = "failed"
		result = fmt.Sprintf("Server returned error: %s %s", resp.Status(), resp.String())
		log.Printf("Job %s failed: %s", job.Name, resp.Status())
	} else {
		log.Printf("Job %s executed successfully", job.Name)
	}

	if err := s.history.Log(job.ID, status, result); err != nil {
		log.Printf("Failed to log history for job %s: %v", job.Name, err)
	}
}

// Watch starts a file watcher to reload jobs on change
func (s *Scheduler) Watch() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Failed to create file watcher: %v", err)
		return
	}
	defer watcher.Close()

	// Watch the directory, not just the file, to handle atomic writes (move/rename)
	configDir := filepath.Dir(s.store.path)
	if err := watcher.Add(configDir); err != nil {
		log.Printf("Failed to watch config directory: %v", err)
		return
	}

	log.Println("Uninterrupted file watcher started for cron jobs")

	// Debounce timer
	var debounceTimer *time.Timer
	debounceDuration := 500 * time.Millisecond

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Check if the event is related to our jobs.json file
			if filepath.Base(event.Name) == filepath.Base(s.store.path) {
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Chmod) {
					// Stop existing timer if any
					if debounceTimer != nil {
						debounceTimer.Stop()
					}
					// Reset timer
					debounceTimer = time.AfterFunc(debounceDuration, func() {
						log.Println("Jobs configuration changed, reloading...")
						if err := s.Reload(); err != nil {
							log.Printf("Failed to reload jobs: %v", err)
						}
					})
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

// parseAtTime parses absolute or relative time strings
func parseAtTime(expr string, tz string) (time.Time, error) {
	// 1. Try relative duration (e.g. "10m", "1h")
	if duration, err := time.ParseDuration(expr); err == nil {
		return time.Now().Add(duration), nil
	}

	// 2. Try absolute layouts
	layouts := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}

	var loc *time.Location = time.Local
	if tz != "" {
		var err error
		loc, err = time.LoadLocation(tz)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid timezone: %v", err)
		}
	}

	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, expr, loc); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid time format: %s", expr)
}

// RunJob manually executes a job by ID
func (s *Scheduler) RunJob(id string) error {
	job, err := s.store.Get(id)
	if err != nil {
		return err
	}

	go s.executeJob(*job)
	return nil
}
