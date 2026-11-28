package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/nexus402/agent-orchestrator/pkg/aevum"
	"github.com/nexus402/agent-orchestrator/pkg/discovery"
	"github.com/nexus402/agent-orchestrator/pkg/sandbox"
	"github.com/nexus402/agent-orchestrator/pkg/types"
	"github.com/nexus402/agent-orchestrator/pkg/webhooks"
	"github.com/nexus402/agent-orchestrator/pkg/x402"
)

var sseHeartbeat = 15 * time.Second

type Server struct {
	aevumClient    *aevum.Client
	x402Client     *x402.Client
	jobsChan       chan *types.TaskJob
	jobsMu         sync.RWMutex
	jobsHistory    []*types.TaskJob
	hourlySpent    *big.Int
	hourlyLimit    *big.Int
	perTaskCap     *big.Int
	events         *eventHub
	stop           chan struct{}
	acceptMu       sync.Mutex
	workers        sync.WaitGroup
	sandboxEnabled bool
	sandboxLimit   *sandbox.Limiter
	watchdog       *aevum.Watchdog
	directory      *discovery.Directory
	chainLedger    []aevum.ChainLedgerEntry
	receipts       map[string]x402.ReceiptDocument
	receiptMu      sync.RWMutex
	webhooks       *webhooks.Registry
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
		aevumClient:    aevumClient,
		x402Client:     x402Client,
		jobsChan:       make(chan *types.TaskJob, 100),
		events:         newEventHub(),
		stop:           make(chan struct{}),
		jobsHistory:    make([]*types.TaskJob, 0),
		hourlySpent:    big.NewInt(0),
		hourlyLimit:    hourlyLimit,
		perTaskCap:     perTaskLimit,
		sandboxEnabled: envBool("ENABLE_PUBLIC_SANDBOX"),
		sandboxLimit:   sandbox.NewLimiter(envPositive("SANDBOX_MAX_JOBS_PER_IP_HOUR", 5), time.Hour),
		watchdog:       aevum.NewWatchdog(aevumClient),
		directory:      discovery.New(uint64(envPositive("MIN_RELIABILITY_BPS", 8000))),
		receipts:       map[string]x402.ReceiptDocument{},
		webhooks:       webhooks.New(envBool("WEBHOOK_ALLOW_PRIVATE")),
	}
	hydrateCtx, hydrateCancel := context.WithTimeout(context.Background(), 45*time.Second)
	chainLedger, hydrateErr := aevumClient.HydrateLedger(hydrateCtx)
	hydrateCancel()
	if hydrateErr != nil {
		log.Printf("On-chain ledger hydration failed: %v", hydrateErr)
		chainLedger = []aevum.ChainLedgerEntry{}
	} else {
		log.Printf("Hydrated %d on-chain escrow events", len(chainLedger))
	}
	server.chainLedger = chainLedger

	workerURLs := getEnv("WORKER_URLS", workerURL)
	refreshCtx, refreshCancel := context.WithTimeout(context.Background(), 8*time.Second)
	server.directory.Refresh(refreshCtx, workerURLs)
	refreshCancel()
	sigCtx, stopSig := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSig()
	server.watchdog.OnProgress = server.onCountdown
	go server.watchdog.Run(sigCtx)

	for i := 1; i <= 4; i++ {
		server.workers.Add(1)
		go server.workerRoutine(i)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins(),
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	publicGate := sandbox.Gate(server.sandboxEnabled, server.sandboxLimit)
	router.GET("/health", server.handleHealth)
	router.GET("/metrics", server.handleMetrics)
	router.GET("/api/telemetry", server.handleTelemetry)
	router.GET("/api/sandbox/status", server.handleSandboxStatus)
	router.GET("/api/services", server.handleServices)
	router.GET("/api/ledger/onchain", server.handleOnchainLedger)
	router.GET("/api/ledger/export", server.handleLedgerExport)
	router.POST("/api/webhooks/register", publicGate, server.handleRegisterWebhook)
	router.GET("/api/receipts/:taskId", server.handleReceipt)
	router.GET("/api/jobs", server.handleGetJobs)
	router.POST("/api/jobs", publicGate, server.handleCreateJob)
	router.POST("/api/scenarios/run", publicGate, server.handleScenario)
	router.GET("/api/events", server.handleEvents)
	x402Client.Notify = func(job *types.TaskJob) {
		server.publishJob(job)
		if job.Status == "SETTLED" {
			server.watchdog.Forget(job.TaskId)
		}
		if job.Status == "ESCROW_LOCKED" && job.TaskId != "" {
			taskID, err := hexTask(job.TaskId)
			if err == nil {
				server.watchdog.Track(taskID, job.TaskId, job.Deadline, server.onRefund)
			}
		}
	}

	httpServer := &http.Server{Addr: ":" + port, Handler: router}
	go func() {
		log.Printf("Orchestrator Agent listening on http://0.0.0.0:%s", port)
		log.Printf("Agent Address: %s", aevumClient.AgentAddress().Hex())
		log.Printf("Principal Wallet: %s", aevumClient.PrincipalAddress().Hex())
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server stopped: %v", err)
		}
	}()

	<-sigCtx.Done()
	log.Printf("Shutting down. Draining queued tasks.")
	close(server.stop)
	server.acceptMu.Lock()
	close(server.jobsChan)
	server.acceptMu.Unlock()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown: %v", err)
	}
	server.workers.Wait()
}

