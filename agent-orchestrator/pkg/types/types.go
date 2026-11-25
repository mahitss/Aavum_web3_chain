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
	ID              string    `json:"id"`
	Prompt          string    `json:"prompt"`
	TaskType        string    `json:"task_type"`
	Service         string    `json:"service,omitempty"`
	Status          string    `json:"status"`
	Amount          *big.Int  `json:"amount,omitempty"`
	TaskHash        string    `json:"task_hash,omitempty"`
	TaskId          string    `json:"task_id,omitempty"`
	LockTxHash      string    `json:"lock_tx_hash,omitempty"`
	ClaimTxHash     string    `json:"claim_tx_hash,omitempty"`
	RefundTxHash    string    `json:"refund_tx_hash,omitempty"`
	BlocksRemaining uint64    `json:"blocks_remaining,omitempty"`
	Result          string    `json:"result,omitempty"`
	ResultHash      string    `json:"result_hash,omitempty"`
	Error           string    `json:"error,omitempty"`
	Event           string    `json:"event,omitempty"`
	Deadline        uint64    `json:"deadline,omitempty"`
	SimulateTimeout bool      `json:"simulate_timeout,omitempty"`
	WorkerURL       string    `json:"worker_url,omitempty"`
	BypassLocalCap  bool      `json:"-"`
	CreatedAt       time.Time `json:"created_at"`
	SettledAt       time.Time `json:"settled_at,omitempty"`
}

type TaskRequest struct {
	Prompt   string `json:"prompt" binding:"required"`
	TaskType string `json:"task_type"`
	Service  string `json:"service"`
}

type TelemetryStats struct {
	AgentAddress   string   `json:"agent_address"`
	Principal      string   `json:"principal"`
	EscrowContract string   `json:"escrow_contract"`
	TotalJobs      int      `json:"total_jobs"`
	SettledJobs    int      `json:"settled_jobs"`
	FailedJobs     int      `json:"failed_jobs"`
	HourlySpent    *big.Int `json:"hourly_spent"`
	HourlyLimit    *big.Int `json:"hourly_limit"`
	PerTaskLimit   *big.Int `json:"per_task_limit"`
	ChainID        int64    `json:"chain_id"`
	Network        string   `json:"network"`
}
