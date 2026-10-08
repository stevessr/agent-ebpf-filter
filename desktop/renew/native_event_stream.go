package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

)

func (a *renewApp) runEventSummaryStream(ctx context.Context) {
	for ctx.Err() == nil {
		err := a.consumeEventSummaryStream(ctx)
		a.update(func() {
			a.eventStreamConnected = false
			if err != nil && ctx.Err() == nil {
				a.eventStreamErr = err.Error()
			}
		})
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (a *renewApp) consumeEventSummaryStream(ctx context.Context) error {
	if a.client == nil {
		return fmt.Errorf("event summary stream: API client unavailable")
	}
	endpoint, err := eventSummaryWebSocketURL(a.client.origin)
	if err != nil {
		return err
	}
	header := http.Header{}
	if a.client.token != "" {
		header.Set("X-API-KEY", a.client.token)
		header.Set("Authorization", "Bearer "+a.client.token)
	}
	dialer := a.client.websocketDialer()
	conn, _, err := dialer.DialContext(ctx, endpoint, header)
	if err != nil {
		return fmt.Errorf("event summary websocket: %w", err)
	}
	defer conn.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	a.update(func() {
		a.eventStreamConnected = true
		a.eventStreamErr = ""
	})

	for ctx.Err() == nil {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("event summary read: %w", err)
		}
		var batch eventSummaryResponse
		if err := json.Unmarshal(data, &batch); err != nil {
			return fmt.Errorf("event summary decode: %w", err)
		}
		if len(batch.Events) == 0 {
			continue
		}
		a.queueEventSummaries(batch.Events)
	}
	return ctx.Err()
}

func eventSummaryWebSocketURL(origin string) (string, error) {
	parsed, err := url.Parse(origin)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported backend scheme %q", parsed.Scheme)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/ws/event-summaries"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
