package main

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nexus402/agent-orchestrator/pkg/types"
)

type metricSnapshot struct {
	settled       int
	refunded      int
	failed        int
	blockedCap    int
	blockedRevoke int
	volume        *big.Int
	latencyMs     int64
}

func (s *Server) handleMetrics(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
	defer cancel()
	gas := "0.000000"
	if balances, err := s.aevumClient.SandboxBalances(ctx); err == nil {
		gas = formatTQDAY(balances.AgentGas)
	}
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(renderMetrics(s.metricSnapshot(), gas)))
}

func (s *Server) metricSnapshot() metricSnapshot {
	seen := map[string]struct{}{}
	snap := metricSnapshot{volume: big.NewInt(0)}
	for _, event := range s.chainLedger {
		switch event.Event {
		case "TaskCompleted":
			snap.settled++
			addVolume(snap.volume, event.Amount)
			seen[event.TxHash] = struct{}{}
		case "TaskRefunded":
			snap.refunded++
			addVolume(snap.volume, event.Amount)
			seen[event.TxHash] = struct{}{}
		}
	}
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()
	var latest time.Time
	for _, job := range s.jobsHistory {
		if job == nil || alreadyCounted(job, seen) {
			continue
		}
		switch job.Status {
		case "SETTLED":
			snap.settled++
			addVolume(snap.volume, amountString(job))
			if !job.SettledAt.IsZero() && job.SettledAt.After(latest) {
				latest = job.SettledAt
				snap.latencyMs = job.SettledAt.Sub(job.CreatedAt).Milliseconds()
			}
		case "REFUNDED":
			snap.refunded++
			addVolume(snap.volume, amountString(job))
		case "FAILED", "AWAITING_REFUND":
			snap.failed++
		case "BLOCKED_HOURLY_CAP":
			snap.blockedCap++
		case "BLOCKED_REVOKED":
			snap.blockedRevoke++
		}
	}
	return snap
}

func alreadyCounted(job *types.TaskJob, seen map[string]struct{}) bool {
	for _, hash := range []string{job.ClaimTxHash, job.RefundTxHash, job.LockTxHash} {
		if hash != "" {
			if _, ok := seen[hash]; ok {
				return true
			}
		}
	}
	return false
}

func addVolume(total *big.Int, amount string) {
	if parsed, ok := new(big.Int).SetString(amount, 10); ok {
		total.Add(total, parsed)
	}
}

func amountString(job *types.TaskJob) string {
	if job.Amount == nil {
		return "0"
	}
	return job.Amount.String()
}

func renderMetrics(snap metricSnapshot, gas string) string {
	return fmt.Sprintf(`# HELP nexus402_tasks_total Tasks observed by this process.
# TYPE nexus402_tasks_total counter
nexus402_tasks_total{status="settled"} %d
nexus402_tasks_total{status="refunded"} %d
nexus402_tasks_total{status="failed"} %d
nexus402_tasks_total{status="blocked_cap"} %d
nexus402_tasks_total{status="blocked_revoked"} %d
# HELP nexus402_tusd8_volume_units Settled and refunded tUSD8 base units.
# TYPE nexus402_tusd8_volume_units counter
nexus402_tusd8_volume_units %s
# HELP nexus402_settlement_latency_ms Most recent in-process settlement latency.
# TYPE nexus402_settlement_latency_ms gauge
nexus402_settlement_latency_ms %d
# HELP nexus402_wallet_gas_balance_tqday Orchestrator agent tQDAY balance.
# TYPE nexus402_wallet_gas_balance_tqday gauge
nexus402_wallet_gas_balance_tqday %s
`, snap.settled, snap.refunded, snap.failed, snap.blockedCap, snap.blockedRevoke, snap.volume.String(), snap.latencyMs, gas)
}

func formatTQDAY(wei string) string {
	value, ok := new(big.Int).SetString(wei, 10)
	if !ok {
		return "0.000000"
	}
	denom := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	whole := new(big.Int).Quo(value, denom)
	frac := new(big.Int).Mod(value, denom)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(12), nil)
	micros := new(big.Int).Quo(frac, scale)
	return fmt.Sprintf("%s.%06d", whole.String(), micros.Int64())
}
