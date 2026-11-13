// Package notify delivers alert notifications to webhooks.
package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"pulsewatch/internal/domain"
)

// Event names in webhook payloads.
const (
	EventFiring   = "alert.firing"
	EventResolved = "alert.resolved"
	EventTest     = "test"
)

// Payload is the JSON body POSTed to webhooks. The top-level "text" field
// makes it directly usable with Slack/Mattermost-style incoming webhooks.
type Payload struct {
	Event  string        `json:"event"`
	Text   string        `json:"text"`
	Alert  *domain.Alert `json:"alert,omitempty"`
	SentAt time.Time     `json:"sent_at"`
}

// WebhookSource lists delivery destinations.
type WebhookSource interface {
	ListWebhooks(ctx context.Context) ([]domain.Webhook, error)
}

type job struct {
	hook    domain.Webhook
	payload []byte
}

// Notifier fans payloads out to all enabled webhooks via a bounded queue
// and a fixed worker pool, with retries and HMAC signing.
type Notifier struct {
	source  WebhookSource
	client  *http.Client
	log     *slog.Logger
	queue   chan job
	workers int
	backoff time.Duration

	closeOnce sync.Once
	closed    chan struct{}
}

// New creates a Notifier. timeout bounds each delivery attempt.
func New(source WebhookSource, timeout time.Duration, log *slog.Logger) *Notifier {
	return &Notifier{
		source:  source,
		client:  &http.Client{Timeout: timeout},
		log:     log,
		queue:   make(chan job, 1024),
		workers: 4,
		backoff: time.Second,
		closed:  make(chan struct{}),
	}
}

// Notify enqueues p for every enabled webhook. It does not block on
// delivery; if the queue is full the notification is dropped and logged.
func (n *Notifier) Notify(ctx context.Context, p Payload) {
	if p.SentAt.IsZero() {
		p.SentAt = time.Now().UTC()
	}
	hooks, err := n.source.ListWebhooks(ctx)
	if err != nil {
		n.log.Error("list webhooks", "err", err)
		return
	}
	body, err := json.Marshal(p)
	if err != nil {
		n.log.Error("marshal payload", "err", err)
		return
	}
	for _, h := range hooks {
		if !h.Enabled {
			continue
		}
		select {
		case <-n.closed:
			n.log.Warn("notifier stopped; dropping notification", "webhook", h.Name, "event", p.Event)
			return
		default:
		}
		select {
		case n.queue <- job{hook: h, payload: body}:
		default:
			n.log.Error("notification queue full; dropping", "webhook", h.Name, "event", p.Event)
		}
	}
}

// Run processes the queue until ctx is cancelled, then spends up to
// drainTimeout delivering what is already queued.
func (n *Notifier) Run(ctx context.Context, drainTimeout time.Duration) {
	var wg sync.WaitGroup
	for range n.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case j := <-n.queue:
					n.deliver(ctx, j)
				}
			}
		}()
	}
	wg.Wait()
	n.closeOnce.Do(func() { close(n.closed) })

	dctx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()
	for {
		select {
		case j := <-n.queue:
			if dctx.Err() != nil {
				n.log.Warn("shutdown: dropping undelivered notification", "webhook", j.hook.Name)
				continue
			}
			n.deliverOnce(dctx, j)
		default:
			return
		}
	}
}

func (n *Notifier) deliver(ctx context.Context, j job) {
	const attempts = 3
	delay := n.backoff
	for i := 1; i <= attempts; i++ {
		err := n.deliverOnce(ctx, j)
		if err == nil {
			return
		}
		var perm permanentError
		if errors.As(err, &perm) || i == attempts || ctx.Err() != nil {
			n.log.Error("webhook delivery failed", "webhook", j.hook.Name, "attempt", i, "err", err)
			return
		}
		n.log.Warn("webhook delivery failed; retrying", "webhook", j.hook.Name, "attempt", i, "err", err)
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		delay *= 2
	}
}

type permanentError struct{ error }

// Send performs a single synchronous delivery; used by the "test webhook" API.
func (n *Notifier) Send(ctx context.Context, h domain.Webhook, p Payload) error {
	if p.SentAt.IsZero() {
		p.SentAt = time.Now().UTC()
	}
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return n.deliverOnce(ctx, job{hook: h, payload: body})
}

func (n *Notifier) deliverOnce(ctx context.Context, j job) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.hook.URL, bytes.NewReader(j.payload))
	if err != nil {
		return permanentError{err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Pulsewatch-Webhook/1.0")
	if j.hook.Secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		req.Header.Set("X-Pulsewatch-Timestamp", ts)
		req.Header.Set("X-Pulsewatch-Signature", "sha256="+Sign(j.hook.Secret, ts, j.payload))
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	switch {
	case resp.StatusCode < 300:
		return nil
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("status %d", resp.StatusCode)
	default:
		return permanentError{fmt.Errorf("status %d", resp.StatusCode)}
	}
}

// Sign computes the hex HMAC-SHA256 of "timestamp.body". Receivers should
// recompute it and compare in constant time, and reject stale timestamps.
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
