package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
)

// streamKeepAlive is under the 60s idle timeout common to reverse proxies.
const streamKeepAlive = 25 * time.Second

type streamEvent struct {
	Cursor   int64  `json:"cursor"`
	Type     string `json:"type"`
	TicketID int64  `json:"ticket_id"`
}

// eventStream sends a server-sent event for every board change. It is a
// doorbell: clients refetch what they show.
func (s *server) eventStream(w http.ResponseWriter, r *http.Request, _ caller) error {
	controller := http.NewResponseController(w)
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-store")
	// Tells nginx not to buffer the stream.
	header.Set("X-Accel-Buffering", "no")
	subscription := s.deps.Broker.Subscribe()
	defer subscription.Close()
	if _, err := fmt.Fprint(w, "retry: 3000\n\n"); err != nil {
		return nil
	}
	if err := controller.Flush(); err != nil {
		return err
	}
	return s.pump(w, r, controller, subscription.Events())
}

func (s *server) pump(w http.ResponseWriter, r *http.Request, controller *http.ResponseController, events <-chan app.Event) error {
	keepAlive := time.NewTicker(streamKeepAlive)
	defer keepAlive.Stop()
	for {
		var err error
		select {
		case <-r.Context().Done():
			return nil
		case event := <-events:
			err = writeEvent(w, event)
		case <-keepAlive.C:
			_, err = fmt.Fprint(w, ": keep-alive\n\n")
		}
		// A write error means the client went away; that ends the stream.
		if err != nil || controller.Flush() != nil {
			return nil
		}
	}
}

func writeEvent(w http.ResponseWriter, event app.Event) error {
	data, err := json.Marshal(streamEvent{Cursor: event.Cursor, Type: string(event.Type), TicketID: event.TicketID})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: ticket\ndata: %s\n\n", data)
	return err
}
