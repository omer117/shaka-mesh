package stream

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func NewHandler(hub *Hub, allowedOrigin string) (http.Handler, error) {
	if hub == nil {
		return nil, errors.New("event hub is required")
	}
	if strings.TrimSpace(allowedOrigin) == "" {
		return nil, errors.New("CORS allowed origin is required")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Add("Vary", "Origin")
			if origin != allowedOrigin {
				http.Error(w, "origin is not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		}
		streamAlerts(w, r, hub)
	})
	return mux, nil
}

func streamAlerts(w http.ResponseWriter, r *http.Request, hub *Hub) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	alerts, unsubscribe := hub.Subscribe()
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-alerts:
			if !open {
				return
			}
			payload, err := json.Marshal(event)
			if err != nil {
				log.Printf("encode swell alert for SSE: %v", err)
				return
			}
			if _, err := fmt.Fprintf(w, "event: swell-alert\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
