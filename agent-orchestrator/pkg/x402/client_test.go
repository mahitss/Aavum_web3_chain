package x402

import (
	"math/big"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestParseChallenge(t *testing.T) {
	token := common.HexToAddress("0x456569838f1d43009A91ebA4263147Afd7233483")
	header := make(http.Header)
	header.Set("X-Payment-Protocol", "Nexus402-v1")
	header.Set("X-Payment-Token", token.Hex())
	header.Set("X-Payment-Amount", "250000")
	header.Set("X-Payment-Receiver", "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266")
	header.Set("X-Task-Hash", "0x0101010101010101010101010101010101010101010101010101010101010101")
	header.Set("X-Task-Timeout-Blocks", "50")

	challenge, err := ParseChallenge(header, token)
	if err != nil {
		t.Fatal(err)
	}
	if challenge.Amount.Cmp(big.NewInt(250000)) != 0 {
		t.Fatalf("amount %s", challenge.Amount)
	}
	if challenge.TimeoutBlocks != 50 {
		t.Fatalf("timeout %d", challenge.TimeoutBlocks)
	}
	if challenge.TaskHash[0] != 0x01 || challenge.TaskHash[31] != 0x01 {
		t.Fatal("task hash mismatch")
	}
}

func TestBudgetRejectsPerTaskOverflow(t *testing.T) {
	budget := NewBudget(big.NewInt(1000), big.NewInt(5000))
	if err := budget.Reserve(big.NewInt(1001)); err == nil {
		t.Fatal("expected per-task rejection")
	}
}
