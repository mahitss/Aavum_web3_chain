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
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nexus402/agent-orchestrator/pkg/types"
)

const expectedProtocol = "Nexus402-v1"

// Locker submits the on-chain escrow lock for a 402 challenge.
type Locker interface {
	LockEscrow(worker common.Address, amount *big.Int, durationBlocks uint64, taskHash [32]byte) ([32]byte, string, error)
}

// Budget is the orchestrator's local per-task and rolling hourly cap.
type Budget struct {
	mu          sync.Mutex
	perTask     *big.Int
	hourly      *big.Int
	spent       *big.Int
	windowStart time.Time
}

func NewBudget(perTask, hourly *big.Int) *Budget {
	return &Budget{
		perTask:     new(big.Int).Set(perTask),
		hourly:      new(big.Int).Set(hourly),
		spent:       big.NewInt(0),
		windowStart: time.Now(),
	}
}

func (b *Budget) Reserve(amount *big.Int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Since(b.windowStart) >= time.Hour {
		b.windowStart = time.Now()
		b.spent = big.NewInt(0)
	}
	if amount.Cmp(b.perTask) > 0 {
		return fmt.Errorf("amount %s exceeds per-task limit %s", amount, b.perTask)
	}
	next := new(big.Int).Add(b.spent, amount)
	if next.Cmp(b.hourly) > 0 {
		return fmt.Errorf("amount %s exceeds remaining hourly allowance", amount)
	}
	b.spent = next
	return nil
}

func (b *Budget) Release(amount *big.Int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.spent.Sub(b.spent, amount)
	if b.spent.Sign() < 0 {
		b.spent = big.NewInt(0)
	}
}

func (b *Budget) Snapshot() (spent, hourly *big.Int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Since(b.windowStart) >= time.Hour {
		return big.NewInt(0), new(big.Int).Set(b.hourly)
	}
	return new(big.Int).Set(b.spent), new(big.Int).Set(b.hourly)
}

type computeResponse struct {
	Status      string `json:"status"`
	Result      string `json:"result"`
	TaskID      string `json:"task_id"`
	ClaimTxHash string `json:"claim_tx_hash"`
}

// DispatchTask performs the 402 handshake, locks escrow, and retries with the task id.
func DispatchTask(workerURL, prompt, taskType string, token common.Address, locker Locker, budget *Budget, logf func(string)) (*types.TaskJob, error) {
	started := time.Now()
	payload, err := json.Marshal(map[string]string{
		"prompt":    prompt,
		"task_type": taskType,
	})
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(workerURL, "/") + "/api/v1/compute"
	logf("[REQ] POST /api/v1/compute (No Escrow Header)")

	response, body, err := post(endpoint, payload, nil)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusPaymentRequired {
		return nil, fmt.Errorf("expected HTTP 402, got %d: %s", response.StatusCode, truncate(body))
	}
	challenge, err := ParseChallenge(response.Header, token)
	if err != nil {
		return nil, err
	}
	logf(fmt.Sprintf("[402] CHALLENGE: Amount=%s | TaskHash=0x%x", challenge.Amount.String(), challenge.TaskHash))
	if err := budget.Reserve(challenge.Amount); err != nil {
		return nil, err
	}
	released := false
	defer func() {
		if !released {
			budget.Release(challenge.Amount)
		}
	}()

	logf("[TX] Locking escrow on Aevum Testnet...")
	taskID, lockTx, err := locker.LockEscrow(challenge.Receiver, challenge.Amount, challenge.TimeoutBlocks, challenge.TaskHash)
	if err != nil {
		if lockTx != "" {
			return failedJob(prompt, taskType, started, lockTx, err), err
		}
		return nil, err
	}
	released = true
	logf(fmt.Sprintf("[RETRY] POST /api/v1/compute (X-Escrow-TaskId=0x%x)", taskID))
	headers := map[string]string{
		"X-Escrow-TaskId": "0x" + hex.EncodeToString(taskID[:]),
		"X-Escrow-TxHash": lockTx,
	}
	paid, paidBody, err := post(endpoint, payload, headers)
	if err != nil {
		return failedJob(prompt, taskType, started, lockTx, err), err
	}
	if paid.StatusCode != http.StatusOK {
		err = fmt.Errorf("worker rejected paid retry with %d: %s", paid.StatusCode, truncate(paidBody))
		return failedJob(prompt, taskType, started, lockTx, err), err
	}
	var computed computeResponse
	if err := json.Unmarshal(paidBody, &computed); err != nil {
		return failedJob(prompt, taskType, started, lockTx, err), err
	}
	released = true
	logf("[PROOF] Worker verified on-chain. Released payout. 200 OK.")
	return &types.TaskJob{
		Prompt:      prompt,
		TaskType:    taskType,
		Status:      "SETTLED",
		TaskId:      taskID,
		LockTxHash:  lockTx,
		ClaimTxHash: computed.ClaimTxHash,
		Result:      computed.Result,
		Timestamp:   started.Unix(),
		LatencyMs:   time.Since(started).Milliseconds(),
		Amount:      challenge.Amount.String(),
	}, nil
}

