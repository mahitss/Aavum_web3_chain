package x402

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestEvaluatePendingExactTask(t *testing.T) {
	worker := common.HexToAddress("0x00000000000000000000000000000000000000b0")
	var hash [32]byte
	hash[0] = 7
	ok, reason := Evaluate(Task{
		Status:   1,
		Worker:   worker,
		Amount:   big.NewInt(600_000),
		TaskHash: hash,
	}, Requirement{
		Scheme:   "exact",
		Network:  Network,
		Asset:    TUSD8,
		Amount:   big.NewInt(600_000),
		PayTo:    worker.Hex(),
		TaskHash: hash,
	})
	if !ok || reason != "" {
		t.Fatalf("expected valid task, got %v %s", ok, reason)
	}
}

func TestEvaluateRejectsUnderfundedAndWrongWorker(t *testing.T) {
	var hash [32]byte
	ok, reason := Evaluate(Task{
		Status:   1,
		Worker:   common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Amount:   big.NewInt(10),
		TaskHash: hash,
	}, Requirement{
		Scheme:   "upto",
		Network:  Network,
		Asset:    TUSD8,
		Amount:   big.NewInt(500_000),
		PayTo:    "0x2222222222222222222222222222222222222222",
		TaskHash: hash,
	})
	if ok || reason != "worker mismatch" {
		t.Fatalf("expected worker mismatch, got %v %s", ok, reason)
	}
}
