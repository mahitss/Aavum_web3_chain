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

const reputationABI = `[{"name":"getWorkerReputation","type":"function","stateMutability":"view","inputs":[{"name":"worker","type":"address"}],"outputs":[{"name":"stats","type":"tuple","components":[{"name":"tasksCompleted","type":"uint256"},{"name":"tasksRefunded","type":"uint256"},{"name":"totalVolumeSettled","type":"uint256"},{"name":"lastActiveBlock","type":"uint256"}]},{"name":"reliabilityBps","type":"uint256"}]}]`

type reputationStats struct {
	TasksCompleted     *big.Int
	TasksRefunded      *big.Int
	TotalVolumeSettled *big.Int
	LastActiveBlock    *big.Int
}

func (c *Client) WorkerReputation(ctx context.Context, worker common.Address) (completed uint64, refunded uint64, volume string, bps uint64, err error) {
	parsed, err := abi.JSON(strings.NewReader(reputationABI))
	if err != nil {
		return 0, 0, "", 0, err
	}
	data, err := parsed.Pack("getWorkerReputation", worker)
	if err != nil {
		return 0, 0, "", 0, err
	}
	raw, err := c.ec.CallContract(ctx, ethereum.CallMsg{To: &c.escrowAddress, Data: data}, nil)
	if err != nil {
		return 0, 0, "", 0, err
	}
	values, err := parsed.Unpack("getWorkerReputation", raw)
	if err != nil {
		return 0, 0, "", 0, err
	}
	if len(values) != 2 {
		return 0, 0, "", 0, fmt.Errorf("unexpected reputation return")
	}
	stats := abi.ConvertType(values[0], new(reputationStats)).(*reputationStats)
	points, ok := values[1].(*big.Int)
	if !ok {
		return 0, 0, "", 0, fmt.Errorf("reliability is not a uint256")
	}
	return stats.TasksCompleted.Uint64(), stats.TasksRefunded.Uint64(), stats.TotalVolumeSettled.String(), points.Uint64(), nil
}
