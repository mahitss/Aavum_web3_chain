package evaluator

import (
	"encoding/hex"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestInspectRejectsGarbageWithAMatchingHash(t *testing.T) {
	output := "GARBAGE::{not-json"
	hash := "0x" + hex.EncodeToString(crypto.Keccak256([]byte(output)))
	approved, reason := Inspect("Explain why a capped session key is safer", output, hash)
	if approved || reason != "output is not valid JSON" {
		t.Fatalf("approved=%v reason=%s", approved, reason)
	}
}

func TestInspectAcceptsRelevantJSON(t *testing.T) {
	output := `{"answer":"A capped session key expires and cannot drain the principal."}`
	hash := "0x" + hex.EncodeToString(crypto.Keccak256([]byte(output)))
	approved, reason := Inspect("Explain why a capped session key is safer", output, hash)
	if !approved {
		t.Fatal(reason)
	}
}
