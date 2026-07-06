package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// makeEventsHandler handles GET /jobs/{jobId}/events.
// It streams Server-Sent Events to the client until the job reaches a terminal
// state or the client disconnects.
func makeEventsHandler(svc Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := chi.URLParam(r, "jobId")

		// Verify the job exists before opening the stream.
		_, ok := svc.Queue.Get(jobID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}

		// Negotiate SSE.
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
		flusher.Flush()

		// Send the current snapshot immediately so the client gets state even
		// if it subscribes after the job has progressed.
		snap, hasSnap := svc.Queue.Snapshot(jobID)
		if hasSnap {
			if err := sendSSEEvent(w, "progress", snap); err == nil {
				flusher.Flush()
			}
		}

		// If the job is already terminal, close immediately.
		if hasSnap && snap.Status.IsTerminal() {
			_ = sendSSEEvent(w, "done", snap)
			flusher.Flush()
			return
		}

		// Subscribe to live updates.
		ch, ok := svc.Queue.Subscribe(jobID)
		if !ok {
			// Job became terminal between the check above and here.
			snap, _ := svc.Queue.Snapshot(jobID)
			_ = sendSSEEvent(w, "done", snap)
			flusher.Flush()
			return
		}

		// Keepalive ticker to prevent proxy timeouts.
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				// Client disconnected.
				return

			case <-ticker.C:
				if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
					return
				}
				flusher.Flush()

			case event, open := <-ch:
				if !open {
					// Broadcaster closed (job terminal). Send final state.
					if finalSnap, ok := svc.Queue.Snapshot(jobID); ok {
						_ = sendSSEEvent(w, "done", finalSnap)
						flusher.Flush()
					}
					return
				}
				eventName := "progress"
				if event.Status.IsTerminal() {
					eventName = "done"
				}
				if err := sendSSEEvent(w, eventName, event); err != nil {
					return
				}
				flusher.Flush()
				if event.Status.IsTerminal() {
					return
				}
			}
		}
	}
}

// sendSSEEvent writes a single SSE event with the given name and JSON data.
func sendSSEEvent(w http.ResponseWriter, name string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, string(b))
	return err
}
