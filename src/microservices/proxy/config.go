package main

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
)

type config struct {
	port    string
	urls    map[string]*url.URL
	gradual bool
	percent int
}

func loadConfig() (config, error) {
	c := config{port: os.Getenv("PORT"), urls: map[string]*url.URL{}}
	if c.port == "" {
		c.port = "8000"
	}
	for name, key := range map[string]string{"monolith": "MONOLITH_URL", "movies": "MOVIES_SERVICE_URL", "events": "EVENTS_SERVICE_URL"} {
		u, e := url.Parse(os.Getenv(key))
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return c, fmt.Errorf("invalid %s", key)
		}
		c.urls[name] = u
	}
	var e error
	c.gradual, e = strconv.ParseBool(os.Getenv("GRADUAL_MIGRATION"))
	if e != nil {
		return c, fmt.Errorf("invalid GRADUAL_MIGRATION: %w", e)
	}
	c.percent, e = strconv.Atoi(os.Getenv("MOVIES_MIGRATION_PERCENT"))
	if e != nil || c.percent < 0 || c.percent > 100 {
		return c, fmt.Errorf("MOVIES_MIGRATION_PERCENT must be 0..100")
	}
	return c, nil
}
