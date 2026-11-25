package httpapi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/nexus402/agent-facilitator/internal/chain"
	"github.com/nexus402/agent-facilitator/internal/x402"
)

type Chain interface {
	ChainID(ctx context.Context) (*big.Int, error)
	GetTask(ctx context.Context, taskID [32]byte) (x402.Task, error)
	CanSettle() bool
	Settle(ctx context.Context, req chain.SettleRequest) (string, error)
}

type Server struct {
	Chain Chain
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /facilitator/supported", s.supported)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
	})
	mux.HandleFunc("POST /facilitator/verify", s.verify)
	mux.HandleFunc("POST /facilitator/settle", s.settle)
	return mux
}

func (s *Server) supported(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"networks": []string{x402.Network},
		"assets":   []string{x402.TUSD8},
		"schemes":  []string{"exact", "upto"},
		"chainId":  44005,
	})
}

type verifyBody struct {
	PaymentSignature struct {
		TaskID string `json:"taskId"`
	} `json:"paymentSignature"`
	PaymentRequired struct {
		Scheme        string `json:"scheme"`
		Network       string `json:"network"`
		Asset         string `json:"asset"`
		Amount        string `json:"amount"`
		PayTo         string `json:"payTo"`
		TaskHash      string `json:"taskHash"`
		TimeoutBlocks uint64 `json:"timeoutBlocks"`
	} `json:"paymentRequired"`
}

func (s *Server) verify(w http.ResponseWriter, r *http.Request) {
	if s.Chain == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"isValid": false, "error": "chain client is unset"})
		return
	}
	var body verifyBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"isValid": false, "error": err.Error()})
		return
	}
	taskID, err := chain.ParseBytes32(body.PaymentSignature.TaskID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"isValid": false, "error": "invalid taskId"})
		return
	}
	taskHash, err := chain.ParseBytes32(body.PaymentRequired.TaskHash)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"isValid": false, "error": "invalid taskHash"})
		return
	}
	amount := big.NewInt(0)
	if body.PaymentRequired.Amount != "" {
		if _, ok := amount.SetString(body.PaymentRequired.Amount, 10); !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"isValid": false, "error": "invalid amount"})
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	task, err := s.Chain.GetTask(ctx, taskID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"isValid": false, "taskId": body.PaymentSignature.TaskID, "error": err.Error()})
		return
	}
	ok, reason := x402.Evaluate(task, x402.Requirement{
		Scheme:   body.PaymentRequired.Scheme,
		Network:  body.PaymentRequired.Network,
		Asset:    body.PaymentRequired.Asset,
		Amount:   amount,
		PayTo:    body.PaymentRequired.PayTo,
		TaskHash: taskHash,
	})
	writeJSON(w, http.StatusOK, map[string]any{"isValid": ok, "taskId": body.PaymentSignature.TaskID, "reason": reason})
}

type settleBody struct {
	TaskID          string `json:"taskId"`
	ActualAmount    string `json:"actualAmount"`
	ResultHash      string `json:"resultHash"`
	WorkerSignature string `json:"workerSignature"`
}

func (s *Server) settle(w http.ResponseWriter, r *http.Request) {
	if s.Chain == nil || !s.Chain.CanSettle() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"settled": false, "error": "facilitator gas wallet is unset"})
		return
	}
	var body settleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"settled": false, "error": err.Error()})
		return
	}
	taskID, err := chain.ParseBytes32(body.TaskID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"settled": false, "error": "invalid taskId"})
		return
	}
	resultHash, err := chain.ParseBytes32(body.ResultHash)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"settled": false, "error": "invalid resultHash"})
		return
	}
	signature, err := decodeHex(body.WorkerSignature)
	if err != nil || len(signature) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"settled": false, "error": "invalid workerSignature"})
		return
	}
	var actual *big.Int
	if strings.TrimSpace(body.ActualAmount) != "" && body.ActualAmount != "0" {
		actual = new(big.Int)
		if _, ok := actual.SetString(body.ActualAmount, 10); !ok || actual.Sign() <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"settled": false, "error": "invalid actualAmount"})
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	txHash, err := s.Chain.Settle(ctx, chain.SettleRequest{
		TaskID:          taskID,
		ActualAmount:    actual,
		ResultHash:      resultHash,
		WorkerSignature: signature,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"settled":     false,
			"txHash":      txHash,
			"explorerUrl": explorer(txHash),
			"error":       err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settled":     true,
		"txHash":      txHash,
		"explorerUrl": explorer(txHash),
	})
}

func explorer(txHash string) string {
	if txHash == "" {
		return ""
	}
	return "https://explorer-test.qday.info/tx/" + txHash
}

func decodeHex(raw string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(raw), "0x"))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
