package x402

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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/nexus402/agent-orchestrator/pkg/aevum"
	"github.com/nexus402/agent-orchestrator/pkg/types"
)

type Client struct {
	httpClient  *http.Client
	aevumClient *aevum.Client
	workerURL   string
	hourlyLimit *big.Int
	perTaskCap  *big.Int

	mu     sync.Mutex
	spent  *big.Int
	Notify func(*types.TaskJob)
}

func NewClient(workerURL string, aevumClient *aevum.Client, perTaskCap *big.Int, hourlyLimit *big.Int) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: 2 * time.Minute},
		aevumClient: aevumClient,
		workerURL:   strings.TrimRight(workerURL, "/"),
		hourlyLimit: new(big.Int).Set(hourlyLimit),
		perTaskCap:  new(big.Int).Set(perTaskCap),
		spent:       big.NewInt(0),
	}
}

func (c *Client) Spent() *big.Int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return new(big.Int).Set(c.spent)
}

func (c *Client) ExecuteTask(ctx context.Context, job *types.TaskJob) error {
	service := canonicalService(job)
	reqBody, err := json.Marshal(struct {
		Prompt          string   `json:"prompt"`
		Service         string   `json:"service"`
		SimulateTimeout bool     `json:"simulate_timeout,omitempty"`
		Scheme          string   `json:"scheme,omitempty"`
		Batch           []string `json:"batch,omitempty"`
	}{Prompt: job.Prompt, Service: service, SimulateTimeout: job.SimulateTimeout, Scheme: job.Scheme, Batch: job.Batch})
	if err != nil {
		return fmt.Errorf("failed to serialize payload: %w", err)
	}

	workerBase := c.workerURL
	if job.WorkerURL != "" {
		workerBase = strings.TrimRight(job.WorkerURL, "/")
	}
	computeURL := workerBase + "/api/v1/compute"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, computeURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("initial request to worker failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusPaymentRequired {
		return fmt.Errorf("expected 402 Payment Required from worker, received: %d", resp.StatusCode)
	}
	job.PaymentRequired = resp.Header.Get("PAYMENT-REQUIRED")

	job.Status = "402_CHALLENGED"
	c.notify(job)
	challenge, err := parsePaymentHeaders(resp.Header, job.Scheme)
	if err != nil {
		return fmt.Errorf("invalid 402 challenge headers: %w", err)
	}
	job.Amount = challenge.Amount
	job.TaskHash = "0x" + hex.EncodeToString(challenge.TaskHash[:])

	reserved := false
	if !job.BypassLocalCap {
		if err := c.reserve(challenge.Amount); err != nil {
			return err
		}
		reserved = true
	}
	if job.Scheme == "batch-settlement" {
		return c.settleOneVoucher(ctx, job, challenge, reqBody, computeURL, reserved)
	}

	var taskID [32]byte
	var lockTxHash string
	var deadline uint64
	if job.Evaluator || job.Scheme == "auth-capture" {
		taskID, lockTxHash, deadline, err = c.lockEvaluated(ctx, challenge)
	} else {
		taskID, lockTxHash, deadline, err = c.aevumClient.LockEscrow(ctx, challenge.Receiver, challenge.Amount, challenge.TimeoutBlocks, challenge.TaskHash)
	}
	if err != nil {
		if reserved {
			c.release(challenge.Amount)
		}
		if lockTxHash != "" {
			job.LockTxHash = lockTxHash
		}
		return fmt.Errorf("on-chain escrow lock failed: %w", err)
	}

	job.Status = "ESCROW_LOCKED"
	job.TaskId = "0x" + hex.EncodeToString(taskID[:])
	job.LockTxHash = lockTxHash
	job.Deadline = deadline
	c.notify(job)

	retryReq, err := http.NewRequestWithContext(ctx, http.MethodPost, computeURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	retryReq.Header.Set("Content-Type", "application/json")
	retryReq.Header.Set("X-Escrow-TaskId", job.TaskId)
	retryReq.Header.Set("X-Escrow-TxHash", job.LockTxHash)
	signatureBody := map[string]any{
		"x402Version": 2,
		"taskId":      job.TaskId,
		"lockTxHash":  job.LockTxHash,
		"agentKey":    c.aevumClient.AgentAddress().Hex(),
		"principal":   c.aevumClient.PrincipalAddress().Hex(),
	}
	if job.Scheme != "" {
		signatureBody["scheme"] = job.Scheme
	}
	signature, err := json.Marshal(signatureBody)
	if err != nil {
		return err
	}
	job.PaymentSignature = base64.StdEncoding.EncodeToString(signature)
	retryReq.Header.Set("PAYMENT-SIGNATURE", job.PaymentSignature)

	retryResp, err := c.httpClient.Do(retryReq)
	if err != nil {
		return fmt.Errorf("second request with escrow proof failed: %w", err)
	}
	defer retryResp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(retryResp.Body, 1<<20))
	if retryResp.StatusCode != http.StatusOK {
		return fmt.Errorf("compute execution failed with status %d: %s", retryResp.StatusCode, string(bodyBytes))
	}

	var computeResp struct {
		Status         string `json:"status"`
		Result         string `json:"result"`
		ResultHash     string `json:"result_hash"`
		ClaimTxHash    string `json:"claim_tx_hash"`
		Ceiling        string `json:"ceiling"`
		ActualAmount   string `json:"actualAmount"`
		RefundedChange string `json:"refundedChange"`
		Breakdown      []struct {
			Prompt string `json:"prompt"`
			Tokens uint64 `json:"tokens"`
			Amount string `json:"amount"`
		} `json:"breakdown"`
		Provenance *types.Provenance `json:"provenance"`
	}
	if err := json.Unmarshal(bodyBytes, &computeResp); err != nil {
		return fmt.Errorf("invalid compute response: %w", err)
	}

	job.PaymentResponse = retryResp.Header.Get("PAYMENT-RESPONSE")
	if computeResp.Status == "SUBMITTED_FOR_EVALUATION" {
		job.Status = "SUBMITTED"
		job.Event = "SUBMITTED_FOR_EVALUATION"
	} else {
		job.Status = "SETTLED"
	}
	job.Result = computeResp.Result
	job.ResultHash = computeResp.ResultHash
	job.ClaimTxHash = computeResp.ClaimTxHash
	job.Ceiling = computeResp.Ceiling
	job.ActualAmount = computeResp.ActualAmount
	job.RefundedChange = computeResp.RefundedChange
	for _, call := range computeResp.Breakdown {
		job.Breakdown = append(job.Breakdown, fmt.Sprintf("%s · %d tokens · %s", call.Prompt, call.Tokens, call.Amount))
	}
	job.Provenance = computeResp.Provenance
	if job.Provenance != nil {
		job.Event = "MULTI_HOP_SETTLED"
	} else if job.ActualAmount != "" {
		job.Event = "TASK_COMPLETED_METERED"
		if calls := len(job.Batch); calls > 1 {
			job.GasSaved = fmt.Sprintf("%d%% fewer locks than %d separate escrows", (calls-1)*100/calls, calls)
		}
	}
	job.SettledAt = time.Now()
	c.notify(job)
	return nil
}

