package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// PaymentChallenge is the HTTP 402 offer returned by the worker.
type PaymentChallenge struct {
	Protocol      string
	Token         common.Address
	Amount        *big.Int
	Receiver      common.Address
	TaskHash      [32]byte
	TimeoutBlocks uint64
}

// TaskJob is one orchestrated compute request and its settlement trail.
type TaskJob struct {
	ID          string
	Prompt      string
	TaskType    string
	Status      string
	TaskId      [32]byte
	LockTxHash  string
	ClaimTxHash string
	Result      string
	Timestamp   int64
	LatencyMs   int64
	LockBlock   uint64
	Amount      string
}
