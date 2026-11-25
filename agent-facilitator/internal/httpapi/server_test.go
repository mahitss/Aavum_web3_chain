package httpapi

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nexus402/agent-facilitator/internal/chain"
	"github.com/nexus402/agent-facilitator/internal/x402"
)

type fakeChain struct {
	task    x402.Task
	err     error
	settled bool
}

func (f *fakeChain) ChainID(context.Context) (*big.Int, error) { return big.NewInt(44005), nil }
func (f *fakeChain) GetTask(context.Context, [32]byte) (x402.Task, error) {
	return f.task, f.err
}
func (f *fakeChain) CanSettle() bool { return f.settled }
func (f *fakeChain) Settle(context.Context, chain.SettleRequest) (string, error) {
	return "", nil
}

func TestSupported(t *testing.T) {
	server := (&Server{Chain: &fakeChain{}}).Handler()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/facilitator/supported", nil)
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	var body struct {
		Networks []string `json:"networks"`
		Schemes  []string `json:"schemes"`
		Assets   []string `json:"assets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Networks) != 1 || body.Networks[0] != "eip155:44005" {
		t.Fatalf("networks %#v", body.Networks)
	}
	if body.Schemes[0] != "exact" || body.Schemes[1] != "upto" {
		t.Fatalf("schemes %#v", body.Schemes)
	}
	if body.Assets[0] != x402.TUSD8 {
		t.Fatalf("asset %s", body.Assets[0])
	}
}

func TestVerifyRejectsMalformedTask(t *testing.T) {
	server := (&Server{Chain: &fakeChain{}}).Handler()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/facilitator/verify", strings.NewReader(`{"paymentSignature":{"taskId":"0x11"}}`))
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", response.Code, response.Body.String())
	}
}

func TestSettleWithoutWalletIsClean(t *testing.T) {
	server := (&Server{Chain: &fakeChain{}}).Handler()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/facilitator/settle", strings.NewReader(`{}`))
	server.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", response.Code)
	}
}
