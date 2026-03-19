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

// BroadcasterConfig holds tunable parameters for the SSE broadcaster.
type BroadcasterConfig struct {
	PGListenerMinReconnect time.Duration // pq.Listener minimum reconnect delay
	PGListenerMaxReconnect time.Duration // pq.Listener maximum reconnect delay
	PGListenerPingInterval time.Duration // interval between pg listener health pings
	ChannelBufferSize      int           // per-client event channel buffer
	KeepaliveInterval      time.Duration // SSE keepalive comment interval
}

// DefaultBroadcasterConfig returns production defaults.
func DefaultBroadcasterConfig() BroadcasterConfig {
	return BroadcasterConfig{
		PGListenerMinReconnect: 10 * time.Second,
		PGListenerMaxReconnect: time.Minute,
		PGListenerPingInterval: 90 * time.Second,
		ChannelBufferSize:      64,
		KeepaliveInterval:      30 * time.Second,
	}
}

// Broadcaster listens to pg_notify on a channel and fans out messages to SSE clients.
type Broadcaster struct {
	mu     sync.RWMutex
	clients map[chan []byte]struct{}
	log     *slog.Logger
	cfg     BroadcasterConfig
}

// NewBroadcaster creates a Broadcaster and starts listening on the given pg_notify channel.
func NewBroadcaster(dbURL string, channel string, log *slog.Logger) (*Broadcaster, error) {
	return NewBroadcasterWithConfig(dbURL, channel, log, DefaultBroadcasterConfig())
}

// NewBroadcasterWithConfig creates a Broadcaster with explicit tuning parameters.
func NewBroadcasterWithConfig(dbURL string, channel string, log *slog.Logger, cfg BroadcasterConfig) (*Broadcaster, error) {
	b := &Broadcaster{
		clients: make(map[chan []byte]struct{}),
		log:     log,
		cfg:     cfg,
	}

	listener := pq.NewListener(dbURL, cfg.PGListenerMinReconnect, cfg.PGListenerMaxReconnect, func(ev pq.ListenerEventType, err error) {
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
		case <-time.After(b.cfg.PGListenerPingInterval):
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
	ch := make(chan []byte, b.cfg.ChannelBufferSize)
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
	ticker := time.NewTicker(b.cfg.KeepaliveInterval)
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
