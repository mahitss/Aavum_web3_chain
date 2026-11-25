package aevum

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	ledgerLookback = 10_000
	ledgerChunk    = 2_000
	explorerTxBase = "https://explorer-test.qday.info/tx/"
)

const ledgerABI = `[
	{"type":"event","name":"SessionAuthorized","inputs":[
		{"name":"principal","type":"address","indexed":true},
		{"name":"agentKey","type":"address","indexed":true},
		{"name":"hourlyLimit","type":"uint256","indexed":false},
		{"name":"perTaskLimit","type":"uint256","indexed":false},
		{"name":"expiresAt","type":"uint256","indexed":false}
	]},
	{"type":"event","name":"SessionRevoked","inputs":[
		{"name":"principal","type":"address","indexed":true},
		{"name":"agentKey","type":"address","indexed":true},
		{"name":"reason","type":"string","indexed":false}
	]},
	{"type":"event","name":"TaskEscrowLocked","inputs":[
		{"name":"taskId","type":"bytes32","indexed":true},
		{"name":"principal","type":"address","indexed":true},
		{"name":"worker","type":"address","indexed":true},
		{"name":"amount","type":"uint256","indexed":false},
		{"name":"deadline","type":"uint256","indexed":false}
	]},
	{"type":"event","name":"TaskCompleted","inputs":[
		{"name":"taskId","type":"bytes32","indexed":true},
		{"name":"worker","type":"address","indexed":true},
		{"name":"resultHash","type":"bytes32","indexed":false},
		{"name":"amount","type":"uint256","indexed":false}
	]},
	{"type":"event","name":"TaskRefunded","inputs":[
		{"name":"taskId","type":"bytes32","indexed":true},
		{"name":"principal","type":"address","indexed":true},
		{"name":"amount","type":"uint256","indexed":false}
	]}
]`

