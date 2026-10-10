package app

import (
	"log/slog"
	"portbridge/internal/store"
	"time"
)

func (h *Hub) monitor() {
	defer close(h.monitorDone)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-h.stopMonitor:
			return
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.mu.Lock()
			seen := map[string]time.Time{}
			for id, l := range h.live {
				seen[id] = l.seen
			}
			h.mu.Unlock()
			if err := h.Store.Update(func(s *store.State) error {
				for id, n := range s.Nodes {
					if seen[id].After(n.LastSeen) {
						n.LastSeen = seen[id]
						s.Nodes[id] = n
					}
				}
				return nil
			}); err != nil {
				slog.Error("persist node status", "error", err)
			}
		}
	}
}
