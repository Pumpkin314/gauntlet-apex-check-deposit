package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/lib/pq"
)

// Broadcaster listens to pg_notify on a channel and fans out messages to SSE clients.
type Broadcaster struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
	log     *slog.Logger
}

// NewBroadcaster creates a Broadcaster and starts listening on the given pg_notify channel.
func NewBroadcaster(dbURL string, channel string, log *slog.Logger) (*Broadcaster, error) {
	b := &Broadcaster{
		clients: make(map[chan []byte]struct{}),
		log:     log,
	}

	listener := pq.NewListener(dbURL, 10*time.Second, time.Minute, func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Error("pg listener event", "error", err)
		}
	})

	if err := listener.Listen(channel); err != nil {
		listener.Close()
		return nil, fmt.Errorf("listen on %s: %w", channel, err)
	}

	go b.listen(listener)
	log.Info("SSE broadcaster started", "channel", channel)
	return b, nil
}

// NewBroadcasterFromDB creates a Broadcaster using an existing *sql.DB connection string.
func NewBroadcasterFromDB(db *sql.DB, dbURL string, channel string, log *slog.Logger) (*Broadcaster, error) {
	return NewBroadcaster(dbURL, channel, log)
}

func (b *Broadcaster) listen(l *pq.Listener) {
	for {
		select {
		case n := <-l.Notify:
			if n == nil {
				continue
			}
			b.broadcast([]byte(n.Extra))
		case <-time.After(90 * time.Second):
			if err := l.Ping(); err != nil {
				b.log.Error("pg listener ping failed", "error", err)
			}
		}
	}
}

func (b *Broadcaster) broadcast(data []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- data:
		default:
			// client too slow, drop message
		}
	}
}

// Subscribe registers a new client and returns its event channel.
func (b *Broadcaster) Subscribe() chan []byte {
	ch := make(chan []byte, 64)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	b.log.Info("SSE client subscribed", "total_clients", b.ClientCount())
	return ch
}

// Unsubscribe removes a client channel.
func (b *Broadcaster) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
	b.log.Info("SSE client unsubscribed", "total_clients", b.ClientCount())
}

// ClientCount returns the number of connected SSE clients.
func (b *Broadcaster) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}

// ServeHTTP is the SSE endpoint handler.
func (b *Broadcaster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	// Write an initial SSE comment to force Cloud Run's proxy to flush headers.
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	ctx := r.Context()
	// Send keepalive comment every 30s
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case data := <-ch:
			// Try to parse as JSON to extract event type
			var payload map[string]interface{}
			eventType := "transfer_update"
			if err := json.Unmarshal(data, &payload); err == nil {
				if et, ok := payload["event_type"].(string); ok {
					eventType = et
				}
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(data))
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive %d\n\n", time.Now().Unix())
			flusher.Flush()
		}
	}
}

// InjectNotifyPayload is used by tests to manually broadcast data without pg_notify.
func (b *Broadcaster) InjectNotifyPayload(ctx context.Context, payload map[string]interface{}) {
	data, _ := json.Marshal(payload)
	b.broadcast(data)
}
