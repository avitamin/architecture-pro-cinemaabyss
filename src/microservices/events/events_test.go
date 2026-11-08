package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/segmentio/kafka-go"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakePublisher struct {
	err   error
	calls int
}

func (p *fakePublisher) Publish(ctx context.Context, e event) (coordinates, error) {
	p.calls++
	return coordinates{2, 0}, p.err
}
func TestHTTP(t *testing.T) {
	for kind, body := range map[string]string{"movie": `{"movie_id":0,"title":"","action":"viewed","extra":1}`, "user": `{"user_id":1,"action":"registered","timestamp":"2026-11-08T00:00:00Z"}`, "payment": `{"payment_id":1,"user_id":1,"amount":0,"status":"ok","timestamp":"2026-11-08T00:00:00Z"}`} {
		t.Run(kind, func(t *testing.T) {
			p := &fakePublisher{}
			w := httptest.NewRecorder()
			handler(p).ServeHTTP(w, httptest.NewRequest("POST", "/api/events/"+kind, strings.NewReader(body)))
			if w.Code != 201 {
				t.Fatal(w.Body.String())
			}
			var v struct {
				coordinates
				Event event
			}
			json.Unmarshal(w.Body.Bytes(), &v)
			if v.Partition != 2 || v.Offset != 0 || v.Event.Type != kind || len(v.Event.ID) != 32 {
				t.Fatal(w.Body.String())
			}
		})
	}
}
func TestInvalid(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"movie_id":null,"title":"a","action":"b"}`, `{"movie_id":"1","title":"a","action":"b"}`, `{"movie_id":1,"title":"a","action":"b"} {}`, `{`} {
		p := &fakePublisher{}
		w := httptest.NewRecorder()
		handler(p).ServeHTTP(w, httptest.NewRequest("POST", "/api/events/movie", strings.NewReader(body)))
		if w.Code != 400 || p.calls != 0 {
			t.Fatal(body, w.Code)
		}
	}
	p := &fakePublisher{err: errors.New("broker down")}
	w := httptest.NewRecorder()
	handler(p).ServeHTTP(w, httptest.NewRequest("POST", "/api/events/movie", strings.NewReader(`{"movie_id":1,"title":"a","action":"b"}`)))
	if w.Code != 500 {
		t.Fatal(w.Code)
	}
}
func TestCompletion(t *testing.T) {
	ch := make(chan publication, 1)
	complete([]kafka.Message{{Partition: 3, Offset: 42, WriterData: ch}}, nil)
	r := <-ch
	if r.pos.Offset != 42 || r.pos.Partition != 3 {
		t.Fatal(r)
	}
	ch <- r
	complete([]kafka.Message{{WriterData: ch}}, nil)
}

type orderedReader struct {
	t      *testing.T
	logged bool
	cancel context.CancelFunc
}

func (r *orderedReader) FetchMessage(context.Context) (kafka.Message, error) {
	return kafka.Message{}, nil
}
func (r *orderedReader) CommitMessages(context.Context, ...kafka.Message) error {
	if !r.logged {
		r.t.Fatal("commit before log")
	}
	r.cancel()
	return nil
}
func TestConsumerOrdering(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &orderedReader{t: t, cancel: cancel}
	consume(ctx, r, func(kafka.Message) error { r.logged = true; return nil })
	var b bytes.Buffer
	l := &resultLogger{out: &b}
	if e := l.record(kafka.Message{Value: []byte("bad")}); e != nil || !strings.Contains(b.String(), "invalid_envelope") {
		t.Fatal(e, b.String())
	}
}

type gatedPublisher struct {
	release chan struct{}
	entered chan struct{}
}

func (p gatedPublisher) Publish(ctx context.Context, e event) (coordinates, error) {
	close(p.entered)
	select {
	case <-p.release:
		return coordinates{0, 17}, nil
	case <-ctx.Done():
		return coordinates{}, ctx.Err()
	}
}
func TestWaitsForAcknowledgement(t *testing.T) {
	p := gatedPublisher{make(chan struct{}), make(chan struct{})}
	done := make(chan struct{})
	w := httptest.NewRecorder()
	go func() {
		defer close(done)
		handler(p).ServeHTTP(w, httptest.NewRequest("POST", "/api/events/movie", strings.NewReader(`{"movie_id":1,"title":"a","action":"b"}`)))
	}()
	<-p.entered
	select {
	case <-done:
		t.Fatal("responded before ack")
	default:
	}
	close(p.release)
	<-done
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
}
func TestValidationTypes(t *testing.T) {
	for _, body := range []string{`{"user_id":1,"action":"a","timestamp":"bad"}`, `{"user_id":1.2,"action":"a","timestamp":"2026-11-08T00:00:00Z"}`, `{"user_id":1,"action":null,"timestamp":"2026-11-08T00:00:00Z"}`} {
		p := &fakePublisher{}
		w := httptest.NewRecorder()
		handler(p).ServeHTTP(w, httptest.NewRequest("POST", "/api/events/user", strings.NewReader(body)))
		if w.Code != 400 || p.calls != 0 {
			t.Fatal(body)
		}
	}
}
func TestLogFailureDoesNotCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &orderedReader{t: t, cancel: cancel}
	consume(ctx, r, func(kafka.Message) error { cancel(); return errors.New("stdout failed") })
	if r.logged {
		t.Fatal("unexpected log")
	}
}
