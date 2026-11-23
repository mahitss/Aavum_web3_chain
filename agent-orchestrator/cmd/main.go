package main

import (
	"context"
	"encoding/hex"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/nexus402/agent-orchestrator/pkg/aevum"
	"github.com/nexus402/agent-orchestrator/pkg/types"
	"github.com/nexus402/agent-orchestrator/pkg/x402"
)

type jobRequest struct {
	Prompt   string `json:"prompt"`
	TaskType string `json:"task_type"`
}

type jobView struct {
	ID          string `json:"id"`
	Prompt      string `json:"prompt"`
	TaskType    string `json:"task_type"`
	Status      string `json:"status"`
	TaskID      string `json:"task_id"`
	LockTxHash  string `json:"lock_tx_hash"`
	ClaimTxHash string `json:"claim_tx_hash"`
	Result      string `json:"result"`
	Timestamp   int64  `json:"timestamp"`
	LatencyMs   int64  `json:"latency_ms"`
	LockBlock   uint64 `json:"lock_block"`
	Amount      string `json:"amount"`
}

type ledgerRow struct {
	Kind   string `json:"kind"`
	TxHash string `json:"tx_hash"`
	Amount string `json:"amount"`
	Status string `json:"status"`
	Block  uint64 `json:"block"`
}

type hub struct {
	mu     sync.Mutex
	jobs   []*types.TaskJob
	logs   []string
	ledger []ledgerRow
}

func (h *hub) addLog(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.logs = append(h.logs, time.Now().Format("15:04:05")+" "+line)
	if len(h.logs) > 200 {
		h.logs = h.logs[len(h.logs)-200:]
	}
	log.Print(line)
}

func (h *hub) snapshot() ([]jobView, []string, []ledgerRow) {
	h.mu.Lock()
	defer h.mu.Unlock()
	views := make([]jobView, 0, len(h.jobs))
	for i := len(h.jobs) - 1; i >= 0; i-- {
		job := h.jobs[i]
		taskID := ""
		if job.TaskId != ([32]byte{}) {
			taskID = "0x" + hex.EncodeToString(job.TaskId[:])
		}
		views = append(views, jobView{
			ID:          job.ID,
			Prompt:      job.Prompt,
			TaskType:    job.TaskType,
			Status:      job.Status,
			TaskID:      taskID,
			LockTxHash:  job.LockTxHash,
			ClaimTxHash: job.ClaimTxHash,
			Result:      job.Result,
			Timestamp:   job.Timestamp,
			LatencyMs:   job.LatencyMs,
			LockBlock:   job.LockBlock,
			Amount:      job.Amount,
		})
	}
	logs := append([]string(nil), h.logs...)
	ledger := append([]ledgerRow(nil), h.ledger...)
	return views, logs, ledger
}

