package main

import (
	"strings"
	"testing"
	"time"

	"github.com/nexus402/agent-orchestrator/pkg/aevum"
	"github.com/nexus402/agent-orchestrator/pkg/types"
)

func TestExportCSVQuotesAndFormatsTUSD(t *testing.T) {
	server := &Server{
		chainLedger: []aevum.ChainLedgerEntry{{
			Event:       "TaskCompleted",
			BlockNumber: 12,
			Timestamp:   uint64(time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC).Unix()),
			TxHash:      "0xabc",
			Amount:      "300000",
			TaskID:      "0xtask",
			Principal:   "0xprincipal",
			Worker:      "0xworker",
			ExplorerURL: "https://explorer-test.qday.info/tx/abc",
		}},
		jobsHistory: []*types.TaskJob{{
			Status:      "SETTLED",
			Scheme:      "exact",
			TaskId:      "0xjob",
			ClaimTxHash: "0xdef",
			SettledAt:   time.Date(2026, 11, 26, 1, 0, 0, 0, time.UTC),
		}},
	}
	text, err := encodeCSV(server.exportRows())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "timestamp,block_number,event_type,task_id,principal,agent_key,worker,scheme,gross_tusd8,net_tusd8,refunded_tusd8,tx_hash,explorer_url\r\n") {
		t.Fatalf("header = %q", text)
	}
	if !strings.Contains(text, "0.300000") || !strings.Contains(text, "SETTLED") {
		t.Fatalf("rows = %s", text)
	}
}
