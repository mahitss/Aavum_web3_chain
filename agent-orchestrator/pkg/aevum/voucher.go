package aevum

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const voucherABI = `[
	{"name":"openSessionTab","type":"function","inputs":[
		{"name":"principal","type":"address"},
		{"name":"worker","type":"address"},
		{"name":"depositAmount","type":"uint256"},
		{"name":"durationBlocks","type":"uint256"}
	],"outputs":[{"name":"tabId","type":"bytes32"}]},
	{"name":"SessionTabOpened","type":"event","inputs":[
		{"name":"tabId","type":"bytes32","indexed":true},
		{"name":"principal","type":"address","indexed":true},
		{"name":"worker","type":"address","indexed":true},
		{"name":"depositAmount","type":"uint256","indexed":false},
		{"name":"deadline","type":"uint256","indexed":false}
	]}
]`

func (c *Client) OpenSessionTab(ctx context.Context, worker common.Address, deposit *big.Int, durationBlocks uint64) ([32]byte, string, error) {
	var tabID [32]byte
	parsed, err := abi.JSON(strings.NewReader(voucherABI))
	if err != nil {
		return tabID, "", err
	}
	data, err := parsed.Pack("openSessionTab", c.principal, worker, deposit, new(big.Int).SetUint64(durationBlocks))
	if err != nil {
		return tabID, "", err
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	nonce, err := c.ec.PendingNonceAt(ctx, c.agentAddress)
	if err != nil {
		return tabID, "", err
	}
	gasPrice, err := c.ec.SuggestGasPrice(ctx)
	if err != nil || gasPrice == nil || gasPrice.Sign() <= 0 {
		gasPrice = big.NewInt(20_000_000_000)
	}
	tx := types.NewTransaction(nonce, c.escrowAddress, big.NewInt(0), 350_000, gasPrice, data)
	signed, err := types.SignTx(tx, types.NewCancunSigner(c.chainID), c.privateKey)
	if err != nil {
		return tabID, "", err
	}
	if err := c.ec.SendTransaction(ctx, signed); err != nil {
		return tabID, signed.Hash().Hex(), c.revertError(ctx, data, err)
	}
	receipt, err := bindWaitMined(ctx, c.ec, signed)
	if err != nil {
		return tabID, signed.Hash().Hex(), err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return tabID, signed.Hash().Hex(), fmt.Errorf("openSessionTab reverted")
	}
	topic := parsed.Events["SessionTabOpened"].ID
	for _, log := range receipt.Logs {
		if len(log.Topics) > 1 && log.Topics[0] == topic {
			copy(tabID[:], log.Topics[1].Bytes())
			return tabID, signed.Hash().Hex(), nil
		}
	}
	return tabID, signed.Hash().Hex(), fmt.Errorf("SessionTabOpened event not found")
}

func (c *Client) SignVoucher(tabID [32]byte, cumulative *big.Int, nonce uint64, validBefore uint64) ([]byte, error) {
	return signVoucher(c.privateKey, c.escrowAddress, tabID, cumulative, nonce, validBefore)
}

func (c *Client) AgentPassport(ctx context.Context, registry common.Address, wallet common.Address) (uint64, error) {
	parsed, err := abi.JSON(strings.NewReader(`[{"name":"agentIdByWallet","type":"function","stateMutability":"view","inputs":[{"name":"wallet","type":"address"}],"outputs":[{"name":"","type":"uint256"}]}]`))
	if err != nil {
		return 0, err
	}
	data, err := parsed.Pack("agentIdByWallet", wallet)
	if err != nil {
		return 0, err
	}
	raw, err := c.ec.CallContract(ctx, ethereum.CallMsg{To: &registry, Data: data}, nil)
	if err != nil {
		return 0, err
	}
	values, err := parsed.Unpack("agentIdByWallet", raw)
	if err != nil || len(values) != 1 {
		return 0, fmt.Errorf("unexpected passport id")
	}
	id, ok := values[0].(*big.Int)
	if !ok {
		return 0, fmt.Errorf("passport id is not a uint256")
	}
	return id.Uint64(), nil
}

func signVoucher(key *ecdsa.PrivateKey, escrow common.Address, tabID [32]byte, cumulative *big.Int, nonce uint64, validBefore uint64) ([]byte, error) {
	domainType := crypto.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
	nameHash := crypto.Keccak256([]byte("Nexus402 Protocol"))
	versionHash := crypto.Keccak256([]byte("1"))
	voucherType := crypto.Keccak256([]byte("PaymentVoucher(bytes32 tabId,uint256 cumulativeAmount,uint256 nonce,uint256 validBefore)"))
	bytes32Ty, _ := abi.NewType("bytes32", "", nil)
	uintTy, _ := abi.NewType("uint256", "", nil)
	addressTy, _ := abi.NewType("address", "", nil)
	domainArgs := abi.Arguments{{Type: bytes32Ty}, {Type: bytes32Ty}, {Type: bytes32Ty}, {Type: uintTy}, {Type: addressTy}}
	var domainTypeWord, nameWord, versionWord [32]byte
	copy(domainTypeWord[:], domainType)
	copy(nameWord[:], nameHash)
	copy(versionWord[:], versionHash)
	encodedDomain, err := domainArgs.Pack(domainTypeWord, nameWord, versionWord, big.NewInt(44005), escrow)
	if err != nil {
		return nil, err
	}
	domainSeparator := crypto.Keccak256(encodedDomain)
	structArgs := abi.Arguments{{Type: bytes32Ty}, {Type: bytes32Ty}, {Type: uintTy}, {Type: uintTy}, {Type: uintTy}}
	var voucherWord [32]byte
	copy(voucherWord[:], voucherType)
	encodedStruct, err := structArgs.Pack(voucherWord, tabID, cumulative, new(big.Int).SetUint64(nonce), new(big.Int).SetUint64(validBefore))
	if err != nil {
		return nil, err
	}
	structHash := crypto.Keccak256(encodedStruct)
	digest := crypto.Keccak256(append(append([]byte{0x19, 0x01}, domainSeparator...), structHash...))
	signature, err := crypto.Sign(digest, key)
	if err != nil {
		return nil, err
	}
	signature[64] += 27
	return signature, nil
}
