package main

import (
	"encoding/csv"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/nexus402/agent-orchestrator/pkg/aevum"
	"github.com/nexus402/agent-orchestrator/pkg/types"
)

type exportRow struct {
	Timestamp   string `json:"timestamp"`
	BlockNumber string `json:"block_number"`
	EventType   string `json:"event_type"`
	TaskID      string `json:"task_id"`
	Principal   string `json:"principal"`
	AgentKey    string `json:"agent_key"`
	Worker      string `json:"worker"`
	Scheme      string `json:"scheme"`
	Gross       string `json:"gross_tusd8"`
	Net         string `json:"net_tusd8"`
	Refunded    string `json:"refunded_tusd8"`
	TxHash      string `json:"tx_hash"`
	ExplorerURL string `json:"explorer_url"`
}

func (s *Server) exportRows() []exportRow {
	rows := make([]exportRow, 0)
	seen := map[string]bool{}
	for _, entry := range s.chainLedger {
		row, ok := rowFromChain(entry)
		if !ok || seen[row.TxHash+row.EventType] {
			continue
		}
		seen[row.TxHash+row.EventType] = true
		rows = append(rows, row)
	}
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()
	for _, job := range s.jobsHistory {
		row, ok := rowFromJob(job)
		if !ok || seen[row.TxHash+row.EventType] {
			continue
		}
		seen[row.TxHash+row.EventType] = true
		rows = append(rows, row)
	}
	return rows
}

func rowFromChain(entry aevum.ChainLedgerEntry) (exportRow, bool) {
	row := exportRow{
		Timestamp:   time.Unix(int64(entry.Timestamp), 0).UTC().Format(time.RFC3339),
		BlockNumber: strconv.FormatUint(entry.BlockNumber, 10),
		TaskID:      entry.TaskID,
		Principal:   entry.Principal,
		AgentKey:    entry.AgentKey,
		Worker:      entry.Worker,
		TxHash:      entry.TxHash,
		ExplorerURL: entry.ExplorerURL,
	}
	switch entry.Event {
	case "TaskCompleted", "TaskCompletedMetered":
		row.EventType = "SETTLED"
		row.Gross = formatTUSD(entry.Amount)
		row.Net = row.Gross
		row.Refunded = "0.000000"
		row.Scheme = "exact"
		if entry.Event == "TaskCompletedMetered" {
			row.Scheme = "upto"
		}
	case "TaskRefunded", "TaskRejected":
		row.EventType = "REFUNDED"
		if entry.Event == "TaskRejected" {
			row.EventType = "REJECTED"
		}
		row.Gross = formatTUSD(entry.Amount)
		row.Net = "0.000000"
		row.Refunded = row.Gross
	default:
		return exportRow{}, false
	}
	return row, row.TxHash != ""
}

func rowFromJob(job *types.TaskJob) (exportRow, bool) {
	if job == nil {
		return exportRow{}, false
	}
	event := ""
	tx := ""
	gross := ""
	net := ""
	refunded := "0.000000"
	switch {
	case job.Event == "EVALUATOR_REJECTED":
		event = "REJECTED"
		tx = job.RefundTxHash
		gross = formatTUSD(amountOf(job))
		net = "0.000000"
		refunded = gross
	case job.Status == "REFUNDED":
		event = "REFUNDED"
		tx = firstHash(job.RefundTxHash, job.LockTxHash)
		gross = formatTUSD(amountOf(job))
		net = "0.000000"
		refunded = gross
	case job.Status == "SETTLED":
		event = "SETTLED"
		tx = job.ClaimTxHash
		gross = formatTUSD(amountOf(job))
		if job.ActualAmount != "" {
			net = formatTUSD(job.ActualAmount)
			refunded = formatTUSD(job.RefundedChange)
		} else {
			net = gross
		}
	default:
		return exportRow{}, false
	}
	if tx == "" {
		return exportRow{}, false
	}
	when := job.SettledAt
	if when.IsZero() {
		when = job.CreatedAt
	}
	return exportRow{
		Timestamp:   when.UTC().Format(time.RFC3339),
		BlockNumber: "",
		EventType:   event,
		TaskID:      job.TaskId,
		Scheme:      job.Scheme,
		Gross:       gross,
		Net:         net,
		Refunded:    refunded,
		TxHash:      tx,
		ExplorerURL: "https://explorer-test.qday.info/tx/" + strings.TrimPrefix(tx, "0x"),
	}, true
}

func amountOf(job *types.TaskJob) string {
	if job.Amount != nil {
		return job.Amount.String()
	}
	if job.ActualAmount != "" {
		return job.ActualAmount
	}
	return ""
}

func formatTUSD(units string) string {
	units = strings.TrimSpace(units)
	if units == "" {
		return ""
	}
	value, ok := new(big.Int).SetString(units, 10)
	if !ok {
		return units
	}
	negative := value.Sign() < 0
	if negative {
		value.Abs(value)
	}
	whole := new(big.Int).Quo(value, big.NewInt(1_000_000))
	frac := new(big.Int).Mod(value, big.NewInt(1_000_000))
	text := fmt.Sprintf("%s.%06d", whole.String(), frac.Int64())
	if negative {
		return "-" + text
	}
	return text
}

func firstHash(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func encodeCSV(rows []exportRow) (string, error) {
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	writer.UseCRLF = true
	header := []string{
		"timestamp", "block_number", "event_type", "task_id", "principal", "agent_key", "worker",
		"scheme", "gross_tusd8", "net_tusd8", "refunded_tusd8", "tx_hash", "explorer_url",
	}
	if err := writer.Write(header); err != nil {
		return "", err
	}
	for _, row := range rows {
		record := []string{
			row.Timestamp, row.BlockNumber, row.EventType, row.TaskID, row.Principal, row.AgentKey, row.Worker,
			row.Scheme, row.Gross, row.Net, row.Refunded, row.TxHash, row.ExplorerURL,
		}
		if err := writer.Write(record); err != nil {
			return "", err
		}
	}
	writer.Flush()
	return builder.String(), writer.Error()
}
