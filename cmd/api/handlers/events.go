package handlers

import (
	"net/http"

	"github.com/apex-checkout/check-deposit/internal/events"
)

// EventsHandler wraps the SSE broadcaster for the events stream endpoint.
type EventsHandler struct {
	Broadcaster *events.Broadcaster
}

// Stream handles GET /events/stream.
func (h *EventsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	h.Broadcaster.ServeHTTP(w, r)
}