type ChainLedgerEntry struct {
	Event       string `json:"event"`
	BlockNumber uint64 `json:"blockNumber"`
	LogIndex    uint   `json:"logIndex"`
	Timestamp   uint64 `json:"timestamp"`
	TxHash      string `json:"txHash"`
	Amount      string `json:"amount"`
	ExplorerURL string `json:"explorerUrl"`
	TaskID      string `json:"taskId,omitempty"`
	Principal   string `json:"principal,omitempty"`
	AgentKey    string `json:"agentKey,omitempty"`
	Worker      string `json:"worker,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

func (c *Client) HydrateLedger(ctx context.Context) ([]ChainLedgerEntry, error) {
	if c.escrowAddress == (common.Address{}) {
		return []ChainLedgerEntry{}, nil
	}
	parsed, err := abi.JSON(strings.NewReader(ledgerABI))
	if err != nil {
		return nil, err
	}
	head, err := c.ec.BlockNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("block height: %w", err)
	}
	start := uint64(0)
	if head > ledgerLookback {
		start = head - ledgerLookback
	}
	topics := []common.Hash{
		parsed.Events["SessionAuthorized"].ID,
		parsed.Events["SessionRevoked"].ID,
		parsed.Events["TaskEscrowLocked"].ID,
		parsed.Events["TaskCompleted"].ID,
		parsed.Events["TaskRefunded"].ID,
	}
	var logs []types.Log
	for from := start; from <= head; {
		to := from + ledgerChunk - 1
		if to > head {
			to = head
		}
		chunk, err := c.filterChunk(ctx, from, to, topics)
		if err != nil {
			return nil, fmt.Errorf("filter logs %d-%d: %w", from, to, err)
		}
		logs = append(logs, chunk...)
		if to == head {
			break
		}
		from = to + 1
	}
	stamps := map[uint64]uint64{}
	entries := make([]ChainLedgerEntry, 0, len(logs))
	for _, entry := range logs {
		stamp, ok := stamps[entry.BlockNumber]
		if !ok {
			header, err := c.ec.HeaderByNumber(ctx, new(big.Int).SetUint64(entry.BlockNumber))
			if err != nil {
				return nil, fmt.Errorf("block %d header: %w", entry.BlockNumber, err)
			}
			stamp = header.Time
			stamps[entry.BlockNumber] = stamp
		}
		parsedEntry, ok := parseLedgerLog(parsed, entry, stamp)
		if ok {
			entries = append(entries, parsedEntry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].BlockNumber == entries[j].BlockNumber {
			return entries[i].LogIndex < entries[j].LogIndex
		}
		return entries[i].BlockNumber < entries[j].BlockNumber
	})
	return entries, nil
}

func (c *Client) filterChunk(ctx context.Context, from uint64, to uint64, topics []common.Hash) ([]types.Log, error) {
	logs, err := c.ec.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(from),
		ToBlock:   new(big.Int).SetUint64(to),
		Addresses: []common.Address{c.escrowAddress},
		Topics:    [][]common.Hash{topics},
	})
	if err != nil && to > from && strings.Contains(strings.ToLower(err.Error()), "block range too large") {
		mid := from + (to-from)/2
		left, leftErr := c.filterChunk(ctx, from, mid, topics)
		if leftErr != nil {
			return nil, leftErr
		}
		right, rightErr := c.filterChunk(ctx, mid+1, to, topics)
		if rightErr != nil {
			return nil, rightErr
		}
		return append(left, right...), nil
	}
	return logs, err
}

func parseLedgerLog(parsed abi.ABI, entry types.Log, timestamp uint64) (ChainLedgerEntry, bool) {
	if len(entry.Topics) == 0 {
		return ChainLedgerEntry{}, false
	}
	row := ChainLedgerEntry{
		BlockNumber: entry.BlockNumber,
		LogIndex:    entry.Index,
		Timestamp:   timestamp,
		TxHash:      entry.TxHash.Hex(),
		Amount:      "0",
		ExplorerURL: explorerTxBase + entry.TxHash.Hex(),
	}
	switch entry.Topics[0] {
	case parsed.Events["SessionAuthorized"].ID:
		values, err := parsed.Events["SessionAuthorized"].Inputs.NonIndexed().Unpack(entry.Data)
		if err != nil || len(values) < 3 || len(entry.Topics) < 3 {
			return ChainLedgerEntry{}, false
		}
		perTask, _ := values[1].(*big.Int)
		row.Event = "SessionAuthorized"
		row.Principal = topicAddress(entry.Topics[1])
		row.AgentKey = topicAddress(entry.Topics[2])
		row.Amount = bigString(perTask)
		return row, true
	case parsed.Events["SessionRevoked"].ID:
		values, err := parsed.Events["SessionRevoked"].Inputs.NonIndexed().Unpack(entry.Data)
		if err != nil || len(entry.Topics) < 3 {
			return ChainLedgerEntry{}, false
		}
		row.Event = "SessionRevoked"
		row.Principal = topicAddress(entry.Topics[1])
		row.AgentKey = topicAddress(entry.Topics[2])
		row.Amount = "0"
		if len(values) > 0 {
			reason, _ := values[0].(string)
			row.Reason = reason
		}
		return row, true
	case parsed.Events["TaskEscrowLocked"].ID:
		values, err := parsed.Events["TaskEscrowLocked"].Inputs.NonIndexed().Unpack(entry.Data)
		if err != nil || len(values) < 1 || len(entry.Topics) < 4 {
			return ChainLedgerEntry{}, false
		}
		row.Event = "TaskEscrowLocked"
		row.TaskID = entry.Topics[1].Hex()
		row.Principal = topicAddress(entry.Topics[2])
		row.Worker = topicAddress(entry.Topics[3])
		row.Amount = bigString(asBig(values[0]))
		return row, true
	case parsed.Events["TaskCompleted"].ID:
		values, err := parsed.Events["TaskCompleted"].Inputs.NonIndexed().Unpack(entry.Data)
		if err != nil || len(values) < 2 || len(entry.Topics) < 3 {
			return ChainLedgerEntry{}, false
		}
		row.Event = "TaskCompleted"
		row.TaskID = entry.Topics[1].Hex()
		row.Worker = topicAddress(entry.Topics[2])
		row.Amount = bigString(asBig(values[1]))
		return row, true
	case parsed.Events["TaskRefunded"].ID:
		values, err := parsed.Events["TaskRefunded"].Inputs.NonIndexed().Unpack(entry.Data)
		if err != nil || len(values) < 1 || len(entry.Topics) < 3 {
			return ChainLedgerEntry{}, false
		}
		row.Event = "TaskRefunded"
		row.TaskID = entry.Topics[1].Hex()
		row.Principal = topicAddress(entry.Topics[2])
		row.Amount = bigString(asBig(values[0]))
		return row, true
	default:
		return ChainLedgerEntry{}, false
	}
}

func topicAddress(topic common.Hash) string {
	return common.BytesToAddress(topic.Bytes()).Hex()
}

func asBig(value interface{}) *big.Int {
	amount, _ := value.(*big.Int)
	return amount
}

func bigString(value *big.Int) string {
	if value == nil {
		return "0"
	}
	return value.String()
}
