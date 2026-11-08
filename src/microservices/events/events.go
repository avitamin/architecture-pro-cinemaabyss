package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"
)

type event struct {
	ID        string                     `json:"id"`
	Type      string                     `json:"type"`
	Timestamp string                     `json:"timestamp"`
	Payload   map[string]json.RawMessage `json:"payload"`
}

var schemas = map[string]map[string]string{
	"movie":   {"movie_id": "integer", "title": "string", "action": "string"},
	"user":    {"user_id": "integer", "action": "string", "timestamp": "time"},
	"payment": {"payment_id": "integer", "user_id": "integer", "amount": "number", "status": "string", "timestamp": "time"},
}

func validField(raw json.RawMessage, kind string) bool {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	switch kind {
	case "string", "time":
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return false
		}
		if kind == "time" {
			_, e := time.Parse(time.RFC3339Nano, s)
			return e == nil
		}
		return true
	case "integer", "number":
		var n json.Number
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		var v any
		if d.Decode(&v) != nil {
			return false
		}
		var ok bool
		n, ok = v.(json.Number)
		if !ok {
			return false
		}
		if kind == "integer" {
			r, ok := new(big.Rat).SetString(n.String())
			return ok && r.IsInt()
		}
		return true
	case "strings":
		var a []json.RawMessage
		if json.Unmarshal(raw, &a) != nil {
			return false
		}
		for _, item := range a {
			if !validField(item, "string") {
				return false
			}
		}
		return true
	}
	return false
}
func validate(kind string, p map[string]json.RawMessage) error {
	for key, typ := range schemas[kind] {
		if !validField(p[key], typ) {
			return fmt.Errorf("invalid or missing %s", key)
		}
	}
	optional := map[string]string{}
	switch kind {
	case "movie":
		optional = map[string]string{"user_id": "integer", "rating": "number", "genres": "strings", "description": "string"}
	case "user":
		optional = map[string]string{"username": "string", "email": "string"}
	case "payment":
		optional = map[string]string{"method_type": "string"}
	}
	for key, typ := range optional {
		if raw, ok := p[key]; ok && !validField(raw, typ) {
			return fmt.Errorf("invalid %s", key)
		}
	}
	return nil
}
func newEvent(kind string, p map[string]json.RawMessage) (event, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return event{}, e
	}
	return event{hex.EncodeToString(b), kind, time.Now().UTC().Format(time.RFC3339Nano), p}, nil
}
