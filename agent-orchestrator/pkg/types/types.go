package types

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type PaymentChallenge struct {
	Protocol      string         `json:"protocol"`
	Token         common.Address `json:"token"`
	Amount        *big.Int       `json:"amount"`
	Receiver      common.Address `json:"receiver"`
	TaskHash      [32]byte       `json:"task_hash"`
	TimeoutBlocks uint64         `json:"timeout_blocks"`
}

type TaskJob struct {
	ID                  string      `json:"id"`
	Prompt              string      `json:"prompt"`
	TaskType            string      `json:"task_type"`
	Service             string      `json:"service,omitempty"`
	Status              string      `json:"status"`
	Amount              *big.Int    `json:"amount,omitempty"`
	TaskHash            string      `json:"task_hash,omitempty"`
	TaskId              string      `json:"task_id,omitempty"`
	LockTxHash          string      `json:"lock_tx_hash,omitempty"`
	ClaimTxHash         string      `json:"claim_tx_hash,omitempty"`
	RefundTxHash        string      `json:"refund_tx_hash,omitempty"`
	BlocksRemaining     uint64      `json:"blocks_remaining,omitempty"`
	Result              string      `json:"result,omitempty"`
	ResultHash          string      `json:"result_hash,omitempty"`
	Error               string      `json:"error,omitempty"`
	Event               string      `json:"event,omitempty"`
	Deadline            uint64      `json:"deadline,omitempty"`
	SimulateTimeout     bool        `json:"simulate_timeout,omitempty"`
	WorkerURL           string      `json:"worker_url,omitempty"`
	Scheme              string      `json:"scheme,omitempty"`
	Batch               []string    `json:"batch,omitempty"`
	Ceiling             string      `json:"ceiling,omitempty"`
	ActualAmount        string      `json:"actual_amount,omitempty"`
	RefundedChange      string      `json:"refunded_change,omitempty"`
	Breakdown           []string    `json:"breakdown,omitempty"`
	GasSaved            string      `json:"gas_saved,omitempty"`
	Provenance          *Provenance `json:"provenance,omitempty"`
	VoucherCalls        int         `json:"voucher_calls,omitempty"`
	AvgLatencyMs        int64       `json:"avg_latency_ms,omitempty"`
	ReliabilityBps      uint64      `json:"reliability_bps,omitempty"`
	PreviousReliability uint64      `json:"previous_reliability_bps,omitempty"`
	Evaluator           bool        `json:"evaluator,omitempty"`
	PaymentRequired     string      `json:"payment_required,omitempty"`
	PaymentSignature    string      `json:"payment_signature,omitempty"`
	PaymentResponse     string      `json:"payment_response,omitempty"`
	BypassLocalCap      bool        `json:"-"`
	CreatedAt           time.Time   `json:"created_at"`
	SettledAt           time.Time   `json:"settled_at,omitempty"`
}

type SubTask struct {
	Service     string `json:"service"`
	TaskID      string `json:"taskId"`
	LockTxHash  string `json:"lockTxHash"`
	ClaimTxHash string `json:"claimTxHash"`
	Amount      string `json:"amount"`
}

type Provenance struct {
	ParentTaskID string    `json:"parentTaskId"`
	ParentAmount string    `json:"parentAmount"`
	Margin       string    `json:"margin"`
	SubTasks     []SubTask `json:"subTasks"`
}

type TaskRequest struct {
	Prompt    string `json:"prompt" binding:"required"`
	TaskType  string `json:"task_type"`
	Service   string `json:"service"`
	Scheme    string `json:"scheme"`
	Evaluator bool   `json:"evaluator"`
}

type TelemetryStats struct {
	AgentAddress     string   `json:"agent_address"`
	Principal        string   `json:"principal"`
	EscrowContract   string   `json:"escrow_contract"`
	IdentityRegistry string   `json:"identity_registry"`
	TotalJobs        int      `json:"total_jobs"`
	SettledJobs      int      `json:"settled_jobs"`
	FailedJobs       int      `json:"failed_jobs"`
	HourlySpent      *big.Int `json:"hourly_spent"`
	HourlyLimit      *big.Int `json:"hourly_limit"`
	PerTaskLimit     *big.Int `json:"per_task_limit"`
	ChainID          int64    `json:"chain_id"`
	Network          string   `json:"network"`
}
