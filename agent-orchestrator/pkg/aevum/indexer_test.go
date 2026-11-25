package aevum

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func TestParseLedgerLogs(t *testing.T) {
	parsed, err := abi.JSON(strings.NewReader(ledgerABI))
	if err != nil {
		t.Fatal(err)
	}
	principal := common.HexToAddress("0x1000000000000000000000000000000000000001")
	agent := common.HexToAddress("0x2000000000000000000000000000000000000002")
	worker := common.HexToAddress("0x3000000000000000000000000000000000000003")
	taskID := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")
	txHash := common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222")

	lockedData, err := parsed.Events["TaskEscrowLocked"].Inputs.NonIndexed().Pack(big.NewInt(150000), big.NewInt(40))
	if err != nil {
		t.Fatal(err)
	}
	locked := parseMust(t, parsed, types.Log{
		Topics:      []common.Hash{parsed.Events["TaskEscrowLocked"].ID, taskID, common.BytesToHash(principal.Bytes()), common.BytesToHash(worker.Bytes())},
		Data:        lockedData,
		BlockNumber: 12,
		Index:       1,
		TxHash:      txHash,
	})
	if locked.Event != "TaskEscrowLocked" || locked.Amount != "150000" || locked.Principal != principal.Hex() {
		t.Fatalf("locked log parsed as %+v", locked)
	}
	if locked.ExplorerURL != explorerTxBase+txHash.Hex() {
		t.Fatalf("explorer url %s", locked.ExplorerURL)
	}

	refundData, err := parsed.Events["TaskRefunded"].Inputs.NonIndexed().Pack(big.NewInt(150000))
	if err != nil {
		t.Fatal(err)
	}
	refunded := parseMust(t, parsed, types.Log{
		Topics: []common.Hash{parsed.Events["TaskRefunded"].ID, taskID, common.BytesToHash(principal.Bytes())},
		Data:   refundData,
		TxHash: txHash,
	})
	if refunded.Event != "TaskRefunded" || refunded.Amount != "150000" {
		t.Fatalf("refund log parsed as %+v", refunded)
	}

	revokeData, err := parsed.Events["SessionRevoked"].Inputs.NonIndexed().Pack("emergency")
	if err != nil {
		t.Fatal(err)
	}
	revoked := parseMust(t, parsed, types.Log{
		Topics: []common.Hash{parsed.Events["SessionRevoked"].ID, common.BytesToHash(principal.Bytes()), common.BytesToHash(agent.Bytes())},
		Data:   revokeData,
		TxHash: txHash,
	})
	if revoked.Event != "SessionRevoked" || revoked.Reason != "emergency" || revoked.AgentKey != agent.Hex() {
		t.Fatalf("revoke log parsed as %+v", revoked)
	}
}

func TestHydrateLedgerReadsAevum(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ec, err := ethclient.Dial("https://rpc-test.qday.info")
	if err != nil {
		t.Fatal(err)
	}
	defer ec.Close()
	client := &Client{ec: ec, escrowAddress: common.HexToAddress("0x0000000000000000000000000000000000000001")}
	rows, err := client.HydrateLedger(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rows == nil {
		t.Fatal("nil ledger")
	}
}

func parseMust(t *testing.T, parsed abi.ABI, entry types.Log) ChainLedgerEntry {
	t.Helper()
	row, ok := parseLedgerLog(parsed, entry, 1)
	if !ok {
		t.Fatalf("log %s did not parse", entry.Topics[0].Hex())
	}
	return row
}
