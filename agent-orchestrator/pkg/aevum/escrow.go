package aevum

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const EscrowABI = `[
	{
		"type": "function",
		"name": "lockTaskEscrow",
		"inputs": [
			{"name": "principal", "type": "address"},
			{"name": "worker", "type": "address"},
			{"name": "amount", "type": "uint256"},
			{"name": "durationBlocks", "type": "uint256"},
			{"name": "taskHash", "type": "bytes32"}
		],
		"outputs": [{"name": "taskId", "type": "bytes32"}]
	},
	{
		"type": "event",
		"name": "TaskEscrowLocked",
		"inputs": [
			{"name": "taskId", "type": "bytes32", "indexed": true},
			{"name": "principal", "type": "address", "indexed": true},
			{"name": "worker", "type": "address", "indexed": true},
			{"name": "amount", "type": "uint256", "indexed": false},
			{"name": "deadline", "type": "uint256", "indexed": false}
		]
	}
]`

type Client struct {
	ec            *ethclient.Client
	contractABI   abi.ABI
	escrowAddress common.Address
	privateKey    *ecdsa.PrivateKey
	agentAddress  common.Address
	principal     common.Address
	chainID       *big.Int
	sendMu        sync.Mutex
}

func NewClient(rpcURL string, privateKeyHex string, escrowAddrHex string, principalAddrHex string, chainID int64) (*Client, error) {
	ec, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RPC: %w", err)
	}

	cleanKey := strings.TrimPrefix(privateKeyHex, "0x")
	key, err := crypto.HexToECDSA(cleanKey)
	if err != nil {
		ec.Close()
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	parsedABI, err := abi.JSON(strings.NewReader(EscrowABI))
	if err != nil {
		ec.Close()
		return nil, fmt.Errorf("failed to parse contract ABI: %w", err)
	}

	return &Client{
		ec:            ec,
		contractABI:   parsedABI,
		escrowAddress: common.HexToAddress(escrowAddrHex),
		privateKey:    key,
		agentAddress:  crypto.PubkeyToAddress(key.PublicKey),
		principal:     common.HexToAddress(principalAddrHex),
		chainID:       big.NewInt(chainID),
	}, nil
}

func (c *Client) AgentAddress() common.Address {
	return c.agentAddress
}

func (c *Client) PrincipalAddress() common.Address {
	return c.principal
}

