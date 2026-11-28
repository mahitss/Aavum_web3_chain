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
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/nexus402/agent-orchestrator/pkg/types"
)

func (s *Server) runVoucherBurst() {
	job := &types.TaskJob{
		ID:        uuid.New().String(),
		Prompt:    "Ten off-chain micro-calls against one session tab",
		TaskType:  "micro_call",
		Service:   "micro_call",
		Status:    "PENDING",
		CreatedAt: time.Now().UTC(),
		Scheme:    "batch-settlement",
	}
	s.remember(job)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	workerURL, err := s.directory.Endpoint("micro_call")
	if err != nil || workerURL == "" {
		workerURL, _ = s.directory.Endpoint("ai_synthesis")
	}
	var worker common.Address
	for _, manifest := range s.directory.List() {
		if manifest.URL == workerURL && common.IsHexAddress(manifest.Worker) {
			worker = common.HexToAddress(manifest.Worker)
		}
	}
	if worker == (common.Address{}) {
		s.failBurst(job, fmt.Errorf("no discovered worker address for the voucher tab"))
		return
	}
	tabID, openTx, err := s.aevumClient.OpenSessionTab(ctx, worker, big.NewInt(1_000_000), 50)
	if err != nil {
		job.LockTxHash = openTx
		s.failBurst(job, err)
		return
	}
	job.LockTxHash = openTx
	job.TaskId = "0x" + hex.EncodeToString(tabID[:])
	job.Status = "ESCROW_LOCKED"
	s.publishJob(job)

	client := &http.Client{Timeout: 15 * time.Second}
	validBefore := uint64(time.Now().Add(time.Hour).Unix())
	var total int64
	for step := 1; step <= 10; step++ {
		signature, err := s.aevumClient.SignVoucher(tabID, big.NewInt(int64(step)*50_000), uint64(step), validBefore)
		if err != nil {
			s.failBurst(job, err)
			return
		}
		body, _ := json.Marshal(map[string]string{"prompt": fmt.Sprintf("micro %d", step), "service": "micro_call"})
		wire, _ := json.Marshal(map[string]any{
			"scheme":           "batch-settlement",
			"tabId":            job.TaskId,
			"cumulativeAmount": fmt.Sprintf("%d", step*50_000),
			"nonce":            step,
			"validBefore":      validBefore,
			"agentSignature":   "0x" + hex.EncodeToString(signature),
			"agentKey":         s.aevumClient.AgentAddress().Hex(),
			"principal":        s.aevumClient.PrincipalAddress().Hex(),
		})
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, workerURL+"/api/v1/compute", bytes.NewReader(body))
		if err != nil {
			s.failBurst(job, err)
			return
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("PAYMENT-SIGNATURE", base64.StdEncoding.EncodeToString(wire))
		request.Header.Set("x-defer-voucher-flush", "1")
		started := time.Now()
		response, err := client.Do(request)
		elapsed := time.Since(started).Milliseconds()
		total += elapsed
		if err != nil {
			s.failBurst(job, err)
			return
		}
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			s.failBurst(job, fmt.Errorf("micro-call %d returned %d: %s", step, response.StatusCode, string(payload)))
			return
		}
	}
	settleRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, workerURL+"/api/v1/tabs/"+job.TaskId+"/settle", nil)
	if err != nil {
		s.failBurst(job, err)
		return
	}
	settleResponse, err := client.Do(settleRequest)
	if err != nil {
		s.failBurst(job, err)
		return
	}
	defer settleResponse.Body.Close()
	var settled struct {
		Settled bool   `json:"settled"`
		TxHash  string `json:"txHash"`
		Error   string `json:"error"`
	}
	_ = json.NewDecoder(settleResponse.Body).Decode(&settled)
	if !settled.Settled || settled.TxHash == "" {
		s.failBurst(job, fmt.Errorf("batch settlement failed: %s", settled.Error))
		return
	}
	job.Status = "SETTLED"
	job.ClaimTxHash = settled.TxHash
	job.VoucherCalls = 10
	job.AvgLatencyMs = total / 10
	job.GasSaved = "90% fewer settlement transactions than 10 separate escrows"
	job.Event = "VOUCHER_BATCH_SETTLED"
	job.SettledAt = time.Now().UTC()
	s.publishJob(job)
}

func (s *Server) failBurst(job *types.TaskJob, err error) {
	job.Status = "FAILED"
	job.Error = err.Error()
	job.Event = "VOUCHER_BATCH_FAILED"
	s.publishJob(job)
}
