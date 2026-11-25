package main

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/nexus402/agent-orchestrator/pkg/types"
)

func TestRenderMetricsCountsLiveJobs(t *testing.T) {
	server := &Server{jobsHistory: []*types.TaskJob{
		{Status: "SETTLED", Amount: big.NewInt(250000), CreatedAt: time.Unix(100, 0), SettledAt: time.Unix(101, 0)},
		{Status: "BLOCKED_HOURLY_CAP"},
		{Status: "REFUNDED", Amount: big.NewInt(150000)},
	}}
	body := renderMetrics(server.metricSnapshot(), "0.050000")
	for _, line := range []string{
		`nexus402_tasks_total{status="settled"} 1`,
		`nexus402_tasks_total{status="refunded"} 1`,
		`nexus402_tasks_total{status="blocked_cap"} 1`,
		"nexus402_tusd8_volume_units 400000",
		"nexus402_settlement_latency_ms 1000",
		"nexus402_wallet_gas_balance_tqday 0.050000",
	} {
		if !strings.Contains(body, line) {
			t.Fatalf("missing %s in %s", line, body)
		}
	}
}

func TestFormatTQDAY(t *testing.T) {
	if got := formatTQDAY("50000000000000000"); got != "0.050000" {
		t.Fatalf("got %s", got)
	}
}
