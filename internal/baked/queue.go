package baked

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/evmac/go-bake/internal/daemonproto"
)

// QueueEntry is a request persisted to disk so it survives restarts.
type QueueEntry struct {
	ID         string              `json:"id"`
	Request    daemonproto.Request `json:"request"`
	Workspace  string              `json:"workspace"`
	EnqueuedAt time.Time           `json:"enqueued_at"`
}

// Queue is a persistent FIFO queue backed by a JSON file in .bake/.
type Queue struct {
	mu      sync.Mutex
	path    string
	entries []QueueEntry
	nextID  int
}

// NewQueue creates (or loads) a persistent queue at rootDir/.bake/queue.json.
func NewQueue(rootDir string) (*Queue, error) {
	p := filepath.Join(rootDir, ".bake", "queue.json")
	q := &Queue{path: p}
	data, err := os.ReadFile(p)
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &q.entries); err != nil {
			q.entries = nil
		}
	}
	for _, e := range q.entries {
		var n int
		if _, err := fmt.Sscanf(e.ID, "q%d", &n); err == nil && n >= q.nextID {
			q.nextID = n + 1
		}
	}
	return q, nil
}

// Enqueue adds a request to the queue and persists it.
func (q *Queue) Enqueue(req daemonproto.Request, workspace string) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	id := fmt.Sprintf("q%d", q.nextID)
	q.nextID++
	q.entries = append(q.entries, QueueEntry{
		ID:         id,
		Request:    req,
		Workspace:  workspace,
		EnqueuedAt: time.Now(),
	})
	return id, q.flush()
}

// Dequeue removes the entry with the given ID and persists.
func (q *Queue) Dequeue(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, e := range q.entries {
		if e.ID == id {
			q.entries = append(q.entries[:i], q.entries[i+1:]...)
			return q.flush()
		}
	}
	return nil
}

// Pending returns a copy of all pending entries.
func (q *Queue) Pending() []QueueEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]QueueEntry, len(q.entries))
	copy(out, q.entries)
	return out
}

// Len returns the number of pending entries.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}

func (q *Queue) flush() error {
	data, err := json.MarshalIndent(q.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(q.path, data, 0644)
}
