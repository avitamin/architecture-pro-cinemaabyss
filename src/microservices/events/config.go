package main

import (
	"fmt"
	"net"
	"os"
	"strings"
)

type config struct {
	port    string
	brokers []string
}

func loadConfig() (config, error) {
	c := config{port: os.Getenv("PORT")}
	if c.port == "" {
		c.port = "8082"
	}
	for _, s := range strings.Split(os.Getenv("KAFKA_BROKERS"), ",") {
		s = strings.TrimSpace(s)
		h, p, e := net.SplitHostPort(s)
		if e != nil || h == "" || p == "" {
			return c, fmt.Errorf("invalid KAFKA_BROKERS address")
		}
		c.brokers = append(c.brokers, s)
	}
	return c, nil
}
