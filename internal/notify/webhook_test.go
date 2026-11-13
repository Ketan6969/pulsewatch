package notify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"pulsewatch/internal/domain"
)

type hooks []domain.Webhook

func (h hooks) ListWebhooks(context.Context) ([]domain.Webhook, error) { return h, nil }

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestNotifySignsAndRetries(t *testing.T) {
	var calls atomic.Int32
	got := make(chan Payload, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway) // transient: should retry
			return
		}
		body, _ := io.ReadAll(r.Body)
		want := "sha256=" + Sign("s3cret", r.Header.Get("X-Pulsewatch-Timestamp"), body)
		if r.Header.Get("X-Pulsewatch-Signature") != want {
			t.Errorf("bad signature")
		}
		var p Payload
		_ = json.Unmarshal(body, &p)
		got <- p
	}))
	defer srv.Close()

	n := New(hooks{{ID: "1", Name: "h", URL: srv.URL, Secret: "s3cret", Enabled: true},
		{ID: "2", Name: "disabled", URL: "http://127.0.0.1:1", Enabled: false}}, time.Second, discard())
	n.backoff = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { n.Run(ctx, time.Second); close(done) }()

	n.Notify(ctx, Payload{Event: EventFiring, Text: "x", Alert: &domain.Alert{ID: "a1"}})
	select {
	case p := <-got:
		if p.Event != EventFiring || p.Alert.ID != "a1" {
			t.Fatalf("unexpected payload %+v", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("webhook not delivered")
	}
	cancel()
	<-done
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestPermanentFailureNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	n := New(nil, time.Second, discard())
	n.backoff = time.Millisecond
	n.deliver(context.Background(), job{hook: domain.Webhook{URL: srv.URL}, payload: []byte("{}")})
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}
