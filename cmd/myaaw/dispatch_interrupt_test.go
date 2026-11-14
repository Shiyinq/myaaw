package main

import (
	"testing"
	"time"
)

func TestKeyedDispatcherBusyCoversRunningAndQueuedWork(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{}, 2)

	d := newKeyedDispatcher(1, func(job []byte) {
		if string(job) == "slow" {
			started <- struct{}{}
			<-block
		}
	})

	if d.Busy("user:1") {
		t.Fatal("an idle dispatcher reported busy")
	}

	d.Submit("user:1", []byte("slow"))
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("the job never started")
	}
	if !d.Busy("user:1") {
		t.Fatal("a running key is not busy")
	}

	// Another key has work but no free worker: it is busy too, because its batch
	// must not flush a second answer behind the queued one.
	d.Submit("user:2", []byte("queued"))
	if !d.Busy("user:2") {
		t.Fatal("a queued key is not busy")
	}
	if d.Busy("user:3") {
		t.Fatal("an untouched key is busy")
	}

	close(block)
}

func TestKeyedDispatcherControlBypassesTheLimit(t *testing.T) {
	block := make(chan struct{})
	started := make(chan string, 2)

	d := newKeyedDispatcher(1, func(job []byte) {
		id := string(job)
		started <- id
		if id == "run" {
			<-block
		}
	})

	d.Submit("user:1", []byte("run"))
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("the first job never started")
	}

	// The only worker is busy; a stop command must still run right away.
	d.SubmitControl([]byte("stop"))
	select {
	case got := <-started:
		if got != "stop" {
			t.Fatalf("control job = %q, want stop", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the control job waited for a worker slot")
	}

	close(block)
}
