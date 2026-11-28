package aevum

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const evaluatedABI = `[
	{"name":"lockEvaluatedEscrow","type":"function","inputs":[
		{"name":"principal","type":"address"},
		{"name":"worker","type":"address"},
		{"name":"evaluator","type":"address"},
		{"name":"amount","type":"uint256"},
		{"name":"durationBlocks","type":"uint256"},
		{"name":"challengeWindowBlocks","type":"uint256"},
		{"name":"taskHash","type":"bytes32"}
	],"outputs":[{"name":"taskId","type":"bytes32"}]},
	{"name":"evaluateTask","type":"function","inputs":[
		{"name":"taskId","type":"bytes32"},
		{"name":"approved","type":"bool"},
		{"name":"evaluationHash","type":"bytes32"},
		{"name":"evaluatorSignature","type":"bytes"}
	],"outputs":[]},
	{"name":"TaskEscrowLocked","type":"event","inputs":[
		{"name":"taskId","type":"bytes32","indexed":true},
		{"name":"principal","type":"address","indexed":true},
		{"name":"worker","type":"address","indexed":true},
		{"name":"amount","type":"uint256","indexed":false},
		{"name":"deadline","type":"uint256","indexed":false}
	]}
]`

func (c *Client) LockEvaluatedEscrow(ctx context.Context, worker, evaluator common.Address, amount *big.Int, durationBlocks, challengeWindow uint64, taskHash [32]byte) ([32]byte, string, uint64, error) {
	var taskID [32]byte
	parsed, err := abi.JSON(strings.NewReader(evaluatedABI))
	if err != nil {
		return taskID, "", 0, err
	}
	data, err := parsed.Pack(
		"lockEvaluatedEscrow",
		c.principal,
		worker,
		evaluator,
		amount,
		new(big.Int).SetUint64(durationBlocks),
		new(big.Int).SetUint64(challengeWindow),
		taskHash,
	)
	if err != nil {
		return taskID, "", 0, err
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	txHash, receipt, err := c.sendSigned(ctx, c.privateKey, data, 400_000)
	if err != nil {
		return taskID, txHash, 0, err
	}
	topic := parsed.Events["TaskEscrowLocked"].ID
	for _, log := range receipt.Logs {
		if len(log.Topics) > 1 && log.Topics[0] == topic {
			copy(taskID[:], log.Topics[1].Bytes())
			deadline := receipt.BlockNumber.Uint64() + durationBlocks
			if len(log.Data) >= 64 {
				deadline = new(big.Int).SetBytes(log.Data[32:64]).Uint64()
			}
			return taskID, txHash, deadline, nil
		}
	}
	return taskID, txHash, 0, fmt.Errorf("TaskEscrowLocked event not found")
}

func (c *Client) EvaluateTask(ctx context.Context, evaluatorKeyHex string, taskID [32]byte, approved bool, reason string) (string, common.Address, error) {
	key, err := crypto.HexToECDSA(strings.TrimPrefix(strings.TrimSpace(evaluatorKeyHex), "0x"))
	if err != nil {
		return "", common.Address{}, fmt.Errorf("invalid evaluator key: %w", err)
	}
	parsed, err := abi.JSON(strings.NewReader(evaluatedABI))
	if err != nil {
		return "", crypto.PubkeyToAddress(key.PublicKey), err
	}
	evaluationHash := crypto.Keccak256Hash([]byte(reason))
	packed := append(append([]byte{}, taskID[:]...), boolByte(approved))
	packed = append(packed, evaluationHash.Bytes()...)
	signature, err := crypto.Sign(accounts.TextHash(crypto.Keccak256(packed)), key)
	if err != nil {
		return "", crypto.PubkeyToAddress(key.PublicKey), err
	}
	signature[64] += 27
	data, err := parsed.Pack("evaluateTask", taskID, approved, evaluationHash, signature)
	if err != nil {
		return "", crypto.PubkeyToAddress(key.PublicKey), err
	}
	txHash, _, err := c.sendSigned(ctx, key, data, 350_000)
	return txHash, crypto.PubkeyToAddress(key.PublicKey), err
}

func (c *Client) sendSigned(ctx context.Context, key *ecdsa.PrivateKey, data []byte, gas uint64) (string, *types.Receipt, error) {
	from := crypto.PubkeyToAddress(key.PublicKey)
	nonce, err := c.ec.PendingNonceAt(ctx, from)
	if err != nil {
		return "", nil, err
	}
	gasPrice, err := c.ec.SuggestGasPrice(ctx)
	if err != nil || gasPrice == nil || gasPrice.Sign() <= 0 {
		gasPrice = big.NewInt(20_000_000_000)
	}
	tx := types.NewTransaction(nonce, c.escrowAddress, big.NewInt(0), gas, gasPrice, data)
	signed, err := types.SignTx(tx, types.NewCancunSigner(c.chainID), key)
	if err != nil {
		return "", nil, err
	}
	if err := c.ec.SendTransaction(ctx, signed); err != nil {
		return signed.Hash().Hex(), nil, c.revertError(ctx, data, err)
	}
	receipt, err := bindWaitMined(ctx, c.ec, signed)
	if err != nil {
		return signed.Hash().Hex(), nil, err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return signed.Hash().Hex(), receipt, c.revertError(ctx, data, fmt.Errorf("transaction reverted: %s", signed.Hash().Hex()))
	}
	return signed.Hash().Hex(), receipt, nil
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}
