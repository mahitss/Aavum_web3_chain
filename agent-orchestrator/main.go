package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
)

type Config struct {
	RPC           string
	ChainID       int64
	Escrow        common.Address
	SessionKey    string
	WorkerURL     string
	PerTaskLimit  *big.Int
	HourlyLimit   *big.Int
	Bind          string
	Token         common.Address
}

type LogLine struct {
	ID     int    `json:"id"`
	Stream string `json:"stream"`
	Text   string `json:"text"`
	Tx     string `json:"tx,omitempty"`
}

type LogBook struct {
	mu    sync.Mutex
	next  int
	lines []LogLine
}

func (b *LogBook) Add(stream, text, tx string) LogLine {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	line := LogLine{ID: b.next, Stream: stream, Text: text, Tx: tx}
	b.lines = append(b.lines, line)
	if len(b.lines) > 400 {
		b.lines = b.lines[len(b.lines)-400:]
	}
	return line
}

func (b *LogBook) Since(id int) []LogLine {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]LogLine, 0)
	for _, line := range b.lines {
		if line.ID > id {
			out = append(out, line)
		}
	}
	return out
}

type Server struct {
	cfg      Config
	chain    *Chain
	velocity *Velocity
	logs     *LogBook
	client   *http.Client
}

func main() {
	loadEnvFile(".env")
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	chain, err := DialChain(ctx, cfg.RPC, cfg.SessionKey, cfg.Escrow, cfg.ChainID)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("session agent %s", chain.from.Hex())
	log.Printf("escrow %s chain %d", cfg.Escrow.Hex(), cfg.ChainID)

	server := &Server{
		cfg:      cfg,
		chain:    chain,
		velocity: NewVelocity(cfg.HourlyLimit),
		logs:     &LogBook{},
		client:   &http.Client{Timeout: 120 * time.Second},
	}

	router := gin.New()
	router.Use(gin.Recovery(), cors())
	router.GET("/health", server.health)
	router.GET("/api/v1/logs", server.getLogs)
	router.POST("/api/v1/tasks", server.dispatch)
	if err := router.Run(cfg.Bind); err != nil {
		log.Fatal(err)
	}
}

func (s *Server) health(c *gin.Context) {
	worker := "down"
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, strings.TrimRight(s.cfg.WorkerURL, "/")+"/health", nil)
	if err == nil {
		resp, err := s.client.Do(req)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				worker = "ok"
			}
			resp.Body.Close()
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"status":       "ok",
		"sessionAgent": s.chain.from.Hex(),
		"escrow":       s.cfg.Escrow.Hex(),
		"worker":       worker,
		"chainId":      s.cfg.ChainID,
		"perTaskLimit": s.cfg.PerTaskLimit.String(),
		"hourlyLimit":  s.cfg.HourlyLimit.String(),
	})
}

func (s *Server) getLogs(c *gin.Context) {
	since, _ := strconv.Atoi(c.Query("since"))
	c.JSON(http.StatusOK, gin.H{"lines": s.logs.Since(since)})
}

