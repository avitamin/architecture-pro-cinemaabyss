package main

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

func chooseMovies(gradual bool, percent int, roll func(int) int) bool {
	return !gradual || roll(100) < percent
}
func segment(path, base string) bool { return path == base || strings.HasPrefix(path, base+"/") }

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(s int) { w.status = s; w.ResponseWriter.WriteHeader(s) }
func newProxy(c config) http.Handler {
	proxies := map[string]*httputil.ReverseProxy{}
	for name, u := range c.urls {
		proxies[name] = &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) { r.SetURL(u); r.SetXForwarded() }, Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext, ResponseHeaderTimeout: 5 * time.Second}, ErrorHandler: func(w http.ResponseWriter, r *http.Request, e error) {
			s := 502
			var ne net.Error
			if errors.Is(e, context.DeadlineExceeded) || (errors.As(e, &ne) && ne.Timeout()) {
				s = 504
			}
			http.Error(w, http.StatusText(s), s)
		}}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" && r.Method == "GET" {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("Strangler Fig Proxy is healthy"))
			return
		}
		name := "monolith"
		if r.URL.Path == "/api/movies/health" {
			name = "movies"
		} else if segment(r.URL.Path, "/api/movies") && chooseMovies(c.gradual, c.percent, rand.IntN) {
			name = "movies"
		} else if segment(r.URL.Path, "/api/events") {
			name = "events"
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		sw := &statusWriter{w, 200}
		start := time.Now()
		proxies[name].ServeHTTP(sw, r.WithContext(ctx))
		slog.Info("proxy_request", "method", r.Method, "path", r.URL.Path, "upstream", name, "status", sw.status, "duration_ms", time.Since(start).Milliseconds())
	})
}
