package x402

import (
	"encoding/base64"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestParsePaymentHeaders(t *testing.T) {
	token := common.HexToAddress("0x456569838f1d43009A91ebA4263147Afd7233483")
	header := make(http.Header)
	header.Set("X-Payment-Protocol", "Nexus402-v1")
	header.Set("X-Payment-Token", token.Hex())
	header.Set("X-Payment-Amount", "250000")
	header.Set("X-Payment-Receiver", "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266")
	header.Set("X-Task-Hash", "0x0101010101010101010101010101010101010101010101010101010101010101")
	header.Set("X-Task-Timeout-Blocks", "50")

	challenge, err := parsePaymentHeaders(header)
	if err != nil {
		t.Fatal(err)
	}
	if challenge.Protocol != "Nexus402-v1" {
		t.Fatalf("protocol %s", challenge.Protocol)
	}
	if challenge.Amount.Cmp(big.NewInt(250000)) != 0 {
		t.Fatalf("amount %s", challenge.Amount)
	}
	if challenge.TimeoutBlocks != 50 {
		t.Fatalf("timeout %d", challenge.TimeoutBlocks)
	}
	if challenge.Token != token {
		t.Fatalf("token %s", challenge.Token.Hex())
	}
	if challenge.TaskHash[0] != 0x01 || challenge.TaskHash[31] != 0x01 {
		t.Fatal("task hash mismatch")
	}
}

func TestParsePaymentRequiredV2(t *testing.T) {
	payload := []byte(`{"scheme":"nexus-escrow-v1","network":"eip155:44005","asset":"0x456569838f1d43009A91ebA4263147Afd7233483","amount":"150000","payTo":"0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266","taskHash":"0x0101010101010101010101010101010101010101010101010101010101010101","timeoutBlocks":50}`)
	header := make(http.Header)
	header.Set("PAYMENT-REQUIRED", base64Encode(payload))
	challenge, err := parsePaymentHeaders(header)
	if err != nil {
		t.Fatal(err)
	}
	if challenge.Protocol != "nexus-escrow-v1" {
		t.Fatalf("scheme %s", challenge.Protocol)
	}
	if challenge.Amount.Cmp(big.NewInt(150000)) != 0 {
		t.Fatalf("amount %s", challenge.Amount)
	}
}

func base64Encode(payload []byte) string {
	return base64.StdEncoding.EncodeToString(payload)
}

func TestReserveRejectsPerTaskAndHourlyCaps(t *testing.T) {
	client := NewClient("http://127.0.0.1:8080", nil, big.NewInt(1000), big.NewInt(1500))
	if err := client.reserve(big.NewInt(1001)); err == nil {
		t.Fatal("expected per-task rejection")
	}
	if err := client.reserve(big.NewInt(1000)); err != nil {
		t.Fatal(err)
	}
	if err := client.reserve(big.NewInt(1000)); err == nil {
		t.Fatal("expected hourly rejection")
	}
	client.release(big.NewInt(1000))
	if client.Spent().Sign() != 0 {
		t.Fatalf("spent %s", client.Spent())
	}
}
