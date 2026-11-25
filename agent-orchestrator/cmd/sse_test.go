package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type flushBuffer struct {
	body    strings.Builder
	flushes int
}

func (f *flushBuffer) Header() http.Header         { return http.Header{} }
func (f *flushBuffer) Write(p []byte) (int, error) { return f.body.Write(p) }
func (f *flushBuffer) WriteHeader(int)             {}
func (f *flushBuffer) Flush()                      { f.flushes++ }

func TestSSEHeartbeatKeepsTheStreamOpen(t *testing.T) {
	buffer := &flushBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan string)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		streamEvents(ctx, buffer, buffer.Flush, events, stop, 20*time.Millisecond)
		close(done)
	}()
	time.Sleep(70 * time.Millisecond)
	cancel()
	<-done
	if strings.Count(buffer.body.String(), ": ping\n\n") < 2 {
		t.Fatalf("expected repeated heartbeats, got %q", buffer.body.String())
	}
}

func TestSSEHeartbeatAcrossOneMinute(t *testing.T) {
	if os.Getenv("NEXUS_SSE_LIVE") == "" {
		t.Skip("set NEXUS_SSE_LIVE=1 to hold a stream open for 62 seconds")
	}
	buffer := &flushBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		streamEvents(ctx, buffer, buffer.Flush, make(chan string), make(chan struct{}), sseHeartbeat)
		close(done)
	}()
	time.Sleep(62 * time.Second)
	cancel()
	<-done
	if strings.Count(buffer.body.String(), ": ping\n\n") < 4 {
		t.Fatalf("expected a ping every 15 seconds, got %q", buffer.body.String())
	}
}
