package x402

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nexus402/agent-orchestrator/pkg/aevum"
	"github.com/nexus402/agent-orchestrator/pkg/types"
)

type Client struct {
	httpClient  *http.Client
	aevumClient *aevum.Client
	workerURL   string
	hourlyLimit *big.Int
	perTaskCap  *big.Int

	mu    sync.Mutex
	spent *big.Int
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
	reqBody, err := json.Marshal(map[string]string{
		"prompt":    job.Prompt,
		"task_type": job.TaskType,
	})
	if err != nil {
		return fmt.Errorf("failed to serialize payload: %w", err)
	}

	computeURL := c.workerURL + "/api/v1/compute"
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

	job.Status = "402_CHALLENGED"
	challenge, err := parsePaymentHeaders(resp.Header)
	if err != nil {
		return fmt.Errorf("invalid 402 challenge headers: %w", err)
	}
	job.Amount = challenge.Amount
	job.TaskHash = "0x" + hex.EncodeToString(challenge.TaskHash[:])

	if err := c.reserve(challenge.Amount); err != nil {
		return err
	}

	taskID, lockTxHash, err := c.aevumClient.LockEscrow(ctx, challenge.Receiver, challenge.Amount, challenge.TimeoutBlocks, challenge.TaskHash)
	if err != nil {
		c.release(challenge.Amount)
		if lockTxHash != "" {
			job.LockTxHash = lockTxHash
		}
		return fmt.Errorf("on-chain escrow lock failed: %w", err)
	}

	job.Status = "ESCROW_LOCKED"
	job.TaskId = "0x" + hex.EncodeToString(taskID[:])
	job.LockTxHash = lockTxHash

	retryReq, err := http.NewRequestWithContext(ctx, http.MethodPost, computeURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	retryReq.Header.Set("Content-Type", "application/json")
	retryReq.Header.Set("X-Escrow-TaskId", job.TaskId)
	retryReq.Header.Set("X-Escrow-TxHash", job.LockTxHash)

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
		Status      string `json:"status"`
		Result      string `json:"result"`
		ClaimTxHash string `json:"claim_tx_hash"`
	}
	if err := json.Unmarshal(bodyBytes, &computeResp); err != nil {
		return fmt.Errorf("invalid compute response: %w", err)
	}

	job.Status = "SETTLED"
	job.Result = computeResp.Result
	job.ClaimTxHash = computeResp.ClaimTxHash
	job.SettledAt = time.Now()
	return nil
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

func parsePaymentHeaders(h http.Header) (*types.PaymentChallenge, error) {
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
