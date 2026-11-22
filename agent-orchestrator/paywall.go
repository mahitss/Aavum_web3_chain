package main

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const paymentProtocol = "Nexus402-v1"

type Challenge struct {
	Protocol      string
	Token         common.Address
	Amount        *big.Int
	Receiver      common.Address
	TaskHash      common.Hash
	TimeoutBlocks uint64
}

func ParseChallenge(header http.Header, expectedToken common.Address) (Challenge, error) {
	protocol := strings.TrimSpace(header.Get("X-Payment-Protocol"))
	if protocol != paymentProtocol {
		return Challenge{}, fmt.Errorf("unexpected payment protocol %q", protocol)
	}
	token := common.HexToAddress(strings.TrimSpace(header.Get("X-Payment-Token")))
	if token == (common.Address{}) || token != expectedToken {
		return Challenge{}, fmt.Errorf("unexpected settlement token")
	}
	amount, ok := new(big.Int).SetString(strings.TrimSpace(header.Get("X-Payment-Amount")), 10)
	if !ok || amount.Sign() <= 0 {
		return Challenge{}, fmt.Errorf("invalid payment amount")
	}
	receiver := common.HexToAddress(strings.TrimSpace(header.Get("X-Payment-Receiver")))
	if receiver == (common.Address{}) {
		return Challenge{}, fmt.Errorf("invalid payment receiver")
	}
	taskHash, err := parseHash(header.Get("X-Task-Hash"))
	if err != nil {
		return Challenge{}, fmt.Errorf("invalid task hash: %w", err)
	}
	var timeout uint64
	if _, err := fmt.Sscan(strings.TrimSpace(header.Get("X-Task-Timeout-Blocks")), &timeout); err != nil || timeout == 0 {
		return Challenge{}, fmt.Errorf("invalid task timeout")
	}
	return Challenge{
		Protocol:      protocol,
		Token:         token,
		Amount:        amount,
		Receiver:      receiver,
		TaskHash:      taskHash,
		TimeoutBlocks: timeout,
	}, nil
}

func VerifyBodyHash(body []byte, claimed common.Hash) error {
	got := crypto.Keccak256Hash(body)
	if got != claimed {
		return fmt.Errorf("task hash does not match payload")
	}
	return nil
}

func CheckLimits(amount, perTask, hourlyRemaining *big.Int) error {
	if amount.Cmp(perTask) > 0 {
		return fmt.Errorf("fee exceeds per-task session limit")
	}
	if amount.Cmp(hourlyRemaining) > 0 {
		return fmt.Errorf("fee exceeds hourly session limit")
	}
	return nil
}

type Velocity struct {
	mu     sync.Mutex
	window int64
	spent  *big.Int
	limit  *big.Int
}

func NewVelocity(limit *big.Int) *Velocity {
	return &Velocity{spent: big.NewInt(0), limit: new(big.Int).Set(limit), window: time.Now().Unix() / 3600}
}

func (v *Velocity) Remaining() *big.Int {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.roll()
	return new(big.Int).Sub(v.limit, v.spent)
}

func (v *Velocity) Commit(amount *big.Int) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.roll()
	next := new(big.Int).Add(v.spent, amount)
	if next.Cmp(v.limit) > 0 {
		return fmt.Errorf("fee exceeds hourly session limit")
	}
	v.spent = next
	return nil
}

func (v *Velocity) Release(amount *big.Int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.roll()
	v.spent.Sub(v.spent, amount)
	if v.spent.Sign() < 0 {
		v.spent = big.NewInt(0)
	}
}

func (v *Velocity) roll() {
	window := time.Now().Unix() / 3600
	if window != v.window {
		v.window = window
		v.spent = big.NewInt(0)
	}
}

func parseHash(raw string) (common.Hash, error) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "0x")
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != 32 {
		return common.Hash{}, fmt.Errorf("expected 32-byte hex")
	}
	return common.BytesToHash(decoded), nil
}
