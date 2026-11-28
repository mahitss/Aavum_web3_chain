package x402

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"unicode/utf16"

	"golang.org/x/text/unicode/norm"
)

// ChainReceipt is the x402 V2 lifecycle from delegation through settlement or refund.
type ChainReceipt struct {
	DelegationGrant   DelegationGrant    `json:"delegationGrant"`
	PaymentIntent     PaymentIntent      `json:"paymentIntent"`
	SettlementReceipt *SettlementReceipt `json:"settlementReceipt,omitempty"`
	RefundClaim       *RefundClaim       `json:"refundClaim,omitempty"`
}

type DelegationGrant struct {
	Kind         string `json:"kind"`
	Principal    string `json:"principal"`
	Agent        string `json:"agent"`
	ChainID      int64  `json:"chainId"`
	HourlyLimit  string `json:"hourlyLimit"`
	PerTaskLimit string `json:"perTaskLimit"`
	Digest       string `json:"digest,omitempty"`
}

type PaymentIntent struct {
	Kind        string `json:"kind"`
	Nonce       string `json:"nonce"`
	TaskID      string `json:"taskId"`
	Amount      string `json:"amount"`
	Scheme      string `json:"scheme"`
	GrantDigest string `json:"grantDigest"`
}

type SettlementReceipt struct {
	Kind        string `json:"kind"`
	TaskID      string `json:"taskId"`
	TxHash      string `json:"txHash"`
	GrantDigest string `json:"grantDigest"`
	IntentNonce string `json:"intentNonce"`
	ResultHash  string `json:"resultHash"`
}

type RefundClaim struct {
	Kind        string `json:"kind"`
	TaskID      string `json:"taskId"`
	TxHash      string `json:"txHash"`
	GrantDigest string `json:"grantDigest"`
	IntentNonce string `json:"intentNonce"`
	Amount      string `json:"amount"`
	Reason      string `json:"reason"`
}

type ReceiptDocument struct {
	TaskID    string       `json:"taskId"`
	Canonical string       `json:"canonical"`
	Digest    string       `json:"digest"`
	Receipt   ChainReceipt `json:"receipt"`
	LockTx    string       `json:"lockTx,omitempty"`
	SettleTx  string       `json:"settleTx,omitempty"`
	RefundTx  string       `json:"refundTx,omitempty"`
}

type ReceiptInput struct {
	Principal    string
	Agent        string
	ChainID      int64
	HourlyLimit  string
	PerTaskLimit string
	Nonce        string
	TaskID       string
	Amount       string
	Scheme       string
	ResultHash   string
	LockTx       string
	SettleTx     string
	RefundTx     string
	Reason       string
}

func BuildReceipt(input ReceiptInput) (ReceiptDocument, error) {
	grant := DelegationGrant{
		Kind:         "DelegationGrant",
		Principal:    input.Principal,
		Agent:        input.Agent,
		ChainID:      input.ChainID,
		HourlyLimit:  input.HourlyLimit,
		PerTaskLimit: input.PerTaskLimit,
	}
	grantCanonical, err := Canonical(grant)
	if err != nil {
		return ReceiptDocument{}, err
	}
	grant.Digest = DigestHex(grantCanonical)
	intent := PaymentIntent{
		Kind:        "PaymentIntent",
		Nonce:       input.Nonce,
		TaskID:      input.TaskID,
		Amount:      input.Amount,
		Scheme:      input.Scheme,
		GrantDigest: grant.Digest,
	}
	receipt := ChainReceipt{DelegationGrant: grant, PaymentIntent: intent}
	if input.SettleTx != "" {
		receipt.SettlementReceipt = &SettlementReceipt{
			Kind:        "SettlementReceipt",
			TaskID:      input.TaskID,
			TxHash:      input.SettleTx,
			GrantDigest: grant.Digest,
			IntentNonce: input.Nonce,
			ResultHash:  input.ResultHash,
		}
	}
	if input.RefundTx != "" {
		receipt.RefundClaim = &RefundClaim{
			Kind:        "RefundClaim",
			TaskID:      input.TaskID,
			TxHash:      input.RefundTx,
			GrantDigest: grant.Digest,
			IntentNonce: input.Nonce,
			Amount:      input.Amount,
			Reason:      input.Reason,
		}
	}
	canonical, err := Canonical(receipt)
	if err != nil {
		return ReceiptDocument{}, err
	}
	return ReceiptDocument{
		TaskID:    input.TaskID,
		Canonical: canonical,
		Digest:    DigestHex(canonical),
		Receipt:   receipt,
		LockTx:    input.LockTx,
		SettleTx:  input.SettleTx,
		RefundTx:  input.RefundTx,
	}, nil
}

func DigestHex(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

// Canonical serializes value with RFC 8785 key ordering and Unicode NFC.
func Canonical(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	encoded, err := encodeCanonical(parsed)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func encodeCanonical(value any) ([]byte, error) {
	switch typed := value.(type) {
	case nil:
		return []byte("null"), nil
	case bool, float64, json.Number:
		return json.Marshal(typed)
	case string:
		return json.Marshal(norm.NFC.String(typed))
	case []any:
		out := []byte{'['}
		for i, item := range typed {
			if i > 0 {
				out = append(out, ',')
			}
			encoded, err := encodeCanonical(item)
			if err != nil {
				return nil, err
			}
			out = append(out, encoded...)
		}
		return append(out, ']'), nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			next := norm.NFC.String(key)
			keys = append(keys, next)
			normalized[next] = item
		}
		sort.Slice(keys, func(i, j int) bool {
			return utf16Less(keys[i], keys[j])
		})
		out := []byte{'{'}
		for i, key := range keys {
			if i > 0 {
				out = append(out, ',')
			}
			encodedKey, err := json.Marshal(key)
			if err != nil {
				return nil, err
			}
			encoded, err := encodeCanonical(normalized[key])
			if err != nil {
				return nil, err
			}
			out = append(out, encodedKey...)
			out = append(out, ':')
			out = append(out, encoded...)
		}
		return append(out, '}'), nil
	default:
		return nil, fmt.Errorf("unsupported canonical value %T", value)
	}
}

func utf16Less(left, right string) bool {
	a := utf16.Encode([]rune(left))
	b := utf16.Encode([]rune(right))
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}