func (c *Client) LockEscrow(ctx context.Context, worker common.Address, amount *big.Int, durationBlocks uint64, taskHash [32]byte) ([32]byte, string, uint64, error) {
	var taskID [32]byte
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	inputData, err := c.contractABI.Pack(
		"lockTaskEscrow",
		c.principal,
		worker,
		amount,
		new(big.Int).SetUint64(durationBlocks),
		taskHash,
	)
	if err != nil {
		return taskID, "", 0, fmt.Errorf("failed to pack lockTaskEscrow calldata: %w", err)
	}

	nonce, err := c.ec.PendingNonceAt(ctx, c.agentAddress)
	if err != nil {
		return taskID, "", 0, fmt.Errorf("failed to get pending nonce: %w", err)
	}

	gasPrice, err := c.ec.SuggestGasPrice(ctx)
	if err != nil || gasPrice == nil || gasPrice.Sign() <= 0 {
		gasPrice = big.NewInt(20_000_000_000)
	}

	tx := types.NewTransaction(nonce, c.escrowAddress, big.NewInt(0), 300_000, gasPrice, inputData)
	signedTx, err := types.SignTx(tx, types.NewCancunSigner(c.chainID), c.privateKey)
	if err != nil {
		return taskID, "", 0, fmt.Errorf("failed to sign transaction: %w", err)
	}

	if err := c.ec.SendTransaction(ctx, signedTx); err != nil {
		return taskID, signedTx.Hash().Hex(), 0, c.revertError(ctx, inputData, err)
	}

	txHash := signedTx.Hash().Hex()
	receipt, err := bindWaitMined(ctx, c.ec, signedTx)
	if err != nil {
		return taskID, txHash, 0, fmt.Errorf("transaction confirmation failed: %w", err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return taskID, txHash, 0, c.revertError(ctx, inputData, fmt.Errorf("transaction execution reverted on-chain: %s", txHash))
	}

	eventTopic := c.contractABI.Events["TaskEscrowLocked"].ID
	for _, log := range receipt.Logs {
		if log.Address == c.escrowAddress && len(log.Topics) > 1 && log.Topics[0] == eventTopic {
			copy(taskID[:], log.Topics[1].Bytes())
			deadline := receipt.BlockNumber.Uint64() + durationBlocks
			if len(log.Data) >= 64 {
				deadline = new(big.Int).SetBytes(log.Data[32:64]).Uint64()
			}
			return taskID, txHash, deadline, nil
		}
	}
	return taskID, txHash, 0, fmt.Errorf("TaskEscrowLocked event not found in receipt")
}

func (c *Client) RefundExpired(ctx context.Context, taskID [32]byte) (string, error) {
	parsed, err := abi.JSON(strings.NewReader(`[{"name":"refundExpiredTask","type":"function","inputs":[{"name":"taskId","type":"bytes32"}],"outputs":[]}]`))
	if err != nil {
		return "", err
	}
	inputData, err := parsed.Pack("refundExpiredTask", taskID)
	if err != nil {
		return "", err
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	nonce, err := c.ec.PendingNonceAt(ctx, c.agentAddress)
	if err != nil {
		return "", err
	}
	gasPrice, err := c.ec.SuggestGasPrice(ctx)
	if err != nil || gasPrice == nil || gasPrice.Sign() <= 0 {
		gasPrice = big.NewInt(20_000_000_000)
	}
	tx := types.NewTransaction(nonce, c.escrowAddress, big.NewInt(0), 200_000, gasPrice, inputData)
	signedTx, err := types.SignTx(tx, types.NewCancunSigner(c.chainID), c.privateKey)
	if err != nil {
		return "", err
	}
	if err := c.ec.SendTransaction(ctx, signedTx); err != nil {
		return signedTx.Hash().Hex(), c.revertError(ctx, inputData, err)
	}
	receipt, err := bindWaitMined(ctx, c.ec, signedTx)
	if err != nil {
		return signedTx.Hash().Hex(), err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return signedTx.Hash().Hex(), c.revertError(ctx, inputData, fmt.Errorf("refund reverted: %s", signedTx.Hash().Hex()))
	}
	return signedTx.Hash().Hex(), nil
}

func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	return c.ec.BlockNumber(ctx)
}

func (c *Client) revertError(ctx context.Context, data []byte, fallback error) error {
	msg := ethereum.CallMsg{From: c.agentAddress, To: &c.escrowAddress, Data: data, Gas: 300_000}
	_, callErr := c.ec.CallContract(ctx, msg, nil)
	text := fallback.Error()
	if callErr != nil {
		text = callErr.Error() + " " + text
	}
	if name := namedCustomError(text); name != "" {
		return fmt.Errorf("%s: %w", name, fallback)
	}
	return fallback
}

func namedCustomError(text string) string {
	lower := strings.ToLower(text)
	for _, name := range []string{"ExceedsHourlyLimit", "SessionInactive", "ExceedsPerTaskLimit", "SessionExpired"} {
		selector := hex.EncodeToString(crypto.Keccak256([]byte(name + "()"))[:4])
		if strings.Contains(lower, strings.ToLower(name)) || strings.Contains(lower, selector) {
			return name
		}
	}
	return ""
}

func bindWaitMined(ctx context.Context, ec *ethclient.Client, tx *types.Transaction) (*types.Receipt, error) {
	ticker := time.NewTicker(800 * time.Millisecond)
	defer ticker.Stop()
	retries := 0
	for {
		receipt, err := ec.TransactionReceipt(ctx, tx.Hash())
		if err == nil {
			return receipt, nil
		}
		if !errors.Is(err, ethereum.NotFound) {
			retries++
			if retries > 3 {
				return nil, err
			}
			backoff := time.Duration(200*(1<<retries)) * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			continue
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
