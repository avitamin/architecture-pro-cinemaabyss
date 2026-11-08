package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type coordinates struct {
	Partition int   `json:"partition"`
	Offset    int64 `json:"offset"`
}
type publisher interface {
	Publish(context.Context, event) (coordinates, error)
}

func respond(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s)
	json.NewEncoder(w).Encode(v)
}
func handler(p publisher) http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/events/health", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]bool{"status": true}) })
	for kind := range schemas {
		m.HandleFunc("POST /api/events/"+kind, func(w http.ResponseWriter, r *http.Request) {
			var payload map[string]json.RawMessage
			d := json.NewDecoder(r.Body)
			e := d.Decode(&payload)
			if e == nil {
				var extra any
				if d.Decode(&extra) != io.EOF {
					e = io.ErrUnexpectedEOF
				}
			}
			if e == nil {
				e = validate(kind, payload)
			}
			if e != nil {
				respond(w, 400, map[string]string{"error": e.Error()})
				return
			}
			ev, e := newEvent(kind, payload)
			if e != nil {
				respond(w, 500, map[string]string{"error": "cannot create event"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			pos, e := p.Publish(ctx, ev)
			if e != nil {
				respond(w, 500, map[string]string{"error": "Kafka publication failed"})
				return
			}
			respond(w, 201, struct {
				Status string `json:"status"`
				coordinates
				Event event `json:"event"`
			}{"success", pos, ev})
		})
	}
	return m
}