func (c *Client) notify(job *types.TaskJob) {
	if c.Notify != nil {
		c.Notify(job)
	}
}

func (c *Client) reserve(amount *big.Int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if amount.Cmp(c.perTaskCap) > 0 {
		return fmt.Errorf("requested fee %s exceeds local per-task cap %s", amount.String(), c.perTaskCap.String())
	}
	next := new(big.Int).Add(c.spent, amount)
	if next.Cmp(c.hourlyLimit) > 0 {
		return fmt.Errorf("requested fee %s exceeds remaining hourly allowance", amount.String())
	}
	c.spent = next
	return nil
}

func (c *Client) release(amount *big.Int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spent.Sub(c.spent, amount)
	if c.spent.Sign() < 0 {
		c.spent = big.NewInt(0)
	}
}

func canonicalService(job *types.TaskJob) string {
	raw := job.Service
	if raw == "" {
		raw = job.TaskType
	}
	switch raw {
	case "web_brief", "chain_audit", "ai_synthesis", "deep_due_diligence", "micro_call":
		return raw
	default:
		return "ai_synthesis"
	}
}

func parsePaymentHeaders(h http.Header, prefer string) (*types.PaymentChallenge, error) {
	if encoded := h.Get("PAYMENT-REQUIRED"); encoded != "" {
		return parsePaymentRequired(encoded, prefer)
	}
	proto := h.Get("X-Payment-Protocol")
	if proto != "Nexus402-v1" {
		return nil, fmt.Errorf("unsupported protocol: %s", proto)
	}

	tokenHex := h.Get("X-Payment-Token")
	if !common.IsHexAddress(tokenHex) {
		return nil, fmt.Errorf("invalid payment token: %s", tokenHex)
	}

	amountStr := h.Get("X-Payment-Amount")
	amount, ok := new(big.Int).SetString(amountStr, 10)
	if !ok {
		return nil, fmt.Errorf("invalid payment amount: %s", amountStr)
	}

	receiverHex := h.Get("X-Payment-Receiver")
	if !common.IsHexAddress(receiverHex) {
		return nil, fmt.Errorf("invalid receiver address: %s", receiverHex)
	}

	taskHashHex := strings.TrimPrefix(h.Get("X-Task-Hash"), "0x")
	decoded, err := hex.DecodeString(taskHashHex)
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("invalid task hash header")
	}
	var taskHash [32]byte
	copy(taskHash[:], decoded)

	timeout := uint64(50)
	if timeoutStr := h.Get("X-Task-Timeout-Blocks"); timeoutStr != "" {
		parsed, err := strconv.ParseUint(timeoutStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout blocks")
		}
		timeout = parsed
	}

	return &types.PaymentChallenge{
		Protocol:      proto,
		Token:         common.HexToAddress(tokenHex),
		Amount:        amount,
		Receiver:      common.HexToAddress(receiverHex),
		TaskHash:      taskHash,
		TimeoutBlocks: timeout,
	}, nil
}

