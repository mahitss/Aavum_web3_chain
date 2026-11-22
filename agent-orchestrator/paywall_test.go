package main

import (
	"math/big"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestParseChallengeAndLimits(t *testing.T) {
	token := common.HexToAddress("0x456569838f1d43009A91ebA4263147Afd7233483")
	body := []byte(`{"job":"keccak256"}`)
	hash := crypto.Keccak256Hash(body)

	header := http.Header{}
	header.Set("X-Payment-Protocol", paymentProtocol)
	header.Set("X-Payment-Token", token.Hex())
	header.Set("X-Payment-Amount", "250000")
	header.Set("X-Payment-Receiver", "0x2222222222222222222222222222222222222222")
	header.Set("X-Task-Hash", hash.Hex())
	header.Set("X-Task-Timeout-Blocks", "50")

	challenge, err := ParseChallenge(header, token)
	if err != nil {
		t.Fatal(err)
	}
	if challenge.Amount.Cmp(big.NewInt(250000)) != 0 {
		t.Fatalf("amount = %s", challenge.Amount)
	}
	if err := VerifyBodyHash(body, challenge.TaskHash); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBodyHash([]byte("other"), challenge.TaskHash); err == nil {
		t.Fatal("expected hash mismatch")
	}
	if err := CheckLimits(challenge.Amount, big.NewInt(250000), big.NewInt(250000)); err != nil {
		t.Fatal(err)
	}
	if err := CheckLimits(challenge.Amount, big.NewInt(249999), big.NewInt(1_000_000)); err == nil {
		t.Fatal("expected per-task rejection")
	}

	header.Set("X-Payment-Token", "0x1111111111111111111111111111111111111111")
	if _, err := ParseChallenge(header, token); err == nil {
		t.Fatal("expected token rejection")
	}
}

func TestVelocityWindow(t *testing.T) {
	velocity := NewVelocity(big.NewInt(300000))
	if err := velocity.Commit(big.NewInt(250000)); err != nil {
		t.Fatal(err)
	}
	if err := velocity.Commit(big.NewInt(250000)); err == nil {
		t.Fatal("expected hourly rejection")
	}
	velocity.Release(big.NewInt(250000))
	if err := velocity.Commit(big.NewInt(250000)); err != nil {
		t.Fatal(err)
	}
}
