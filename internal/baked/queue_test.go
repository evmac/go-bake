package baked

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evmac/go-bake/internal/daemonproto"
)

func TestNewQueueEmpty(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	q, err := NewQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	if q.Len() != 0 {
		t.Errorf("expected 0 entries, got %d", q.Len())
	}
	if pending := q.Pending(); len(pending) != 0 {
		t.Errorf("expected empty pending, got %d", len(pending))
	}
}

func TestEnqueueDequeue(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	q, err := NewQueue(dir)
	if err != nil {
		t.Fatal(err)
	}

	id, err := q.Enqueue(daemonproto.Request{Run: "build"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
	if q.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", q.Len())
	}

	pending := q.Pending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].Request.Run != "build" {
		t.Errorf("expected Run=build, got %q", pending[0].Request.Run)
	}
	if pending[0].Workspace != dir {
		t.Errorf("expected Workspace=%q, got %q", dir, pending[0].Workspace)
	}

	if err := q.Dequeue(id); err != nil {
		t.Fatal(err)
	}
	if q.Len() != 0 {
		t.Errorf("expected 0 entries after dequeue, got %d", q.Len())
	}
}

func TestDequeueUnknownID(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	q, _ := NewQueue(dir)
	if err := q.Dequeue("nonexistent"); err != nil {
		t.Errorf("dequeue unknown: %v", err)
	}
}

func TestQueueMultipleItems(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	q, _ := NewQueue(dir)

	id1, _ := q.Enqueue(daemonproto.Request{Run: "build"}, dir)
	id2, _ := q.Enqueue(daemonproto.Request{Run: "test"}, dir)
	id3, _ := q.Enqueue(daemonproto.Request{Up: true}, dir)

	if q.Len() != 3 {
		t.Errorf("expected 3, got %d", q.Len())
	}

	q.Dequeue(id2)
	if q.Len() != 2 {
		t.Errorf("expected 2 after dequeue middle, got %d", q.Len())
	}
	pending := q.Pending()
	if pending[0].ID != id1 || pending[1].ID != id3 {
		t.Errorf("unexpected ordering: %v", pending)
	}
}

func TestQueuePersistence(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)

	q1, _ := NewQueue(dir)
	q1.Enqueue(daemonproto.Request{Run: "build"}, dir)
	q1.Enqueue(daemonproto.Request{Run: "test"}, dir)

	// Load a new queue from the same path — should restore entries
	q2, _ := NewQueue(dir)
	if q2.Len() != 2 {
		t.Errorf("expected 2 restored entries, got %d", q2.Len())
	}
	pending := q2.Pending()
	if pending[0].Request.Run != "build" || pending[1].Request.Run != "test" {
		t.Errorf("unexpected restored entries: %v", pending)
	}

	// IDs should continue from where q1 left off
	id, _ := q2.Enqueue(daemonproto.Request{Run: "lint"}, dir)
	if id == "q0" || id == "q1" {
		t.Errorf("expected ID > q1, got %q", id)
	}
}

func TestQueueCorruptFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	os.WriteFile(filepath.Join(dir, ".bake", "queue.json"), []byte("not json"), 0644)

	q, err := NewQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	if q.Len() != 0 {
		t.Errorf("corrupt file should result in empty queue, got %d", q.Len())
	}
}