type paymentAccept struct {
	Scheme        string `json:"scheme"`
	Network       string `json:"network"`
	Asset         string `json:"asset"`
	Amount        string `json:"amount"`
	PayTo         string `json:"payTo"`
	TaskHash      string `json:"taskHash"`
	TimeoutBlocks uint64 `json:"timeoutBlocks"`
}

func chooseAccept(accepts []paymentAccept, prefer string) (paymentAccept, error) {
	if prefer != "" && prefer != "exact" && prefer != "nexus-escrow-v1" {
		for _, accept := range accepts {
			if accept.Scheme == prefer {
				return accept, nil
			}
		}
		return paymentAccept{}, fmt.Errorf("worker did not offer a %s scheme", prefer)
	}
	for _, accept := range accepts {
		if accept.Scheme == "exact" || accept.Scheme == "nexus-escrow-v1" {
			return accept, nil
		}
	}
	if len(accepts) == 0 {
		return paymentAccept{}, fmt.Errorf("worker offered no payment schemes")
	}
	return accepts[0], nil
}

func parsePaymentRequired(encoded string, prefer string) (*types.PaymentChallenge, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid PAYMENT-REQUIRED encoding: %w", err)
	}
	var envelope struct {
		X402Version   int             `json:"x402Version"`
		Accepts       []paymentAccept `json:"accepts"`
		Scheme        string          `json:"scheme"`
		Asset         string          `json:"asset"`
		Amount        string          `json:"amount"`
		PayTo         string          `json:"payTo"`
		TaskHash      string          `json:"taskHash"`
		TimeoutBlocks uint64          `json:"timeoutBlocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("invalid PAYMENT-REQUIRED body: %w", err)
	}
	challenge := struct {
		Scheme        string
		Asset         string
		Amount        string
		PayTo         string
		TaskHash      string
		TimeoutBlocks uint64
	}{envelope.Scheme, envelope.Asset, envelope.Amount, envelope.PayTo, envelope.TaskHash, envelope.TimeoutBlocks}
	if len(envelope.Accepts) > 0 {
		if envelope.X402Version != 2 {
			return nil, fmt.Errorf("unsupported x402Version %d", envelope.X402Version)
		}
		accepted, err := chooseAccept(envelope.Accepts, prefer)
		if err != nil {
			return nil, err
		}
		if accepted.Network != "" && accepted.Network != "eip155:44005" {
			return nil, fmt.Errorf("unsupported network %s", accepted.Network)
		}
		challenge.Scheme = accepted.Scheme
		challenge.Asset = accepted.Asset
		challenge.Amount = accepted.Amount
		challenge.PayTo = accepted.PayTo
		challenge.TaskHash = accepted.TaskHash
		challenge.TimeoutBlocks = accepted.TimeoutBlocks
	}
	switch challenge.Scheme {
	case "exact", "upto", "nexus-escrow-v1", "auth-capture", "batch-settlement":
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", challenge.Scheme)
	}
	if !common.IsHexAddress(challenge.Asset) || !common.IsHexAddress(challenge.PayTo) {
		return nil, fmt.Errorf("invalid payment address")
	}
	amount, ok := new(big.Int).SetString(challenge.Amount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid payment amount")
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(challenge.TaskHash, "0x"))
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("invalid task hash")
	}
	var taskHash [32]byte
	copy(taskHash[:], decoded)
	timeout := challenge.TimeoutBlocks
	if timeout == 0 {
		timeout = 50
	}
	return &types.PaymentChallenge{
		Protocol:      challenge.Scheme,
		Token:         common.HexToAddress(challenge.Asset),
		Amount:        amount,
		Receiver:      common.HexToAddress(challenge.PayTo),
		TaskHash:      taskHash,
		TimeoutBlocks: timeout,
	}, nil
}

func (c *Client) lockEvaluated(ctx context.Context, challenge *types.PaymentChallenge) ([32]byte, string, uint64, error) {
	var taskID [32]byte
	keyHex := strings.TrimSpace(os.Getenv("EVALUATOR_PRIVATE_KEY"))
	if keyHex == "" {
		return taskID, "", 0, fmt.Errorf("EVALUATOR_PRIVATE_KEY is unset")
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(keyHex, "0x"))
	if err != nil {
		return taskID, "", 0, fmt.Errorf("invalid evaluator key: %w", err)
	}
	evaluator := crypto.PubkeyToAddress(key.PublicKey)
	window := challenge.TimeoutBlocks
	if window == 0 {
		window = 30
	}
	return c.aevumClient.LockEvaluatedEscrow(ctx, challenge.Receiver, evaluator, challenge.Amount, challenge.TimeoutBlocks, window, challenge.TaskHash)
}

func (c *Client) settleOneVoucher(ctx context.Context, job *types.TaskJob, challenge *types.PaymentChallenge, reqBody []byte, computeURL string, reserved bool) error {
	release := func() {
		if reserved && challenge.Amount != nil {
			c.release(challenge.Amount)
		}
	}
	tabID, openTx, err := c.aevumClient.OpenSessionTab(ctx, challenge.Receiver, challenge.Amount, challenge.TimeoutBlocks)
	if err != nil {
		release()
		if openTx != "" {
			job.LockTxHash = openTx
		}
		return fmt.Errorf("openSessionTab failed: %w", err)
	}
	job.LockTxHash = openTx
	job.TaskId = "0x" + hex.EncodeToString(tabID[:])
	job.Status = "ESCROW_LOCKED"
	job.Amount = challenge.Amount
	c.notify(job)
	validBefore := uint64(time.Now().Add(time.Hour).Unix())
	signature, err := c.aevumClient.SignVoucher(tabID, challenge.Amount, 1, validBefore)
	if err != nil {
		release()
		return err
	}
	wire, err := json.Marshal(map[string]any{
		"scheme":           "batch-settlement",
		"tabId":            job.TaskId,
		"cumulativeAmount": challenge.Amount.String(),
		"nonce":            1,
		"validBefore":      validBefore,
		"agentSignature":   "0x" + hex.EncodeToString(signature),
		"agentKey":         c.aevumClient.AgentAddress().Hex(),
		"principal":        c.aevumClient.PrincipalAddress().Hex(),
	})
	if err != nil {
		release()
		return err
	}
	job.PaymentSignature = base64.StdEncoding.EncodeToString(wire)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, computeURL, bytes.NewReader(reqBody))
	if err != nil {
		release()
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("PAYMENT-SIGNATURE", job.PaymentSignature)
	request.Header.Set("x-defer-voucher-flush", "1")
	response, err := c.httpClient.Do(request)
	if err != nil {
		release()
		return err
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	job.PaymentResponse = response.Header.Get("PAYMENT-RESPONSE")
	if response.StatusCode != http.StatusOK {
		release()
		return fmt.Errorf("voucher call returned %d: %s", response.StatusCode, string(payload))
	}
	settleURL := strings.TrimSuffix(computeURL, "/api/v1/compute") + "/api/v1/tabs/" + job.TaskId + "/settle"
	settleRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, settleURL, nil)
	if err != nil {
		return err
	}
	settleResponse, err := c.httpClient.Do(settleRequest)
	if err != nil {
		return err
	}
	defer settleResponse.Body.Close()
	var settled struct {
		Settled bool   `json:"settled"`
		TxHash  string `json:"txHash"`
		Error   string `json:"error"`
	}
	_ = json.NewDecoder(settleResponse.Body).Decode(&settled)
	if !settled.Settled || settled.TxHash == "" {
		return fmt.Errorf("batch settlement failed: %s", settled.Error)
	}
	var body struct {
		Result     string `json:"result"`
		ResultHash string `json:"result_hash"`
	}
	_ = json.Unmarshal(payload, &body)
	job.Result = body.Result
	job.ResultHash = body.ResultHash
	job.ClaimTxHash = settled.TxHash
	job.VoucherCalls = 1
	job.Status = "SETTLED"
	job.Event = "VOUCHER_BATCH_SETTLED"
	job.SettledAt = time.Now().UTC()
	c.notify(job)
	return nil
}
