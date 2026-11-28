package x402

import "testing"

func TestCanonicalReceiptLinksGrantAndIntent(t *testing.T) {
	doc, err := BuildReceipt(ReceiptInput{
		Principal:    "0x1111111111111111111111111111111111111111",
		Agent:        "0x2222222222222222222222222222222222222222",
		ChainID:      44005,
		HourlyLimit:  "10000000",
		PerTaskLimit: "1000000",
		Nonce:        "intent-1",
		TaskID:       "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Amount:       "300000",
		Scheme:       "auth-capture",
		RefundTx:     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Reason:       "output is not valid JSON",
		LockTx:       "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := Canonical(doc.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if again != doc.Canonical || DigestHex(again) != doc.Digest {
		t.Fatalf("digest is not stable: %s", doc.Digest)
	}
	if doc.Receipt.RefundClaim == nil {
		t.Fatal("refund claim missing")
	}
	if doc.Receipt.RefundClaim.GrantDigest != doc.Receipt.DelegationGrant.Digest {
		t.Fatal("refund claim is not bound to the delegation grant")
	}
	if doc.Receipt.RefundClaim.IntentNonce != doc.Receipt.PaymentIntent.Nonce {
		t.Fatal("refund claim is not bound to the payment intent nonce")
	}
	if doc.Receipt.DelegationGrant.Digest == "" || doc.Digest == doc.Receipt.DelegationGrant.Digest {
		t.Fatal("stage digests collapsed")
	}
}

func TestCanonicalSortsKeys(t *testing.T) {
	text, err := Canonical(map[string]string{"b": "2", "a": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if text != `{"a":"1","b":"2"}` {
		t.Fatalf("canonical form = %s", text)
	}
}