func main() {
	_ = godotenv.Load(".env", "../.env")
	port := env("ORCHESTRATOR_PORT", "8081")
	workerURL := env("WORKER_URL", "http://127.0.0.1:8080")
	rpcURL := env("AEVUM_RPC_URL", "https://rpc-test.qday.info")
	chainID := int64(envInt("AEVUM_CHAIN_ID", 44005))
	token := common.HexToAddress(env("TUSD8_ADDRESS", "0x456569838f1d43009A91ebA4263147Afd7233483"))
	escrow := common.HexToAddress(os.Getenv("NEXUS_ESCROW_ADDRESS"))
	principal := common.HexToAddress(os.Getenv("PRINCIPAL_ADDRESS"))
	agentKey := os.Getenv("ORCHESTRATOR_AGENT_PRIVATE_KEY")
	perTask := envBig("PER_TASK_LIMIT", 1_000_000)
	hourly := envBig("HOURLY_LIMIT", 10_000_000)

	state := &hub{}
	budget := x402.NewBudget(perTask, hourly)
	var chain *aevum.Client
	if agentKey != "" && escrow != (common.Address{}) && principal != (common.Address{}) {
		client, err := aevum.Dial(rpcURL, chainID, agentKey, escrow, principal)
		if err != nil {
			log.Printf("aevum dial failed: %v", err)
		} else {
			chain = client
			defer chain.Close()
			state.addLog("session key " + chain.AgentAddress().Hex() + " connected to Aevum")
		}
	} else {
		state.addLog("orchestrator waiting for agent key, principal, and escrow address")
	}

	jobs := make(chan *types.TaskJob, 32)
	for worker := 0; worker < 4; worker++ {
		go consume(jobs, state, workerURL, token, chain, budget)
	}

	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})
	router.POST("/api/jobs", func(c *gin.Context) {
		var request jobRequest
		if err := c.ShouldBindJSON(&request); err != nil || request.Prompt == "" || request.TaskType == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prompt and task_type are required"})
			return
		}
		job := &types.TaskJob{
			ID:        uuid.NewString(),
			Prompt:    request.Prompt,
			TaskType:  request.TaskType,
			Status:    "PENDING",
			Timestamp: time.Now().Unix(),
		}
		state.mu.Lock()
		state.jobs = append(state.jobs, job)
		if len(state.jobs) > 100 {
			state.jobs = state.jobs[len(state.jobs)-100:]
		}
		state.mu.Unlock()
		state.addLog("[QUEUE] " + job.ID + " " + request.TaskType)
		jobs <- job
		c.JSON(http.StatusAccepted, gin.H{"id": job.ID, "status": job.Status})
	})
	router.GET("/api/jobs", func(c *gin.Context) {
		views, _, _ := state.snapshot()
		c.JSON(http.StatusOK, gin.H{"jobs": views})
	})
	router.GET("/api/telemetry", func(c *gin.Context) {
		spent, limit := budget.Snapshot()
		_, logs, ledger := state.snapshot()
		agent := ""
		connected := false
		if chain != nil {
			agent = chain.AgentAddress().Hex()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			connected = chain.Connected(ctx)
			cancel()
		}
		c.JSON(http.StatusOK, gin.H{
			"spent_in_window": spent.String(),
			"hourly_limit":    limit.String(),
			"per_task_limit":  perTask.String(),
			"agent":           agent,
			"principal":       principal.Hex(),
			"connected":       connected,
			"chain_id":        chainID,
			"worker_url":      workerURL,
			"logs":            logs,
			"ledger":          ledger,
		})
	})

	log.Printf("orchestrator listening on :%s", port)
	if err := router.Run("0.0.0.0:" + port); err != nil {
		log.Fatal(err)
	}
}

func consume(jobs <-chan *types.TaskJob, state *hub, workerURL string, token common.Address, chain *aevum.Client, budget *x402.Budget) {
	for job := range jobs {
		if chain == nil {
			finish(state, job, &types.TaskJob{Status: "FAILED", Result: "aevum client is not configured"})
			continue
		}
		settled, err := x402.DispatchTask(workerURL, job.Prompt, job.TaskType, token, chain, budget, state.addLog, func(status string, taskID [32]byte, lockTx, amount string) {
			state.mu.Lock()
			job.Status = status
			job.TaskId = taskID
			job.LockTxHash = lockTx
			job.Amount = amount
			state.mu.Unlock()
		})
		if err != nil {
			if settled == nil {
				settled = &types.TaskJob{Status: "FAILED", Result: err.Error()}
			}
			state.addLog("[FAIL] " + err.Error())
		}
		if settled.LockTxHash != "" {
			state.mu.Lock()
			state.ledger = append(state.ledger, ledgerRow{
				Kind:   "Escrow Lock",
				TxHash: settled.LockTxHash,
				Amount: settled.Amount,
				Status: "Confirmed",
				Block:  chain.Block(settled.LockTxHash),
			})
			state.mu.Unlock()
			settled.LockBlock = chain.Block(settled.LockTxHash)
		}
		if settled.ClaimTxHash != "" {
			state.mu.Lock()
			state.ledger = append(state.ledger, ledgerRow{
				Kind:   "Proof Claim",
				TxHash: settled.ClaimTxHash,
				Amount: settled.Amount,
				Status: "Confirmed",
			})
			state.mu.Unlock()
		}
		finish(state, job, settled)
	}
}

func finish(state *hub, job *types.TaskJob, settled *types.TaskJob) {
	state.mu.Lock()
	defer state.mu.Unlock()
	job.Status = settled.Status
	job.TaskId = settled.TaskId
	job.LockTxHash = settled.LockTxHash
	job.ClaimTxHash = settled.ClaimTxHash
	job.Result = settled.Result
	job.LatencyMs = settled.LatencyMs
	job.LockBlock = settled.LockBlock
	job.Amount = settled.Amount
	if job.LatencyMs == 0 && job.Timestamp > 0 {
		job.LatencyMs = time.Since(time.Unix(job.Timestamp, 0)).Milliseconds()
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBig(key string, fallback int64) *big.Int {
	value := os.Getenv(key)
	if value == "" {
		return big.NewInt(fallback)
	}
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return big.NewInt(fallback)
	}
	return parsed
}