func (s *Server) dispatch(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil || len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task payload required"})
		return
	}
	s.logs.Add("http", "dispatching compute task to worker", "")

	workerURL := strings.TrimRight(s.cfg.WorkerURL, "/") + "/api/v1/compute"
	first, err := s.postWorker(c.Request.Context(), workerURL, body, "")
	if err != nil {
		s.logs.Add("http", "worker request failed: "+err.Error(), "")
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer first.Body.Close()
	if first.StatusCode != http.StatusPaymentRequired {
		raw, _ := io.ReadAll(first.Body)
		s.logs.Add("http", fmt.Sprintf("worker returned %d before payment", first.StatusCode), "")
		c.JSON(http.StatusBadGateway, gin.H{"error": string(raw), "status": first.StatusCode})
		return
	}

	challenge, err := ParseChallenge(first.Header, s.cfg.Token)
	if err != nil {
		s.logs.Add("http", "rejected 402 challenge: "+err.Error(), "")
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if err := VerifyBodyHash(body, challenge.TaskHash); err != nil {
		s.logs.Add("http", err.Error(), "")
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if err := CheckLimits(challenge.Amount, s.cfg.PerTaskLimit, s.velocity.Remaining()); err != nil {
		s.logs.Add("http", "fee outside session limits: "+err.Error(), "")
		c.JSON(http.StatusPaymentRequired, gin.H{"error": err.Error()})
		return
	}
	if err := s.velocity.Commit(challenge.Amount); err != nil {
		s.logs.Add("http", err.Error(), "")
		c.JSON(http.StatusPaymentRequired, gin.H{"error": err.Error()})
		return
	}

	s.logs.Add("http", fmt.Sprintf("HTTP 402 Nexus402-v1 amount %s receiver %s", challenge.Amount, challenge.Receiver.Hex()), "")

	var taskID common.Hash
	if _, err := rand.Read(taskID[:]); err != nil {
		s.velocity.Release(challenge.Amount)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "task id"})
		return
	}

	lockCtx, cancel := context.WithTimeout(c.Request.Context(), 70*time.Second)
	lockTx, err := s.chain.Lock(lockCtx, taskID, challenge.Receiver, challenge.Amount, challenge.TimeoutBlocks, challenge.TaskHash)
	cancel()
	if err != nil {
		s.velocity.Release(challenge.Amount)
		s.logs.Add("chain", "lockTaskEscrow failed: "+err.Error(), lockTx.Hex())
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "lockTx": emptyHash(lockTx)})
		return
	}
	s.logs.Add("chain", "locked tUSD8 escrow", lockTx.Hex())
	s.logs.Add("http", "retrying worker with X-Escrow-TaskId "+taskID.Hex(), "")

	second, err := s.postWorker(c.Request.Context(), workerURL, body, taskID.Hex())
	if err != nil {
		s.logs.Add("http", "funded retry failed: "+err.Error(), "")
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "taskId": taskID.Hex(), "lockTx": lockTx.Hex()})
		return
	}
	defer second.Body.Close()
	raw, _ := io.ReadAll(second.Body)
	if second.StatusCode != http.StatusOK {
		s.logs.Add("http", fmt.Sprintf("worker claim failed (%d): %s", second.StatusCode, strings.TrimSpace(string(raw))), "")
		c.JSON(second.StatusCode, gin.H{
			"error":  strings.TrimSpace(string(raw)),
			"taskId": taskID.Hex(),
			"lockTx": lockTx.Hex(),
		})
		return
	}

	var claimed struct {
		Result     json.RawMessage `json:"result"`
		ResultHash string          `json:"resultHash"`
		ClaimTx    string          `json:"claimTx"`
	}
	if err := json.Unmarshal(raw, &claimed); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "worker returned invalid json", "taskId": taskID.Hex(), "lockTx": lockTx.Hex()})
		return
	}
	s.logs.Add("chain", "worker claimed escrow", claimed.ClaimTx)
	s.logs.Add("http", "compute result accepted", "")
	c.JSON(http.StatusOK, gin.H{
		"taskId":     taskID.Hex(),
		"lockTx":     lockTx.Hex(),
		"claimTx":    claimed.ClaimTx,
		"result":     claimed.Result,
		"resultHash": claimed.ResultHash,
	})
}

func (s *Server) postWorker(ctx context.Context, url string, body []byte, taskID string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if taskID != "" {
		req.Header.Set("X-Escrow-TaskId", taskID)
	}
	return s.client.Do(req)
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-Escrow-TaskId")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func loadConfig() (Config, error) {
	chainID, err := strconv.ParseInt(env("CHAIN_ID", "44005"), 10, 64)
	if err != nil {
		return Config{}, fmt.Errorf("CHAIN_ID: %w", err)
	}
	perTask, ok := new(big.Int).SetString(env("PER_TASK_LIMIT", "1000000"), 10)
	if !ok {
		return Config{}, fmt.Errorf("PER_TASK_LIMIT")
	}
	hourly, ok := new(big.Int).SetString(env("HOURLY_LIMIT", "10000000"), 10)
	if !ok {
		return Config{}, fmt.Errorf("HOURLY_LIMIT")
	}
	escrowRaw := strings.TrimSpace(os.Getenv("ESCROW_ADDRESS"))
	key := strings.TrimSpace(os.Getenv("SESSION_PRIVATE_KEY"))
	if escrowRaw == "" || key == "" {
		return Config{}, fmt.Errorf("ESCROW_ADDRESS and SESSION_PRIVATE_KEY are required")
	}
	return Config{
		RPC:          env("RPC_URL", "https://rpc-test.qday.info"),
		ChainID:      chainID,
		Escrow:       common.HexToAddress(escrowRaw),
		SessionKey:   key,
		WorkerURL:    env("WORKER_URL", "http://127.0.0.1:4020"),
		PerTaskLimit: perTask,
		HourlyLimit:  hourly,
		Bind:         env("BIND_ADDR", "0.0.0.0:8080"),
		Token:        common.HexToAddress(env("TUSD8_ADDRESS", "0x456569838f1d43009A91ebA4263147Afd7233483")),
	}, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func emptyHash(hash common.Hash) string {
	if hash == (common.Hash{}) {
		return ""
	}
	return hash.Hex()
}

func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}
