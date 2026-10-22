package a2a

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T) *TaskStore {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "a2a.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store := NewTaskStore(db)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return store
}

func TestTaskStoreRecoverStale(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for _, task := range []Task{
		{ID: "working", Status: TaskStatus{State: StateWorking}},
		{ID: "submitted", Status: TaskStatus{State: StateSubmitted}},
		{ID: "done", Status: TaskStatus{State: StateCompleted}},
	} {
		if err := store.Create(ctx, task); err != nil {
			t.Fatalf("Create %s: %v", task.ID, err)
		}
	}

	recovered, err := store.RecoverStale(ctx)
	if err != nil {
		t.Fatalf("RecoverStale: %v", err)
	}
	if recovered != 2 {
		t.Fatalf("recovered = %d, want 2", recovered)
	}

	for _, id := range []string{"working", "submitted"} {
		task, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if task.Status.State != StateFailed || task.Status.Message == nil {
			t.Fatalf("%s = %+v", id, task)
		}
	}
	done, err := store.Get(ctx, "done")
	if err != nil {
		t.Fatalf("Get done: %v", err)
	}
	if done.Status.State != StateCompleted {
		t.Fatalf("done = %+v", done)
	}
}

func TestTaskStoreCreateGetUpdate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	task := Task{
		ID:        "task-1",
		ContextID: "conv-1",
		Status:    TaskStatus{State: StateWorking},
		Messages:  []Message{{Role: "user", Parts: []Part{{Kind: "text", Text: "hi"}}}},
	}
	if err := store.Create(ctx, task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.Get(ctx, "task-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ContextID != "conv-1" || got.Status.State != StateWorking || len(got.Messages) != 1 {
		t.Fatalf("got = %+v", got)
	}

	task.Status = TaskStatus{State: StateCompleted}
	task.Artifacts = []Artifact{{Name: "response", Parts: []Part{{Kind: "text", Text: "done"}}}}
	if err := store.Update(ctx, task); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = store.Get(ctx, "task-1")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Status.State != StateCompleted || len(got.Artifacts) != 1 || got.Artifacts[0].Parts[0].Text != "done" {
		t.Fatalf("got = %+v", got)
	}

	if _, err := store.Get(ctx, "missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}
	if err := store.Create(ctx, task); err == nil {
		t.Fatal("duplicate task should fail")
	}
}

func TestEnsureCaller(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	alice, err := store.EnsureCaller(ctx, "alice")
	if err != nil {
		t.Fatalf("EnsureCaller alice: %v", err)
	}
	if alice >= -1000 {
		t.Fatalf("alice id = %d, want a synthetic id below -1000", alice)
	}
	bob, err := store.EnsureCaller(ctx, "bob")
	if err != nil {
		t.Fatalf("EnsureCaller bob: %v", err)
	}
	if bob == alice {
		t.Fatalf("bob and alice share id %d", alice)
	}
	again, err := store.EnsureCaller(ctx, "alice")
	if err != nil {
		t.Fatalf("EnsureCaller alice again: %v", err)
	}
	if again != alice {
		t.Fatalf("alice id changed: %d -> %d", alice, again)
	}
}
