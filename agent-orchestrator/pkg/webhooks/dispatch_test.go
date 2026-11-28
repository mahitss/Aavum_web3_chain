package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexus402/agent-orchestrator/pkg/types"
	"github.com/nexus402/agent-orchestrator/pkg/x402"
)

func TestRejectsLoopbackUnlessAllowed(t *testing.T) {
	if err := ValidateURL("http://127.0.0.1/hook", false); err == nil {
		t.Fatal("loopback webhook was accepted")
	}
	if err := ValidateURL("http://127.0.0.1/hook", true); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchSignsCanonicalPayload(t *testing.T) {
	var hits atomic.Int32
	var gotSignature, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotSignature = r.Header.Get("X-Nexus402-Signature")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		if hits.Load() == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	registry := New(true)
	registry.backoff = 10 * time.Millisecond
	if err := registry.Register(server.URL, "test-secret", []string{"SETTLED"}); err != nil {
		t.Fatal(err)
	}
	registry.Dispatch(&types.TaskJob{ID: "job-1", Status: "SETTLED", TaskId: "0xabc"})
	deadline := time.Now().Add(2 * time.Second)
	for hits.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(15 * time.Millisecond)
	}
	if hits.Load() < 2 {
		t.Fatalf("expected a retry, got %d hits", hits.Load())
	}
	canonical, err := x402.Canonical(map[string]string{
		"event": "SETTLED", "jobId": "job-1", "status": "SETTLED", "taskId": "0xabc", "resultHash": "", "txHash": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody != canonical {
		t.Fatalf("body = %s", gotBody)
	}
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write([]byte(canonical))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotSignature != want {
		t.Fatalf("signature = %s", gotSignature)
	}
}
