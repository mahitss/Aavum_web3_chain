package aevum

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const tusd8Address = "0x456569838f1d43009A91ebA4263147Afd7233483"

const readABI = `[
	{
		"name": "balanceOf",
		"type": "function",
		"stateMutability": "view",
		"inputs": [{"name": "account", "type": "address"}],
		"outputs": [{"name": "", "type": "uint256"}]
	},
	{
		"name": "getSession",
		"type": "function",
		"stateMutability": "view",
		"inputs": [
			{"name": "principal", "type": "address"},
			{"name": "agentKey", "type": "address"}
		],
		"outputs": [{
			"name": "",
			"type": "tuple",
			"components": [
				{"name": "agentKey", "type": "address"},
				{"name": "hourlyLimit", "type": "uint256"},
				{"name": "perTaskLimit", "type": "uint256"},
				{"name": "expiresAt", "type": "uint256"},
				{"name": "currentWindow", "type": "uint256"},
				{"name": "spentInWindow", "type": "uint256"},
				{"name": "isActive", "type": "bool"}
			]
		}]
	}
]`

type SandboxBalances struct {
	PrincipalTusd8 string
	AgentGas       string
	SessionActive  bool
}

func (c *Client) SandboxBalances(ctx context.Context) (SandboxBalances, error) {
	parsed, err := abi.JSON(strings.NewReader(readABI))
	if err != nil {
		return SandboxBalances{}, err
	}
	gas, err := c.ec.BalanceAt(ctx, c.agentAddress, nil)
	if err != nil {
		return SandboxBalances{}, fmt.Errorf("agent gas balance: %w", err)
	}
	token := common.HexToAddress(tusd8Address)
	tusd8, err := callUint(ctx, c, parsed, token, "balanceOf", c.principal)
	if err != nil {
		return SandboxBalances{}, fmt.Errorf("principal tUSD8 balance: %w", err)
	}
	active, err := c.sessionActive(ctx, parsed)
	if err != nil {
		return SandboxBalances{}, err
	}
	return SandboxBalances{
		PrincipalTusd8: tusd8.String(),
		AgentGas:       gas.String(),
		SessionActive:  active,
	}, nil
}

func (c *Client) sessionActive(ctx context.Context, parsed abi.ABI) (bool, error) {
	data, err := parsed.Pack("getSession", c.principal, c.agentAddress)
	if err != nil {
		return false, err
	}
	out, err := c.ec.CallContract(ctx, ethereum.CallMsg{To: &c.escrowAddress, Data: data}, nil)
	if err != nil {
		return false, fmt.Errorf("getSession: %w", err)
	}
	if len(out) < 32*7 {
		return false, fmt.Errorf("short getSession result")
	}
	return new(big.Int).SetBytes(out[len(out)-32:]).Sign() > 0, nil
}

func callUint(ctx context.Context, c *Client, parsed abi.ABI, to common.Address, method string, account common.Address) (*big.Int, error) {
	data, err := parsed.Pack(method, account)
	if err != nil {
		return nil, err
	}
	out, err := c.ec.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	values, err := parsed.Unpack(method, out)
	if err != nil {
		return nil, err
	}
	amount, ok := values[0].(*big.Int)
	if !ok || amount == nil {
		return big.NewInt(0), nil
	}
	return amount, nil
}
