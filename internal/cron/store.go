package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	path string
	mu   sync.RWMutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() ([]Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create dir if not exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		return []Job{}, nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}

	var jobs []Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, err
	}

	return jobs, nil
}

func (s *Store) Save(jobs []Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0644)
}

func (s *Store) Add(job Job) error {
	jobs, err := s.Load()
	if err != nil {
		return err
	}

	jobs = append(jobs, job)
	return s.Save(jobs)
}

func (s *Store) Remove(id string) error {
	jobs, err := s.Load()
	if err != nil {
		return err
	}

	var newJobs []Job
	found := false
	for _, j := range jobs {
		if j.ID != id {
			newJobs = append(newJobs, j)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("job not found: %s", id)
	}

	return s.Save(newJobs)
}

func (s *Store) Update(job Job) error {
	jobs, err := s.Load()
	if err != nil {
		return err
	}

	found := false
	for i, j := range jobs {
		if j.ID == job.ID {
			jobs[i] = job
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("job not found: %s", job.ID)
	}

	return s.Save(jobs)
}

func (s *Store) Get(id string) (*Job, error) {
	jobs, err := s.Load()
	if err != nil {
		return nil, err
	}

	for _, j := range jobs {
		if j.ID == id {
			return &j, nil
		}
	}

	return nil, fmt.Errorf("job not found: %s", id)
}
