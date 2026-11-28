package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/nexus402/agent-orchestrator/pkg/evaluator"
	"github.com/nexus402/agent-orchestrator/pkg/types"
	"github.com/nexus402/agent-orchestrator/pkg/x402"
)

func (s *Server) runEvaluatorDispute() {
	prompt := "Explain why a capped session key is safer than an unlimited token approval."
	job := &types.TaskJob{
		ID:        uuid.New().String(),
		Prompt:    prompt,
		TaskType:  "ai_synthesis",
		Service:   "ai_synthesis",
		Status:    "PENDING",
		CreatedAt: time.Now().UTC(),
		Scheme:    "auth-capture",
		Amount:    big.NewInt(300_000),
	}
	s.remember(job)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	evaluatorKey := strings.TrimSpace(os.Getenv("EVALUATOR_PRIVATE_KEY"))
	if evaluatorKey == "" {
		s.failDispute(job, fmt.Errorf("EVALUATOR_PRIVATE_KEY is unset"))
		return
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(evaluatorKey, "0x"))
	if err != nil {
		s.failDispute(job, fmt.Errorf("invalid evaluator key: %w", err))
		return
	}
	evaluatorAddress := crypto.PubkeyToAddress(key.PublicKey)

	workerURL, err := s.directory.Endpoint("ai_synthesis")
	if err != nil || workerURL == "" {
		s.failDispute(job, fmt.Errorf("no discovered ai_synthesis worker"))
		return
	}
	var worker common.Address
	for _, manifest := range s.directory.List() {
		if manifest.URL == workerURL && common.IsHexAddress(manifest.Worker) {
			worker = common.HexToAddress(manifest.Worker)
		}
	}
	if worker == (common.Address{}) {
		s.failDispute(job, fmt.Errorf("discovered worker has no address"))
		return
	}
	job.WorkerURL = workerURL

	taskHash, err := challengeTaskHash(ctx, workerURL, prompt)
	if err != nil {
		s.failDispute(job, err)
		return
	}
	before, _, _, beforeBps, repErr := s.aevumClient.WorkerReputation(ctx, worker)
	if repErr == nil {
		job.PreviousReliability = beforeBps
	}
	_ = before

	taskID, lockTx, _, err := s.aevumClient.LockEvaluatedEscrow(ctx, worker, evaluatorAddress, big.NewInt(300_000), 50, 30, taskHash)
	if err != nil {
		job.LockTxHash = lockTx
		s.failDispute(job, err)
		return
	}
	job.LockTxHash = lockTx
	job.TaskId = "0x" + hex.EncodeToString(taskID[:])
	job.TaskHash = "0x" + hex.EncodeToString(taskHash[:])
	job.Status = "ESCROW_LOCKED"
	job.Event = "ESCROW_LOCKED"
	s.publishJob(job)

	body, _ := json.Marshal(map[string]any{
		"prompt":           prompt,
		"service":          "ai_synthesis",
		"scheme":           "auth-capture",
		"simulate_garbage": true,
	})
	wire, _ := json.Marshal(map[string]any{
		"scheme":     "auth-capture",
		"taskId":     job.TaskId,
		"lockTxHash": lockTx,
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, workerURL+"/api/v1/compute", bytes.NewReader(body))
	if err != nil {
		s.failDispute(job, err)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("PAYMENT-SIGNATURE", base64.StdEncoding.EncodeToString(wire))
	request.Header.Set("x-simulate-garbage", "1")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		s.failDispute(job, err)
		return
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != http.StatusOK {
		s.failDispute(job, fmt.Errorf("worker returned %d: %s", response.StatusCode, string(payload)))
		return
	}
	var submitted struct {
		Result     string `json:"result"`
		ResultHash string `json:"result_hash"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(payload, &submitted); err != nil {
		s.failDispute(job, err)
		return
	}
	job.Result = submitted.Result
	job.ResultHash = submitted.ResultHash
	approved, reason := evaluator.Inspect(prompt, submitted.Result, submitted.ResultHash)
	if approved {
		s.failDispute(job, fmt.Errorf("evaluator accepted garbage output"))
		return
	}
	refundTx, _, err := s.aevumClient.EvaluateTask(ctx, evaluatorKey, taskID, false, reason)
	if err != nil {
		job.RefundTxHash = refundTx
		s.failDispute(job, err)
		return
	}
	_, _, _, afterBps, repErr := s.aevumClient.WorkerReputation(ctx, worker)
	if repErr != nil {
		s.failDispute(job, repErr)
		return
	}
	if job.PreviousReliability != 0 && afterBps >= job.PreviousReliability {
		s.failDispute(job, fmt.Errorf("reliability did not fall: %d -> %d", job.PreviousReliability, afterBps))
		return
	}
	doc, err := x402.BuildReceipt(x402.ReceiptInput{
		Principal:    s.aevumClient.PrincipalAddress().Hex(),
		Agent:        s.aevumClient.AgentAddress().Hex(),
		ChainID:      44005,
		HourlyLimit:  s.hourlyLimit.String(),
		PerTaskLimit: s.perTaskCap.String(),
		Nonce:        job.ID,
		TaskID:       job.TaskId,
		Amount:       "300000",
		Scheme:       "auth-capture",
		ResultHash:   submitted.ResultHash,
		LockTx:       lockTx,
		RefundTx:     refundTx,
		Reason:       reason,
	})
	if err != nil {
		s.failDispute(job, err)
		return
	}
	s.storeReceipt(doc)
	job.Status = "REFUNDED"
	job.RefundTxHash = refundTx
	job.ReliabilityBps = afterBps
	job.Event = "EVALUATOR_REJECTED"
	job.Result = reason + ": " + submitted.Result
	job.SettledAt = time.Now().UTC()
	s.publishJob(job)
}

func (s *Server) failDispute(job *types.TaskJob, err error) {
	job.Status = "FAILED"
	job.Error = err.Error()
	job.Event = "EVALUATOR_FAILED"
	s.publishJob(job)
}

func (s *Server) storeReceipt(doc x402.ReceiptDocument) {
	s.receiptMu.Lock()
	defer s.receiptMu.Unlock()
	if s.receipts == nil {
		s.receipts = map[string]x402.ReceiptDocument{}
	}
	s.receipts[strings.ToLower(doc.TaskID)] = doc
}

func challengeTaskHash(ctx context.Context, workerURL, prompt string) ([32]byte, error) {
	var taskHash [32]byte
	body, _ := json.Marshal(map[string]string{"prompt": prompt, "service": "ai_synthesis"})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, workerURL+"/api/v1/compute", bytes.NewReader(body))
	if err != nil {
		return taskHash, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return taskHash, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	raw := response.Header.Get("X-Task-Hash")
	if raw == "" {
		if encoded := response.Header.Get("PAYMENT-REQUIRED"); encoded != "" {
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err == nil {
				var envelope struct {
					Accepts []struct {
						TaskHash string `json:"taskHash"`
					} `json:"accepts"`
				}
				if json.Unmarshal(decoded, &envelope) == nil && len(envelope.Accepts) > 0 {
					raw = envelope.Accepts[0].TaskHash
				}
			}
		}
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(raw, "0x"))
	if err != nil || len(decoded) != 32 {
		return taskHash, fmt.Errorf("worker did not return a task hash")
	}
	copy(taskHash[:], decoded)
	return taskHash, nil
}
