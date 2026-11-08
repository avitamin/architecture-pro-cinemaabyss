package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	c, e := loadConfig()
	if e != nil {
		slog.Error("configuration", "error", e)
		os.Exit(1)
	}
	s := &http.Server{Addr: ":" + c.port, Handler: newProxy(c), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	var serverErr error
	go func() {
		defer close(done)
		if e := s.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			serverErr = e
			slog.Error("http", "error", e)
			stop()
		}
	}()
	<-ctx.Done()
	end, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if s.Shutdown(end) != nil {
		s.Close()
	}
	<-done
	if serverErr != nil {
		os.Exit(1)
	}
}