func (s *Server) workerRoutine(id int) {
	defer s.workers.Done()
	for job := range s.jobsChan {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		err := s.x402Client.ExecuteTask(ctx, job)
		cancel()

		s.jobsMu.Lock()
		if err != nil {
			s.markFailed(job, err)
			log.Printf("[Worker-%d] Task %s failed: %v", id, job.ID, err)
			s.publishJob(job)
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

	identity := os.Getenv("NEXUS_IDENTITY_REGISTRY_ADDRESS")
	if identity == "" {
		identity = os.Getenv("NEXUS_IDENTITY_ADDRESS")
	}
	c.JSON(http.StatusOK, types.TelemetryStats{
		AgentAddress:     s.aevumClient.AgentAddress().Hex(),
		Principal:        s.aevumClient.PrincipalAddress().Hex(),
		EscrowContract:   os.Getenv("NEXUS_ESCROW_ADDRESS"),
		IdentityRegistry: identity,
		TotalJobs:        len(s.jobsHistory),
		SettledJobs:      settled,
		FailedJobs:       failed,
		HourlySpent:      new(big.Int).Set(s.hourlySpent),
		HourlyLimit:      s.hourlyLimit,
		PerTaskLimit:     s.perTaskCap,
		ChainID:          44005,
		Network:          "QDay Aevum Testnet",
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

	service := req.Service
	if service == "" {
		service = req.TaskType
	}
	scheme := req.Scheme
	if req.Evaluator && scheme == "" {
		scheme = "auth-capture"
	}
	job := &types.TaskJob{
		ID:        uuid.New().String(),
		Prompt:    req.Prompt,
		TaskType:  req.TaskType,
		Service:   service,
		Status:    "PENDING",
		CreatedAt: time.Now().UTC(),
		Scheme:    scheme,
		Evaluator: req.Evaluator || scheme == "auth-capture",
	}

	s.jobsMu.Lock()
	s.jobsHistory = append(s.jobsHistory, job)
	s.jobsMu.Unlock()

	s.acceptMu.Lock()
	select {
	case <-s.stop:
		s.acceptMu.Unlock()
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "orchestrator is shutting down"})
		return
	default:
		s.jobsChan <- job
	}
	s.acceptMu.Unlock()
	c.JSON(http.StatusAccepted, job)
}

func (s *Server) markFailed(job *types.TaskJob, err error) {
	job.Error = err.Error()
	switch {
	case strings.Contains(err.Error(), "ExceedsHourlyLimit"):
		job.Status = "BLOCKED_HOURLY_CAP"
	case strings.Contains(err.Error(), "SessionInactive"):
		job.Status = "BLOCKED_REVOKED"
	case strings.Contains(err.Error(), "504"):
		job.Status = "AWAITING_REFUND"
	default:
		job.Status = "FAILED"
	}
}

func (s *Server) onCountdown(taskID string, head uint64, deadline uint64) {
	s.jobsMu.Lock()
	var job *types.TaskJob
	for _, candidate := range s.jobsHistory {
		if candidate.TaskId == taskID {
			job = candidate
			break
		}
	}
	if job == nil || job.Status == "SETTLED" || job.Status == "REFUNDED" {
		s.jobsMu.Unlock()
		return
	}
	remaining := uint64(0)
	if deadline > head {
		remaining = deadline - head
	}
	job.BlocksRemaining = remaining
	job.Event = "BLOCK_COUNTDOWN"
	s.jobsMu.Unlock()
	s.publishJob(job)
}

func (s *Server) onRefund(taskID string, refundTx string, err error) {
	s.jobsMu.Lock()
	var job *types.TaskJob
	for _, candidate := range s.jobsHistory {
		if candidate.TaskId == taskID {
			job = candidate
			break
		}
	}
	if job == nil {
		s.jobsMu.Unlock()
		return
	}
	if err != nil {
		job.Error = err.Error()
		s.jobsMu.Unlock()
		s.publishJob(job)
		return
	}
	job.Status = "REFUNDED"
	job.RefundTxHash = refundTx
	job.Event = "TASK_REFUNDED"
	s.jobsMu.Unlock()
	s.publishJob(job)
}

func hexTask(value string) ([32]byte, error) {
	var taskID [32]byte
	raw, err := hex.DecodeString(strings.TrimPrefix(value, "0x"))
	if err != nil || len(raw) != 32 {
		return taskID, fmt.Errorf("invalid task id")
	}
	copy(taskID[:], raw)
	return taskID, nil
}

func (s *Server) remember(job *types.TaskJob) {
	s.jobsMu.Lock()
	s.jobsHistory = append(s.jobsHistory, job)
	s.jobsMu.Unlock()
	s.publishJob(job)
}

func (s *Server) runDirect(job *types.TaskJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	err := s.x402Client.ExecuteTask(ctx, job)
	cancel()
	s.jobsMu.Lock()
	if err != nil {
		s.markFailed(job, err)
	}
	s.hourlySpent = s.x402Client.Spent()
	s.jobsMu.Unlock()
	if err != nil {
		s.publishJob(job)
	}
}

func (s *Server) handleScenario(c *gin.Context) {
	var body struct {
		Scenario string `json:"scenario"`
		Prompt   string `json:"prompt"`
		Target   string `json:"target"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	switch body.Scenario {
	case "happy_path":
		prompt := body.Prompt
		service := "ai_synthesis"
		if body.Target != "" {
			prompt = body.Target
			service = "chain_audit"
		}
		if prompt == "" {
			prompt = "Explain why a capped session key is safer than an unlimited token approval."
		}
		job := s.queueJob(prompt, service, false, false)
		c.JSON(http.StatusAccepted, gin.H{"scenario": body.Scenario, "job_id": job.ID})
	case "velocity_attack":
		go s.runVelocity()
		c.JSON(http.StatusAccepted, gin.H{"scenario": body.Scenario, "status": "running"})
	case "kill_switch_test":
		job := s.queueJob("Kill switch probe. This lock must revert if the session was revoked.", "ai_synthesis", false, false)
		c.JSON(http.StatusAccepted, gin.H{"scenario": body.Scenario, "job_id": job.ID})
	case "worker_timeout_refund":
		job := s.queueJob("https://example.com", "web_brief", false, true)
		c.JSON(http.StatusAccepted, gin.H{"scenario": body.Scenario, "job_id": job.ID})
	case "voucher_burst":
		go s.runVoucherBurst()
		c.JSON(http.StatusAccepted, gin.H{"scenario": body.Scenario, "status": "running"})
	case "evaluator_dispute_slash":
		go s.runEvaluatorDispute()
		c.JSON(http.StatusAccepted, gin.H{"scenario": body.Scenario, "status": "running"})
	case "multi_hop_swarm":
		workerURL, _ := s.directory.Endpoint("deep_due_diligence")
		job := &types.TaskJob{
			ID:        uuid.New().String(),
			Prompt:    "Audit 0x456569838f1d43009A91ebA4263147Afd7233483 and brief https://explorer-test.qday.info",
			TaskType:  "deep_due_diligence",
			Service:   "deep_due_diligence",
			Status:    "PENDING",
			CreatedAt: time.Now().UTC(),
			WorkerURL: workerURL,
		}
		s.enqueue(job)
		c.JSON(http.StatusAccepted, gin.H{"scenario": body.Scenario, "job_id": job.ID})
	case "metered_batch":
		prompts := []string{
			"Name one risk of an unlimited token approval.",
			"Name one benefit of a one-hour session cap.",
			"Name one reason a worker should prove execution.",
		}
		workerURL, _ := s.directory.Endpoint("ai_synthesis")
		job := &types.TaskJob{
			ID:        uuid.New().String(),
			Prompt:    strings.Join(prompts, "\n"),
			TaskType:  "ai_synthesis",
			Service:   "ai_synthesis",
			Status:    "PENDING",
			CreatedAt: time.Now().UTC(),
			Scheme:    "upto",
			Batch:     prompts,
			WorkerURL: workerURL,
		}
		s.enqueue(job)
		c.JSON(http.StatusAccepted, gin.H{"scenario": body.Scenario, "job_id": job.ID})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "scenario must be happy_path, velocity_attack, kill_switch_test, worker_timeout_refund, metered_batch, multi_hop_swarm, voucher_burst, or evaluator_dispute_slash"})
	}
}

func (s *Server) handleOnchainLedger(c *gin.Context) {
	events := s.chainLedger
	if events == nil {
		events = []aevum.ChainLedgerEntry{}
	}
	c.JSON(http.StatusOK, gin.H{"events": events})
}

func (s *Server) handleReceipt(c *gin.Context) {
	taskID := strings.ToLower(strings.TrimSpace(c.Param("taskId")))
	s.receiptMu.RLock()
	doc, ok := s.receipts[taskID]
	s.receiptMu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "receipt not found"})
		return
	}
	c.Header("Content-Disposition", "inline")
	c.JSON(http.StatusOK, doc)
}

func (s *Server) handleServices(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	s.directory.Refresh(ctx, getEnv("WORKER_URLS", getEnv("WORKER_URL", "http://localhost:8080")))
	for _, worker := range s.directory.List() {
		if !common.IsHexAddress(worker.Worker) {
			continue
		}
		completed, refunded, volume, bps, err := s.aevumClient.WorkerReputation(ctx, common.HexToAddress(worker.Worker))
		if err != nil {
			s.directory.SetReputation(worker.URL, 0, 0, "0", 0, false)
			continue
		}
		s.directory.SetReputation(worker.URL, completed, refunded, volume, bps, true)
	}
	if identity := os.Getenv("NEXUS_IDENTITY_ADDRESS"); common.IsHexAddress(identity) {
		registry := common.HexToAddress(identity)
		for _, worker := range s.directory.List() {
			if !common.IsHexAddress(worker.Worker) {
				continue
			}
			agentID, err := s.aevumClient.AgentPassport(ctx, registry, common.HexToAddress(worker.Worker))
			if err != nil {
				continue
			}
			s.directory.SetIdentity(worker.URL, agentID, registry.Hex())
		}
	}
	c.JSON(http.StatusOK, gin.H{"workers": s.directory.List()})
}

func (s *Server) queueJob(prompt string, service string, bypass bool, simulate bool) *types.TaskJob {
	workerURL, _ := s.directory.Endpoint(service)
	job := &types.TaskJob{
		ID:              uuid.New().String(),
		Prompt:          prompt,
		TaskType:        service,
		Service:         service,
		Status:          "PENDING",
		CreatedAt:       time.Now().UTC(),
		BypassLocalCap:  bypass,
		SimulateTimeout: simulate,
		WorkerURL:       workerURL,
	}
	s.enqueue(job)
	return job
}

func (s *Server) enqueue(job *types.TaskJob) {
	s.jobsMu.Lock()
	s.jobsHistory = append(s.jobsHistory, job)
	s.jobsMu.Unlock()
	s.acceptMu.Lock()
	select {
	case <-s.stop:
	default:
		s.jobsChan <- job
	}
	s.acceptMu.Unlock()
}

func (s *Server) runVelocity() {
	for n := 1; n <= 48; n++ {
		job := &types.TaskJob{
			ID:             uuid.New().String(),
			Prompt:         fmt.Sprintf("Velocity probe %d. Repeat spend until the hourly cap reverts.", n),
			TaskType:       "ai_synthesis",
			Service:        "ai_synthesis",
			Status:         "PENDING",
			CreatedAt:      time.Now().UTC(),
			BypassLocalCap: true,
		}
		s.remember(job)
		s.runDirect(job)
		if job.Status == "BLOCKED_HOURLY_CAP" || job.Status == "BLOCKED_REVOKED" {
			return
		}
		if strings.Contains(job.Error, "insufficient funds") {
			return
		}
	}
}

func (s *Server) publishJob(job *types.TaskJob) {
	s.events.broadcast(job)
	if s.webhooks != nil {
		s.webhooks.Dispatch(job)
	}
}

func (s *Server) handleRegisterWebhook(c *gin.Context) {
	var body struct {
		URL    string   `json:"url"`
		Secret string   `json:"secret"`
		Events []string `json:"events"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.webhooks.Register(body.URL, body.Secret, body.Events); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"registered": true, "events": body.Events})
}

func (s *Server) handleLedgerExport(c *gin.Context) {
	format := c.DefaultQuery("format", "json")
	rows := s.exportRows()
	switch format {
	case "json":
		c.JSON(http.StatusOK, rows)
	case "csv":
		text, err := encodeCSV(rows)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Disposition", "attachment; filename=nexus402-ledger.csv")
		c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(text))
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "format must be csv or json"})
	}
}

