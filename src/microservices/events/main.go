package main

import (
	"context"
	"github.com/segmentio/kafka-go"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	c, e := loadConfig()
	if e != nil {
		slog.Error("configuration", "error", e)
		os.Exit(1)
	}
	p := newPublisher(c.brokers)
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: c.brokers, GroupID: "cinemaabyss-events", GroupTopics: []string{"movie-events", "user-events", "payment-events"}, StartOffset: kafka.FirstOffset, CommitInterval: 0, MaxWait: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	l := &resultLogger{out: os.Stdout}
	go func() { defer close(done); consume(ctx, r, l.record) }()
	s := &http.Server{Addr: ":" + c.port, Handler: handler(p), ReadHeaderTimeout: 5 * time.Second}
	sig, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	httpDone := make(chan struct{})
	var serverErr error
	go func() {
		defer close(httpDone)
		if e := s.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			serverErr = e
			slog.Error("http", "error", e)
			stop()
		}
	}()
	<-sig.Done()
	end, finish := context.WithTimeout(context.Background(), 10*time.Second)
	defer finish()
	if s.Shutdown(end) != nil {
		s.Close()
	}
	<-httpDone
	cancel()
	r.Close()
	<-done
	p.writer.Close()
	if serverErr != nil {
		os.Exit(1)
	}
}
