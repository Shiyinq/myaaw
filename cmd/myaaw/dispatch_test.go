package main

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitForCondition(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestKeyedDispatcherKeepsPerKeyOrder(t *testing.T) {
	release := make(chan struct{})
	firstStarted := make(chan struct{})
	otherDone := make(chan struct{})

	var (
		mu    sync.Mutex
		order []string
	)
	d := newKeyedDispatcher(4, func(job []byte) {
		id := string(job)
		mu.Lock()
		order = append(order, id)
		mu.Unlock()

		switch id {
		case "a1":
			close(firstStarted)
			<-release
		case "b1":
			close(otherDone)
		}
	})

	d.Submit("a", []byte("a1"))
	d.Submit("a", []byte("a2"))
	d.Submit("a", []byte("a3"))
	d.Submit("b", []byte("b1"))

	<-firstStarted

	// Another key must not wait behind the blocked conversation.
	select {
	case <-otherDone:
	case <-time.After(2 * time.Second):
		t.Fatal("a different key was blocked by a running conversation")
	}

	close(release)

	waitForCondition(t, "all jobs", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order) == 4
	})

	var aOrder []string
	for _, id := range order {
		if id == "a1" || id == "a2" || id == "a3" {
			aOrder = append(aOrder, id)
		}
	}
	want := []string{"a1", "a2", "a3"}
	for i := range want {
		if aOrder[i] != want[i] {
			t.Fatalf("per-key order = %v, want %v", aOrder, want)
		}
	}
}

func TestKeyedDispatcherRespectsLimit(t *testing.T) {
	var (
		active  atomic.Int32
		peak    atomic.Int32
		running sync.WaitGroup
	)
	run := func([]byte) {
		current := active.Add(1)
		for {
			observed := peak.Load()
			if current <= observed || peak.CompareAndSwap(observed, current) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		active.Add(-1)
		running.Done()
	}

	d := newKeyedDispatcher(2, run)
	running.Add(6)
	for i := 0; i < 6; i++ {
		d.Submit("user-"+strconv.Itoa(i), []byte("job"))
	}
	running.Wait()

	if got := peak.Load(); got > 2 {
		t.Fatalf("peak concurrency = %d, want <= 2", got)
	}
}

func TestKeyedDispatcherContinuesQueuedKeyWhenSlotFrees(t *testing.T) {
	var (
		mu      sync.Mutex
		started []string
	)
	d := newKeyedDispatcher(1, func(job []byte) {
		mu.Lock()
		started = append(started, string(job))
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	})

	d.Submit("user", []byte("first"))
	d.Submit("other", []byte("second"))
	d.Submit("third", []byte("third"))

	waitForCondition(t, "queued jobs to run", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(started) == 3
	})
}

func TestMessageQueueKeyGroupsByUser(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "telegram",
			body: `{"channel":"telegram","payload":{"update_id":1,"message":{"from":{"id":123},"chat":{"id":123}}}}`,
			want: "user:123",
		},
		{
			name: "discord",
			body: `{"channel":"discord","payload":{"message":{"from":{"id":456},"chat":{"id":"chan"}},"discord_author":{"id":"456"}}}`,
			want: "user:456",
		},
		{
			name: "discord author fallback",
			body: `{"channel":"discord","payload":{"discord_author":{"id":"789"}}}`,
			want: "user:789",
		},
		{
			name: "unknown sender",
			body: `{"channel":"telegram","payload":{}}`,
			want: "channel:telegram",
		},
		{
			name: "invalid json",
			body: `not-json`,
			want: "unparsed",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := messageQueueKey([]byte(c.body)); got != c.want {
				t.Fatalf("messageQueueKey = %q, want %q", got, c.want)
			}
		})
	}
}
