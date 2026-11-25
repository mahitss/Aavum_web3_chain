package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/nexus402/agent-facilitator/internal/chain"
	"github.com/nexus402/agent-facilitator/internal/httpapi"
)

func main() {
	_ = godotenv.Load()
	rpcURL := env("AEVUM_RPC_URL", "https://rpc-test.qday.info")
	port := env("FACILITATOR_PORT", "8082")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	client, err := chain.Dial(ctx, rpcURL, os.Getenv("NEXUS_ESCROW_ADDRESS"), os.Getenv("FACILITATOR_PRIVATE_KEY"))
	cancel()
	if err != nil {
		log.Fatalf("facilitator chain dial failed: %v", err)
	}
	if id, err := client.ChainID(context.Background()); err == nil {
		log.Printf("Facilitator connected to chain %s", id.String())
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           (&httpapi.Server{Chain: client}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("x402 facilitator listening on http://0.0.0.0:%s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
