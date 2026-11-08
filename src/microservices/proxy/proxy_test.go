package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSelection(t *testing.T) {
	for _, c := range []struct {
		g bool
		p int
	}{{true, 0}, {true, 50}, {true, 100}, {false, 0}} {
		for n := 0; n < 100; n++ {
			got := chooseMovies(c.g, c.p, func(int) int { return n })
			if got != (!c.g || n < c.p) {
				t.Fatal(c, n)
			}
		}
	}
}
func TestRoutesAndPreservation(t *testing.T) {
	for _, path := range []string{"/api/movies?id=7", "/api/movies/health", "/api/movies-other", "/api/events/user", "/api/users"} {
		t.Run(path, func(t *testing.T) {
			servers := map[string]*url.URL{}
			for _, name := range []string{"movies", "monolith", "events"} {
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					b, _ := io.ReadAll(r.Body)
					if r.Method != "POST" || string(b) != "body" || r.Header.Get("X-Test") != "yes" || r.URL.RequestURI() != path {
						t.Error("request changed")
					}
					w.Header().Set("X-Upstream", name)
					w.WriteHeader(201)
					w.Write([]byte("reply"))
				}))
				defer s.Close()
				servers[name], _ = url.Parse(s.URL)
			}
			r := httptest.NewRequest("POST", path, strings.NewReader("body"))
			r.Header.Set("X-Test", "yes")
			w := httptest.NewRecorder()
			newProxy(config{urls: servers, percent: 100, gradual: true}).ServeHTTP(w, r)
			want := "monolith"
			if segment(r.URL.Path, "/api/movies") {
				want = "movies"
			}
			if segment(r.URL.Path, "/api/events") {
				want = "events"
			}
			if w.Code != 201 || w.Body.String() != "reply" || w.Header().Get("X-Upstream") != want {
				t.Fatal(w)
			}
		})
	}
}
func TestBadConfig(t *testing.T) {
	for k, v := range map[string]string{"MONOLITH_URL": "http://localhost", "MOVIES_SERVICE_URL": "http://localhost", "EVENTS_SERVICE_URL": "http://localhost", "GRADUAL_MIGRATION": "true", "MOVIES_MIGRATION_PERCENT": "50"} {
		t.Setenv(k, v)
	}
	if _, e := loadConfig(); e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"MONOLITH_URL", "MOVIES_SERVICE_URL", "EVENTS_SERVICE_URL", "GRADUAL_MIGRATION", "MOVIES_MIGRATION_PERCENT"} {
		old := os.Getenv(key)
		t.Setenv(key, "invalid")
		if _, e := loadConfig(); e == nil {
			t.Fatal(key)
		}
		t.Setenv(key, old)
	}
}

func TestFailuresNoFallback(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		calls := 0
		fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
		defer fallback.Close()
		release := make(chan struct{})
		bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
		u, _ := url.Parse(bad.URL)
		if !timeout {
			bad.Close()
		}
		f, _ := url.Parse(fallback.URL)
		h := newProxy(config{urls: map[string]*url.URL{"movies": u, "monolith": f, "events": f}, percent: 100, gradual: true})
		r := httptest.NewRequest("POST", "/api/movies", strings.NewReader("write"))
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Millisecond)
		defer cancel()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r.WithContext(ctx))
		close(release)
		bad.Close()
		want := 502
		if timeout {
			want = 504
		}
		if w.Code != want || calls != 0 {
			t.Fatal(w.Code, calls)
		}
	}
}

func TestMoviesHealthBypassesZeroPercent(t *testing.T) {
	urls := make(map[string]*url.URL)
	for _, name := range []string{"monolith", "movies", "events"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Upstream", name)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		urls[name], _ = url.Parse(server.URL)
	}
	h := newProxy(config{urls: urls, gradual: true, percent: 0})
	for _, tc := range []struct{ path, upstream string }{
		{"/api/movies", "monolith"},
		{"/api/movies/health", "movies"},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != http.StatusOK || w.Header().Get("X-Upstream") != tc.upstream {
			t.Fatalf("%s: status=%d upstream=%q, want %q", tc.path, w.Code, w.Header().Get("X-Upstream"), tc.upstream)
		}
	}
}
