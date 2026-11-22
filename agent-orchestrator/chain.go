package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const lockABI = `[{"inputs":[{"name":"taskId","type":"bytes32"},{"name":"worker","type":"address"},{"name":"amount","type":"uint256"},{"name":"deadline","type":"uint256"},{"name":"taskHash","type":"bytes32"}],"name":"lockTaskEscrow","outputs":[],"stateMutability":"nonpayable","type":"function"}]`

type Chain struct {
	client  *ethclient.Client
	key     *ecdsa.PrivateKey
	from    common.Address
	escrow  common.Address
	chainID *big.Int
	abi     abi.ABI
}

func DialChain(ctx context.Context, rpc string, privateKey string, escrow common.Address, chainID int64) (*Chain, error) {
	key, err := crypto.HexToECDSA(strings.TrimPrefix(strings.TrimSpace(privateKey), "0x"))
	if err != nil {
		return nil, fmt.Errorf("session key: %w", err)
	}
	client, err := ethclient.DialContext(ctx, rpc)
	if err != nil {
		return nil, fmt.Errorf("dial aevum: %w", err)
	}
	parsed, err := abi.JSON(strings.NewReader(lockABI))
	if err != nil {
		return nil, err
	}
	return &Chain{
		client:  client,
		key:     key,
		from:    crypto.PubkeyToAddress(key.PublicKey),
		escrow:  escrow,
		chainID: big.NewInt(chainID),
		abi:     parsed,
	}, nil
}

func (c *Chain) Lock(ctx context.Context, taskID common.Hash, worker common.Address, amount *big.Int, timeoutBlocks uint64, taskHash common.Hash) (common.Hash, error) {
	block, err := c.client.BlockNumber(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("block number: %w", err)
	}
	deadline := new(big.Int).SetUint64(block + timeoutBlocks)
	data, err := c.abi.Pack("lockTaskEscrow", taskID, worker, amount, deadline, taskHash)
	if err != nil {
		return common.Hash{}, err
	}

	nonce, err := c.client.PendingNonceAt(ctx, c.from)
	if err != nil {
		return common.Hash{}, err
	}
	msg := ethereum.CallMsg{From: c.from, To: &c.escrow, Data: data}
	gas, err := c.client.EstimateGas(ctx, msg)
	if err != nil {
		return common.Hash{}, fmt.Errorf("estimate lockTaskEscrow: %w", err)
	}
	gas = gas * 12 / 10

	tx, err := c.sign(ctx, nonce, gas, data)
	if err != nil {
		return common.Hash{}, err
	}
	if err := c.client.SendTransaction(ctx, tx); err != nil {
		return common.Hash{}, fmt.Errorf("send lockTaskEscrow: %w", err)
	}
	receipt, err := c.wait(ctx, tx.Hash())
	if err != nil {
		return tx.Hash(), err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return tx.Hash(), fmt.Errorf("lockTaskEscrow reverted")
	}
	return tx.Hash(), nil
}

func (c *Chain) sign(ctx context.Context, nonce uint64, gas uint64, data []byte) (*types.Transaction, error) {
	to := c.escrow
	header, err := c.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	var tx *types.Transaction
	if header.BaseFee != nil {
		tip, err := c.client.SuggestGasTipCap(ctx)
		if err != nil {
			return nil, err
		}
		feeCap := new(big.Int).Mul(header.BaseFee, big.NewInt(2))
		feeCap.Add(feeCap, tip)
		tx = types.NewTx(&types.DynamicFeeTx{
			ChainID:   c.chainID,
			Nonce:     nonce,
			GasTipCap: tip,
			GasFeeCap: feeCap,
			Gas:       gas,
			To:        &to,
			Value:     big.NewInt(0),
			Data:      data,
		})
	} else {
		gasPrice, err := c.client.SuggestGasPrice(ctx)
		if err != nil {
			return nil, err
		}
		tx = types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			GasPrice: gasPrice,
			Gas:      gas,
			To:       &to,
			Value:    big.NewInt(0),
			Data:     data,
		})
	}
	signer := types.LatestSignerForChainID(c.chainID)
	return types.SignTx(tx, signer, c.key)
}

func (c *Chain) wait(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	for {
		receipt, err := c.client.TransactionReceipt(ctx, hash)
		if err == nil {
			return receipt, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for %s: %w", hash.Hex(), ctx.Err())
		case <-ticker.C:
		}
	}
}