func (s *Server) handleEvents(c *gin.Context) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming is not supported"})
		return
	}
	sub := s.events.subscribe()
	defer s.events.unsubscribe(sub)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Status(http.StatusOK)
	fmt.Fprintf(c.Writer, ": connected\n\n")
	flusher.Flush()
	streamEvents(c.Request.Context(), c.Writer, flusher.Flush, sub, s.stop, sseHeartbeat)
}

func streamEvents(ctx context.Context, w http.ResponseWriter, flush func(), events <-chan string, stop <-chan struct{}, heartbeat time.Duration) {
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flush()
		case payload, open := <-events:
			if !open {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flush()
		}
	}
}

type eventHub struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{subs: map[chan string]struct{}{}}
}

func (h *eventHub) subscribe() chan string {
	ch := make(chan string, 16)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *eventHub) unsubscribe(ch chan string) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *eventHub) broadcast(job *types.TaskJob) {
	payload, err := json.Marshal(job)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- string(payload):
		default:
		}
	}
}

func (s *Server) handleSandboxStatus(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	balances, err := s.aevumClient.SandboxBalances(ctx)
	body := gin.H{
		"sandboxEnabled":        s.sandboxEnabled,
		"remainingIpQuota":      s.sandboxLimit.Remaining(sandbox.ClientKey(c)),
		"principalTusd8Balance": "0",
		"agentGasBalance":       "0",
		"sessionActive":         false,
	}
	if err != nil {
		body["rpcError"] = err.Error()
		c.JSON(http.StatusOK, body)
		return
	}
	body["principalTusd8Balance"] = balances.PrincipalTusd8
	body["agentGasBalance"] = balances.AgentGas
	body["sessionActive"] = balances.SessionActive
	c.JSON(http.StatusOK, body)
}

func allowedOrigins() []string {
	raw := getEnv("ALLOWED_ORIGINS", "http://localhost:3000")
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && part != "*" {
			origins = append(origins, part)
		}
	}
	if len(origins) == 0 {
		return []string{"http://localhost:3000"}
	}
	return origins
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envPositive(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
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
