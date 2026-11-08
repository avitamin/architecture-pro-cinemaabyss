package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/segmentio/kafka-go"
	"io"
	"sync"
	"time"
)

type kafkaPublisher struct{ writer *kafka.Writer }
type publication struct {
	pos coordinates
	err error
}

func complete(messages []kafka.Message, e error) {
	for _, m := range messages {
		if ch, ok := m.WriterData.(chan publication); ok {
			select {
			case ch <- publication{coordinates{m.Partition, m.Offset}, e}:
			default:
			}
		}
	}
}
func newPublisher(b []string) *kafkaPublisher {
	return &kafkaPublisher{&kafka.Writer{Addr: kafka.TCP(b...), RequiredAcks: kafka.RequireAll, BatchSize: 1, MaxAttempts: 2, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, Transport: &kafka.Transport{DialTimeout: 3 * time.Second}, Completion: complete}}
}
func (p *kafkaPublisher) Publish(ctx context.Context, e event) (coordinates, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return coordinates{}, err
	}
	ch := make(chan publication, 1)
	err = p.writer.WriteMessages(ctx, kafka.Message{Topic: e.Type + "-events", Key: []byte(e.ID), Value: b, WriterData: ch})
	if err != nil {
		return coordinates{}, err
	}
	select {
	case r := <-ch:
		return r.pos, r.err
	case <-ctx.Done():
		return coordinates{}, ctx.Err()
	}
}

type messageReader interface {
	FetchMessage(context.Context) (kafka.Message, error)
	CommitMessages(context.Context, ...kafka.Message) error
}
type resultLogger struct {
	mu  sync.Mutex
	out io.Writer
}

func (l *resultLogger) record(m kafka.Message) error {
	var e event
	err := json.Unmarshal(m.Value, &e)
	if err == nil {
		if _, ok := schemas[e.Type]; !ok || e.ID == "" {
			err = fmt.Errorf("invalid envelope")
		} else if _, x := time.Parse(time.RFC3339Nano, e.Timestamp); x != nil {
			err = x
		} else {
			err = validate(e.Type, e.Payload)
		}
	}
	v := map[string]any{"topic": m.Topic, "partition": m.Partition, "offset": m.Offset}
	if err != nil {
		v["message"] = "invalid_envelope"
		v["error"] = err.Error()
	} else {
		v["message"] = "event_processed"
		v["event_id"] = e.ID
		v["type"] = e.Type
		v["domain_id"] = e.Payload[e.Type+"_id"]
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return json.NewEncoder(l.out).Encode(v)
}
func pause(ctx context.Context) bool {
	t := time.NewTimer(time.Second)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
func consume(ctx context.Context, r messageReader, logResult func(kafka.Message) error) {
	for ctx.Err() == nil {
		m, e := r.FetchMessage(ctx)
		if e != nil {
			if !pause(ctx) {
				return
			}
			continue
		}
		for {
			if e = logResult(m); e == nil {
				break
			}
			if !pause(ctx) {
				return
			}
		}
		for {
			if e = r.CommitMessages(ctx, m); e == nil {
				break
			}
			if !pause(ctx) {
				return
			}
		}
	}
}
