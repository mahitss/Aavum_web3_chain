package x402

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

const TUSD8 = "0x456569838f1d43009A91ebA4263147Afd7233483"
const Network = "eip155:44005"

type Task struct {
	Status   uint8
	Worker   common.Address
	Amount   *big.Int
	TaskHash [32]byte
}

type Requirement struct {
	Scheme   string
	Network  string
	Asset    string
	Amount   *big.Int
	PayTo    string
	TaskHash [32]byte
}

func Evaluate(task Task, required Requirement) (bool, string) {
	switch required.Scheme {
	case "exact", "upto", "nexus-escrow-v1", "":
	default:
		return false, "unsupported scheme"
	}
	if required.Network != "" && required.Network != Network {
		return false, "unsupported network"
	}
	if required.Asset != "" && !strings.EqualFold(required.Asset, TUSD8) {
		return false, "unsupported asset"
	}
	if task.Status != 1 {
		return false, "task is not pending"
	}
	if !strings.EqualFold(task.Worker.Hex(), required.PayTo) {
		return false, "worker mismatch"
	}
	if task.TaskHash != required.TaskHash {
		return false, "task hash mismatch"
	}
	if required.Amount != nil && task.Amount.Cmp(required.Amount) < 0 {
		return false, "escrow amount is below the requirement"
	}
	return true, ""
}
