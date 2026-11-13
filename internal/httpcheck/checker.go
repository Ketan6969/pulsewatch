// Package httpcheck performs HTTP availability checks and schedules them.
package httpcheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pulsewatch/internal/domain"
)

const (
	maxBodyRead  = 1 << 20 // bytes read from a response to measure full transfer time
	maxRedirects = 5
	userAgent    = "Pulsewatch/1.0 (+uptime-check)"
)

// Checker executes single HTTP checks. It is safe for concurrent use.
type Checker struct {
	client *http.Client
	now    func() time.Time
}

// NewChecker returns a Checker with a transport tuned for monitoring:
// keep-alives are disabled so every check measures a full connection
// (DNS, TCP, TLS) the way a new client would, and no idle sockets are
// held open between checks.
func NewChecker() *Checker {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DisableKeepAlives = true
	tr.MaxResponseHeaderBytes = 64 << 10
	tr.ResponseHeaderTimeout = 0 // governed by the per-check context
	return &Checker{
		client: &http.Client{
			Transport: tr,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("stopped after %d redirects", maxRedirects)
				}
				return nil
			},
		},
		now: time.Now,
	}
}

// Check performs one request for m. It never returns an error: failures
// are reported in the result so they can be persisted and alerted on.
func (c *Checker) Check(ctx context.Context, m domain.Monitor) domain.CheckResult {
	res := domain.CheckResult{MonitorID: m.ID, CheckedAt: c.now().UTC()}
	timeout := time.Duration(m.TimeoutMS) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, m.Method, m.URL, nil)
	if err != nil {
		res.Error = "invalid request: " + err.Error()
		return res
	}
	req.Header.Set("User-Agent", userAgent)
	for k, v := range m.Headers {
		if strings.EqualFold(k, "host") {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		res.ResponseMS = elapsedMS(start)
		res.Error = describeError(err, timeout)
		return res
	}
	_, copyErr := io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyRead))
	resp.Body.Close()
	res.ResponseMS = elapsedMS(start)
	res.StatusCode = resp.StatusCode

	switch {
	case copyErr != nil && !errors.Is(copyErr, io.EOF):
		res.Error = "reading body: " + describeError(copyErr, timeout)
	case !statusOK(resp.StatusCode, m.ExpectedStatus):
		if m.ExpectedStatus != 0 {
			res.Error = fmt.Sprintf("unexpected status %d (expected %d)", resp.StatusCode, m.ExpectedStatus)
		} else {
			res.Error = fmt.Sprintf("unexpected status %d", resp.StatusCode)
		}
	default:
		res.Success = true
	}
	return res
}

func statusOK(code, expected int) bool {
	if expected != 0 {
		return code == expected
	}
	return code >= 200 && code < 400
}

// elapsedMS rounds up so sub-millisecond responses report 1ms, not 0.
func elapsedMS(start time.Time) int {
	return int((time.Since(start) + time.Millisecond - 1) / time.Millisecond)
}

// describeError turns transport errors into short, operator-friendly text.
func describeError(err error, timeout time.Duration) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("timeout after %s", timeout)
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return fmt.Sprintf("timeout after %s", timeout)
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns: " + dnsErr.Err + " (" + dnsErr.Name + ")"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return "connection failed: " + opErr.Err.Error()
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err.Error()
	}
	return err.Error()
}
