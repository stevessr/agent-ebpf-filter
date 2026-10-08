package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"agent-ebpf-filter/pb"
)

// ConnState is the coarse connection status shown in the header.
type ConnState int

const (
	StateConnecting ConnState = iota
	StateConnected
	StateDisconnected
)

func (s ConnState) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	default:
		return "disconnected"
	}
}

// StreamHandler receives decoded events. Received is the local arrival time
// (the wire format carries no timestamp); for backfilled history it is the
// backend's capture time.
type StreamHandler interface {
	OnEvents(events []*pb.Event, received time.Time)
	OnHistory(records []*pb.CapturedEventRecord)
	OnState(state ConnState, detail string)
}

// Stream keeps one websocket subscription to the backend's /ws event feed
// alive, reconnecting with capped backoff, and decodes each EventBatch frame.
type Stream struct {
	cfg     Config
	handler StreamHandler
	dialer  *websocket.Dialer
	http    *http.Client
	// backoff bounds the reconnect delay; tests shrink it.
	minBackoff, maxBackoff time.Duration
}

func NewStream(cfg Config, handler StreamHandler) *Stream {
	return &Stream{
		cfg:        cfg,
		handler:    handler,
		dialer:     &websocket.Dialer{HandshakeTimeout: 5 * time.Second},
		http:       &http.Client{Timeout: 10 * time.Second},
		minBackoff: time.Second,
		maxBackoff: 15 * time.Second,
	}
}

func (s *Stream) headers() http.Header {
	h := http.Header{}
	if s.cfg.Token != "" {
		h.Set("X-API-KEY", s.cfg.Token)
	}
	return h
}

func (s *Stream) wsURL() (string, error) {
	parsed, err := url.Parse(s.cfg.BackendURL)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	default:
		parsed.Scheme = "ws"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/ws"
	return parsed.String(), nil
}

// Run blocks until ctx is cancelled. The first connection is preceded by a
// history backfill so the table is not empty on a quiet system.
func (s *Stream) Run(ctx context.Context) {
	backoff := s.minBackoff
	backfilled := false
	for {
		s.handler.OnState(StateConnecting, "")
		if !backfilled && s.cfg.Backfill > 0 {
			if records, err := s.fetchHistory(ctx, s.cfg.Backfill); err == nil {
				s.handler.OnHistory(records)
				backfilled = true
			}
		}
		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			s.handler.OnState(StateDisconnected, "stopped")
			return
		}
		if err == nil {
			err = errors.New("connection closed")
		}
		s.handler.OnState(StateDisconnected, fmt.Sprintf("%v (retry in %s)", err, backoff))
		select {
		case <-ctx.Done():
			s.handler.OnState(StateDisconnected, "stopped")
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, s.maxBackoff)
	}
}

func (s *Stream) runOnce(ctx context.Context) error {
	target, err := s.wsURL()
	if err != nil {
		return err
	}
	conn, resp, err := s.dialer.DialContext(ctx, target, s.headers())
	if err != nil {
		if resp != nil {
			return fmt.Errorf("%w (HTTP %d)", err, resp.StatusCode)
		}
		return err
	}
	defer conn.Close()
	s.handler.OnState(StateConnected, target)

	// Closing the socket is the only way to interrupt ReadMessage.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		batch := &pb.EventBatch{}
		if err := proto.Unmarshal(data, batch); err != nil {
			return fmt.Errorf("decode EventBatch: %w", err)
		}
		if len(batch.Events) > 0 {
			s.handler.OnEvents(batch.Events, time.Now())
		}
	}
}

// fetchHistory loads the most recent events via /events/recent in protobuf
// form, oldest first.
func (s *Stream) fetchHistory(ctx context.Context, limit int) ([]*pb.CapturedEventRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.BackendURL+"/events/recent?limit="+strconv.Itoa(limit), nil)
	if err != nil {
		return nil, err
	}
	req.Header = s.headers()
	req.Header.Set("Accept", "application/x-protobuf")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/events/recent: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	history := &pb.EventHistoryResponse{}
	if err := proto.Unmarshal(body, history); err != nil {
		return nil, fmt.Errorf("decode EventHistoryResponse: %w", err)
	}
	records := history.Events
	// The backend returns newest first; the table wants chronological order.
	if len(records) > 1 && records[0].GetTimestamp() > records[len(records)-1].GetTimestamp() {
		for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
			records[i], records[j] = records[j], records[i]
		}
	}
	return records, nil
}
