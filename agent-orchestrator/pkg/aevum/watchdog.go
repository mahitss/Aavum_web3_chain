package aevum

import (
	"context"
	"sync"
	"time"
)

type RefundHandler func(taskID string, refundTx string, err error)

type watched struct {
	taskID   [32]byte
	hexID    string
	deadline uint64
	done     RefundHandler
}

type Watchdog struct {
	client     *Client
	mu         sync.Mutex
	tasks      []watched
	OnProgress func(hexID string, head uint64, deadline uint64)
}

func NewWatchdog(client *Client) *Watchdog {
	return &Watchdog{client: client}
}

func (w *Watchdog) Track(taskID [32]byte, hexID string, deadline uint64, done RefundHandler) {
	if deadline == 0 || hexID == "" {
		return
	}
	w.mu.Lock()
	for _, existing := range w.tasks {
		if existing.hexID == hexID {
			w.mu.Unlock()
			return
		}
	}
	w.tasks = append(w.tasks, watched{taskID: taskID, hexID: hexID, deadline: deadline, done: done})
	w.mu.Unlock()
}

func (w *Watchdog) Forget(hexID string) {
	if hexID == "" {
		return
	}
	w.mu.Lock()
	kept := make([]watched, 0, len(w.tasks))
	for _, task := range w.tasks {
		if task.hexID != hexID {
			kept = append(kept, task)
		}
	}
	w.tasks = kept
	w.mu.Unlock()
}

func (w *Watchdog) Run(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.sweep(ctx)
		}
	}
}

func (w *Watchdog) sweep(ctx context.Context) {
	head, err := w.client.BlockNumber(ctx)
	if err != nil {
		return
	}
	w.mu.Lock()
	pending := append([]watched(nil), w.tasks...)
	w.mu.Unlock()
	finished := map[string]struct{}{}
	for _, task := range pending {
		if head <= task.deadline {
			if w.OnProgress != nil {
				w.OnProgress(task.hexID, head, task.deadline)
			}
			continue
		}
		refundTx, refundErr := w.client.RefundExpired(ctx, task.taskID)
		if task.done != nil {
			task.done(task.hexID, refundTx, refundErr)
		}
		if refundErr == nil {
			finished[task.hexID] = struct{}{}
		}
	}
	if len(finished) == 0 {
		return
	}
	w.mu.Lock()
	kept := make([]watched, 0, len(w.tasks))
	for _, task := range w.tasks {
		if _, ok := finished[task.hexID]; !ok {
			kept = append(kept, task)
		}
	}
	w.tasks = kept
	w.mu.Unlock()
}
