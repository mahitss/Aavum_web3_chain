package evaluator

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/ethereum/go-ethereum/crypto"
)

// Inspect applies the deterministic ERC-8183 checks. A matching worker signature is not enough.
func Inspect(prompt, output, resultHash string) (bool, string) {
	if strings.TrimSpace(output) == "" {
		return false, "empty output"
	}
	sum := crypto.Keccak256([]byte(output))
	if !strings.EqualFold(hex.EncodeToString(sum), strings.TrimPrefix(resultHash, "0x")) {
		return false, "result hash mismatch"
	}
	if !json.Valid([]byte(output)) {
		return false, "output is not valid JSON"
	}
	if !relevant(prompt, output) {
		return false, "output is not relevant to the prompt"
	}
	return true, "accepted"
}

func relevant(prompt, output string) bool {
	outputFold := strings.ToLower(output)
	for _, word := range strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if len(word) >= 5 && strings.Contains(outputFold, word) {
			return true
		}
	}
	return false
}
