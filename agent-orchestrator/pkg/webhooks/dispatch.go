package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nexus402/agent-orchestrator/pkg/types"
	"github.com/nexus402/agent-orchestrator/pkg/x402"
)

var allowedEvents = map[string]bool{
	"SETTLED":         true,
	"REFUNDED":        true,
	"REJECTED":        true,
	"SESSION_REVOKED": true,
}

type Subscription struct {
	URL    string
	Secret string
	Events map[string]bool
}

type Registry struct {
	mu           sync.Mutex
	subs         []Subscription
	allowPrivate bool
	client       *http.Client
	backoff      time.Duration
}

func New(allowPrivate bool) *Registry {
	return &Registry{
		allowPrivate: allowPrivate,
		client:       &http.Client{Timeout: 5 * time.Second},
		backoff:      200 * time.Millisecond,
	}
}

func (r *Registry) Register(rawURL, secret string, events []string) error {
	if strings.TrimSpace(secret) == "" {
		return fmt.Errorf("webhook secret is required")
	}
	if err := ValidateURL(rawURL, r.allowPrivate); err != nil {
		return err
	}
	selected := map[string]bool{}
	for _, event := range events {
		if !allowedEvents[event] {
			return fmt.Errorf("unsupported webhook event %s", event)
		}
		selected[event] = true
	}
	if len(selected) == 0 {
		return fmt.Errorf("at least one webhook event is required")
	}
	r.mu.Lock()
	r.subs = append(r.subs, Subscription{URL: rawURL, Secret: secret, Events: selected})
	r.mu.Unlock()
	return nil
}

func (r *Registry) Dispatch(job *types.TaskJob) {
	event := TerminalEvent(job)
	if event == "" {
		return
	}
	payload := map[string]string{
		"event":      event,
		"jobId":      job.ID,
		"status":     job.Status,
		"taskId":     job.TaskId,
		"resultHash": job.ResultHash,
		"txHash":     firstNonEmpty(job.ClaimTxHash, job.RefundTxHash, job.LockTxHash),
	}
	if job.Amount != nil {
		payload["amount"] = job.Amount.String()
	}
	canonical, err := x402.Canonical(payload)
	if err != nil {
		return
	}
	r.mu.Lock()
	subs := append([]Subscription(nil), r.subs...)
	r.mu.Unlock()
	for _, sub := range subs {
		if !sub.Events[event] {
			continue
		}
		go r.deliver(sub, canonical)
	}
}

func (r *Registry) deliver(sub Subscription, canonical string) {
	mac := hmac.New(sha256.New, []byte(sub.Secret))
	_, _ = mac.Write([]byte(canonical))
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	delay := r.backoff
	if delay <= 0 {
		delay = 200 * time.Millisecond
	}
	for attempt := 0; attempt < 3; attempt++ {
		request, err := http.NewRequest(http.MethodPost, sub.URL, bytes.NewReader([]byte(canonical)))
		if err != nil {
			return
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Nexus402-Signature", signature)
		response, err := r.client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
			response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				return
			}
		}
		if attempt == 2 {
			return
		}
		time.Sleep(delay)
		delay *= 2
	}
}

func TerminalEvent(job *types.TaskJob) string {
	if job == nil {
		return ""
	}
	switch {
	case job.Event == "EVALUATOR_REJECTED" || job.Status == "REJECTED":
		return "REJECTED"
	case job.Status == "REFUNDED" || job.Event == "TASK_REFUNDED":
		return "REFUNDED"
	case job.Event == "SESSION_REVOKED" || job.Status == "BLOCKED_REVOKED":
		return "SESSION_REVOKED"
	case job.Status == "SETTLED":
		return "SETTLED"
	default:
		return ""
	}
}

func ValidateURL(raw string, allowPrivate bool) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("webhook url must be an absolute http(s) url")
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		if allowPrivate {
			return nil
		}
		return fmt.Errorf("webhook url targets a private address")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !allowPrivate && isPrivate(ip) {
			return fmt.Errorf("webhook url targets a private address")
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("webhook host could not be resolved")
	}
	if !allowPrivate {
		for _, ip := range ips {
			if isPrivate(ip) {
				return fmt.Errorf("webhook url targets a private address")
			}
		}
	}
	return nil
}

func isPrivate(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
