package main

import (
	"context"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/nexus402/agent-orchestrator/pkg/aevum"
	"github.com/nexus402/agent-orchestrator/pkg/types"
	"github.com/nexus402/agent-orchestrator/pkg/x402"
)

type Server struct {
	aevumClient *aevum.Client
	x402Client  *x402.Client
	jobsChan    chan *types.TaskJob
	jobsMu      sync.RWMutex
	jobsHistory []*types.TaskJob
	hourlySpent *big.Int
	hourlyLimit *big.Int
	perTaskCap  *big.Int
}

func main() {
	_ = godotenv.Load()

	rpcURL := getEnv("AEVUM_RPC_URL", "https://rpc-test.qday.info")
	privKey := os.Getenv("ORCHESTRATOR_AGENT_PRIVATE_KEY")
	if privKey == "" {
		log.Fatal("ORCHESTRATOR_AGENT_PRIVATE_KEY environment variable required")
	}
	escrowAddr := os.Getenv("NEXUS_ESCROW_ADDRESS")
	if escrowAddr == "" {
		log.Fatal("NEXUS_ESCROW_ADDRESS environment variable required")
	}
	principalAddr := os.Getenv("PRINCIPAL_ADDRESS")
	if principalAddr == "" {
		log.Fatal("PRINCIPAL_ADDRESS environment variable required")
	}

	workerURL := getEnv("WORKER_URL", "http://localhost:8080")
	port := getEnv("ORCHESTRATOR_PORT", "8081")
	perTaskLimit := envInt("PER_TASK_LIMIT", 1_000_000)
	hourlyLimit := envInt("HOURLY_LIMIT", 10_000_000)

	log.Println("Initializing Nexus402 Go Orchestrator Client...")
	aevumClient, err := aevum.NewClient(rpcURL, privKey, escrowAddr, principalAddr, 44005)
	if err != nil {
		log.Fatalf("Failed to initialize Aevum client: %v", err)
	}
	x402Client := x402.NewClient(workerURL, aevumClient, perTaskLimit, hourlyLimit)

	server := &Server{
		aevumClient: aevumClient,
		x402Client:  x402Client,
		jobsChan:    make(chan *types.TaskJob, 100),
		jobsHistory: make([]*types.TaskJob, 0),
		hourlySpent: big.NewInt(0),
		hourlyLimit: hourlyLimit,
		perTaskCap:  perTaskLimit,
	}

	for i := 1; i <= 4; i++ {
		go server.workerRoutine(i)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:  []string{"*"},
		AllowMethods:  []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:  []string{"Origin", "Content-Type", "Accept"},
		ExposeHeaders: []string{"Content-Length"},
		MaxAge:        12 * time.Hour,
	}))

	router.GET("/health", server.handleHealth)
	router.GET("/api/telemetry", server.handleTelemetry)
	router.GET("/api/jobs", server.handleGetJobs)
	router.POST("/api/jobs", server.handleCreateJob)

	log.Printf("Orchestrator Agent listening on http://0.0.0.0:%s", port)
	log.Printf("Agent Address: %s", aevumClient.AgentAddress().Hex())
	log.Printf("Principal Wallet: %s", aevumClient.PrincipalAddress().Hex())

	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Server stopped: %v", err)
	}
}

func (s *Server) workerRoutine(id int) {
	for job := range s.jobsChan {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		err := s.x402Client.ExecuteTask(ctx, job)
		cancel()

		s.jobsMu.Lock()
		if err != nil {
			job.Status = "FAILED"
			job.Error = err.Error()
			log.Printf("[Worker-%d] Task %s failed: %v", id, job.ID, err)
		} else {
			log.Printf("[Worker-%d] Task %s SETTLED successfully. TxHash: %s", id, job.ID, job.ClaimTxHash)
		}
		s.hourlySpent = s.x402Client.Spent()
		s.jobsMu.Unlock()
	}
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"agent":   s.aevumClient.AgentAddress().Hex(),
		"network": "QDay Aevum Testnet (#44005)",
	})
}

func (s *Server) handleTelemetry(c *gin.Context) {
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()

	settled := 0
	failed := 0
	for _, job := range s.jobsHistory {
		switch job.Status {
		case "SETTLED":
			settled++
		case "FAILED":
			failed++
		}
	}

	c.JSON(http.StatusOK, types.TelemetryStats{
		AgentAddress:   s.aevumClient.AgentAddress().Hex(),
		Principal:      s.aevumClient.PrincipalAddress().Hex(),
		EscrowContract: os.Getenv("NEXUS_ESCROW_ADDRESS"),
		TotalJobs:      len(s.jobsHistory),
		SettledJobs:    settled,
		FailedJobs:     failed,
		HourlySpent:    new(big.Int).Set(s.hourlySpent),
		HourlyLimit:    s.hourlyLimit,
		PerTaskLimit:   s.perTaskCap,
		ChainID:        44005,
		Network:        "QDay Aevum Testnet",
	})
}

func (s *Server) handleGetJobs(c *gin.Context) {
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()

	limit := 20
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	total := len(s.jobsHistory)
	start := 0
	if total > limit {
		start = total - limit
	}
	c.JSON(http.StatusOK, s.jobsHistory[start:])
}

func (s *Server) handleCreateJob(c *gin.Context) {
	var req types.TaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	job := &types.TaskJob{
		ID:        uuid.New().String(),
		Prompt:    req.Prompt,
		TaskType:  req.TaskType,
		Status:    "PENDING",
		CreatedAt: time.Now().UTC(),
	}

	s.jobsMu.Lock()
	s.jobsHistory = append(s.jobsHistory, job)
	s.jobsMu.Unlock()

	s.jobsChan <- job
	c.JSON(http.StatusAccepted, job)
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int64) *big.Int {
	raw := os.Getenv(key)
	if raw == "" {
		return big.NewInt(fallback)
	}
	parsed, ok := new(big.Int).SetString(raw, 10)
	if !ok || parsed.Sign() < 0 {
		return big.NewInt(fallback)
	}
	return parsed
}