func failedJob(prompt, taskType string, started time.Time, lockTx string, cause error) *types.TaskJob {
	return &types.TaskJob{
		Prompt:     prompt,
		TaskType:   taskType,
		Status:     "FAILED",
		LockTxHash: lockTx,
		Result:     cause.Error(),
		Timestamp:  started.Unix(),
		LatencyMs:  time.Since(started).Milliseconds(),
	}
}

// ParseChallenge reads the Nexus402 payment headers.
func ParseChallenge(header http.Header, expectedToken common.Address) (types.PaymentChallenge, error) {
	var challenge types.PaymentChallenge
	challenge.Protocol = header.Get("X-Payment-Protocol")
	if challenge.Protocol != expectedProtocol {
		return challenge, fmt.Errorf("unexpected protocol %q", challenge.Protocol)
	}
	token := common.HexToAddress(header.Get("X-Payment-Token"))
	if token != expectedToken {
		return challenge, fmt.Errorf("unexpected payment token %s", token.Hex())
	}
	challenge.Token = token
	amount, ok := new(big.Int).SetString(strings.TrimSpace(header.Get("X-Payment-Amount")), 10)
	if !ok || amount.Sign() <= 0 {
		return challenge, fmt.Errorf("invalid payment amount")
	}
	challenge.Amount = amount
	receiver := common.HexToAddress(header.Get("X-Payment-Receiver"))
	if receiver == (common.Address{}) {
		return challenge, fmt.Errorf("invalid payment receiver")
	}
	challenge.Receiver = receiver
	hashBytes, err := decode32(header.Get("X-Task-Hash"))
	if err != nil {
		return challenge, fmt.Errorf("task hash: %w", err)
	}
	challenge.TaskHash = hashBytes
	challenge.TimeoutBlocks = 50
	if raw := strings.TrimSpace(header.Get("X-Task-Timeout-Blocks")); raw != "" {
		timeout, ok := new(big.Int).SetString(raw, 10)
		if !ok {
			return challenge, fmt.Errorf("invalid timeout blocks")
		}
		challenge.TimeoutBlocks = timeout.Uint64()
	}
	return challenge, nil
}

func post(url string, payload []byte, headers map[string]string) (*http.Response, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return response, body, err
}

func decode32(value string) ([32]byte, error) {
	var out [32]byte
	raw := strings.TrimPrefix(strings.TrimSpace(value), "0x")
	decoded, err := hex.DecodeString(raw)
	if err != nil {
		return out, err
	}
	if len(decoded) != 32 {
		return out, fmt.Errorf("want 32 bytes, got %d", len(decoded))
	}
	copy(out[:], decoded)
	return out, nil
}

func truncate(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 240 {
		return text[:240]
	}
	return text
}
