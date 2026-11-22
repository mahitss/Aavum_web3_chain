package aevum

import (
	"context"
	"crypto/ecdsa"
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

const lockABI = `[{"name":"lockTaskEscrow","type":"function","stateMutability":"nonpayable","inputs":[{"name":"principal","type":"address"},{"name":"worker","type":"address"},{"name":"amount","type":"uint256"},{"name":"durationBlocks","type":"uint256"},{"name":"taskHash","type":"bytes32"}],"outputs":[{"name":"taskId","type":"bytes32"}]}]`

const lockedEvent = "TaskEscrowLocked(bytes32,address,address,uint256,uint256)"

// Client signs lockTaskEscrow as the orchestrator agent key.
type Client struct {
	rpc       *ethclient.Client
	key       *ecdsa.PrivateKey
	chainID   *big.Int
	escrow    common.Address
	principal common.Address
	parsed    abi.ABI
	eventID   common.Hash

	mu     sync.Mutex
	blocks map[string]uint64
}

func Dial(rpcURL string, chainID int64, privateKey string, escrow common.Address, principal common.Address) (*Client, error) {
	rpc, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, err
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(privateKey, "0x"))
	if err != nil {
		rpc.Close()
		return nil, fmt.Errorf("ORCHESTRATOR_AGENT_PRIVATE_KEY: %w", err)
	}
	parsed, err := abi.JSON(strings.NewReader(lockABI))
	if err != nil {
		rpc.Close()
		return nil, err
	}
	return &Client{
		rpc:       rpc,
		key:       key,
		chainID:   big.NewInt(chainID),
		escrow:    escrow,
		principal: principal,
		parsed:    parsed,
		eventID:   crypto.Keccak256Hash([]byte(lockedEvent)),
		blocks:    map[string]uint64{},
	}, nil
}

func (c *Client) Close() {
	if c != nil && c.rpc != nil {
		c.rpc.Close()
	}
}

func (c *Client) AgentAddress() common.Address {
	return crypto.PubkeyToAddress(c.key.PublicKey)
}

func (c *Client) Connected(ctx context.Context) bool {
	if c == nil || c.rpc == nil {
		return false
	}
	_, err := c.rpc.ChainID(ctx)
	return err == nil
}

func (c *Client) Block(txHash string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blocks[strings.ToLower(txHash)]
}

// LockEscrow broadcasts lockTaskEscrow and returns the task id from TaskEscrowLocked.
func (c *Client) LockEscrow(worker common.Address, amount *big.Int, durationBlocks uint64, taskHash [32]byte) ([32]byte, string, error) {
	var taskID [32]byte
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	data, err := c.parsed.Pack("lockTaskEscrow", c.principal, worker, amount, new(big.Int).SetUint64(durationBlocks), taskHash)
	if err != nil {
		return taskID, "", err
	}
	from := c.AgentAddress()
	nonce, err := c.rpc.PendingNonceAt(ctx, from)
	if err != nil {
		return taskID, "", err
	}
	gasPrice, err := c.rpc.SuggestGasPrice(ctx)
	if err != nil || gasPrice == nil || gasPrice.Sign() == 0 {
		gasPrice = big.NewInt(1_000_000_000)
	}
	to := c.escrow
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		GasPrice: gasPrice,
		Gas:      300000,
		To:       &to,
		Value:    big.NewInt(0),
		Data:     data,
	})
	signed, err := types.SignTx(tx, types.NewEIP155Signer(c.chainID), c.key)
	if err != nil {
		return taskID, "", err
	}
	if err := c.rpc.SendTransaction(ctx, signed); err != nil {
		return taskID, "", err
	}
	receipt, err := waitReceipt(ctx, c.rpc, signed.Hash())
	if err != nil {
		return taskID, signed.Hash().Hex(), err
	}
	if receipt.Status == 0 {
		return taskID, signed.Hash().Hex(), fmt.Errorf("lock reverted in block %d", receipt.BlockNumber.Uint64())
	}
	taskID, err = taskIDFromReceipt(receipt, c.eventID)
	if err != nil {
		return taskID, signed.Hash().Hex(), err
	}
	c.mu.Lock()
	c.blocks[strings.ToLower(signed.Hash().Hex())] = receipt.BlockNumber.Uint64()
	c.mu.Unlock()
	return taskID, signed.Hash().Hex(), nil
}

func waitReceipt(ctx context.Context, rpc *ethclient.Client, hash common.Hash) (*types.Receipt, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		receipt, err := rpc.TransactionReceipt(ctx, hash)
		if err == nil && receipt != nil {
			return receipt, nil
		}
		if err != nil && err != ethereum.NotFound {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func taskIDFromReceipt(receipt *types.Receipt, eventID common.Hash) ([32]byte, error) {
	var taskID [32]byte
	for _, entry := range receipt.Logs {
		if len(entry.Topics) < 2 || entry.Topics[0] != eventID {
			continue
		}
		copy(taskID[:], entry.Topics[1].Bytes())
		return taskID, nil
	}
	return taskID, fmt.Errorf("TaskEscrowLocked log missing from receipt")
}
