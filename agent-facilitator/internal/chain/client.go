package chain

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
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
	"github.com/nexus402/agent-facilitator/internal/x402"
)

const escrowABI = `[
	{"name":"getTask","type":"function","stateMutability":"view","inputs":[{"name":"taskId","type":"bytes32"}],"outputs":[{"name":"","type":"tuple","components":[
		{"name":"taskId","type":"bytes32"},
		{"name":"principal","type":"address"},
		{"name":"agentKey","type":"address"},
		{"name":"worker","type":"address"},
		{"name":"amount","type":"uint256"},
		{"name":"deadline","type":"uint256"},
		{"name":"taskHash","type":"bytes32"},
		{"name":"resultHash","type":"bytes32"},
		{"name":"status","type":"uint8"}
	]}]},
	{"name":"completeTaskWithProof","type":"function","stateMutability":"nonpayable","inputs":[
		{"name":"taskId","type":"bytes32"},
		{"name":"resultHash","type":"bytes32"},
		{"name":"workerSignature","type":"bytes"}
	],"outputs":[]},
	{"name":"completeTaskMetered","type":"function","stateMutability":"nonpayable","inputs":[
		{"name":"taskId","type":"bytes32"},
		{"name":"actualAmount","type":"uint256"},
		{"name":"resultHash","type":"bytes32"},
		{"name":"workerSignature","type":"bytes"}
	],"outputs":[]}
]`

type onchainTask struct {
	TaskId     [32]byte
	Principal  common.Address
	AgentKey   common.Address
	Worker     common.Address
	Amount     *big.Int
	Deadline   *big.Int
	TaskHash   [32]byte
	ResultHash [32]byte
	Status     uint8
}

type Client struct {
	ec      *ethclient.Client
	abi     abi.ABI
	escrow  common.Address
	key     *ecdsa.PrivateKey
	from    common.Address
	chainID *big.Int
}

func Dial(ctx context.Context, rpcURL, escrowHex, privateKeyHex string) (*Client, error) {
	ec, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, err
	}
	parsed, err := abi.JSON(strings.NewReader(escrowABI))
	if err != nil {
		ec.Close()
		return nil, err
	}
	client := &Client{
		ec:      ec,
		abi:     parsed,
		escrow:  common.HexToAddress(escrowHex),
		chainID: big.NewInt(44005),
	}
	if strings.TrimSpace(privateKeyHex) != "" {
		key, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
		if err != nil {
			ec.Close()
			return nil, fmt.Errorf("invalid facilitator key: %w", err)
		}
		client.key = key
		client.from = crypto.PubkeyToAddress(key.PublicKey)
	}
	return client, nil
}

func (c *Client) ChainID(ctx context.Context) (*big.Int, error) {
	return c.ec.ChainID(ctx)
}

func (c *Client) GetTask(ctx context.Context, taskID [32]byte) (x402.Task, error) {
	if c.escrow == (common.Address{}) {
		return x402.Task{}, fmt.Errorf("NEXUS_ESCROW_ADDRESS is unset")
	}
	data, err := c.abi.Pack("getTask", taskID)
	if err != nil {
		return x402.Task{}, err
	}
	raw, err := c.ec.CallContract(ctx, ethereum.CallMsg{To: &c.escrow, Data: data}, nil)
	if err != nil {
		return x402.Task{}, err
	}
	values, err := c.abi.Unpack("getTask", raw)
	if err != nil {
		return x402.Task{}, err
	}
	if len(values) != 1 {
		return x402.Task{}, fmt.Errorf("unexpected getTask return")
	}
	decoded := abi.ConvertType(values[0], new(onchainTask)).(*onchainTask)
	return x402.Task{
		Status:   decoded.Status,
		Worker:   decoded.Worker,
		Amount:   decoded.Amount,
		TaskHash: decoded.TaskHash,
	}, nil
}

func (c *Client) CanSettle() bool {
	return c.key != nil && c.escrow != (common.Address{})
}

type SettleRequest struct {
	TaskID          [32]byte
	ActualAmount    *big.Int
	ResultHash      [32]byte
	WorkerSignature []byte
}

func (c *Client) Settle(ctx context.Context, req SettleRequest) (string, error) {
	if !c.CanSettle() {
		return "", fmt.Errorf("facilitator gas wallet is unset")
	}
	var data []byte
	var err error
	if req.ActualAmount != nil && req.ActualAmount.Sign() > 0 {
		data, err = c.abi.Pack("completeTaskMetered", req.TaskID, req.ActualAmount, req.ResultHash, req.WorkerSignature)
	} else {
		data, err = c.abi.Pack("completeTaskWithProof", req.TaskID, req.ResultHash, req.WorkerSignature)
	}
	if err != nil {
		return "", err
	}
	nonce, err := c.ec.PendingNonceAt(ctx, c.from)
	if err != nil {
		return "", err
	}
	gasPrice, err := c.ec.SuggestGasPrice(ctx)
	if err != nil || gasPrice == nil || gasPrice.Sign() <= 0 {
		gasPrice = big.NewInt(20_000_000_000)
	}
	tx := types.NewTransaction(nonce, c.escrow, big.NewInt(0), 350_000, gasPrice, data)
	signed, err := types.SignTx(tx, types.NewCancunSigner(c.chainID), c.key)
	if err != nil {
		return "", err
	}
	if err := c.ec.SendTransaction(ctx, signed); err != nil {
		return signed.Hash().Hex(), err
	}
	receipt, err := waitMined(ctx, c.ec, signed)
	if err != nil {
		return signed.Hash().Hex(), err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return signed.Hash().Hex(), fmt.Errorf("settlement reverted")
	}
	return signed.Hash().Hex(), nil
}

func waitMined(ctx context.Context, ec *ethclient.Client, tx *types.Transaction) (*types.Receipt, error) {
	for {
		receipt, err := ec.TransactionReceipt(ctx, tx.Hash())
		if err == nil && receipt != nil {
			return receipt, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func ParseBytes32(raw string) ([32]byte, error) {
	var out [32]byte
	clean := strings.TrimPrefix(strings.TrimSpace(raw), "0x")
	decoded, err := hex.DecodeString(clean)
	if err != nil || len(decoded) != 32 {
		return out, fmt.Errorf("expected 32-byte hex")
	}
	copy(out[:], decoded)
	return out, nil
}
