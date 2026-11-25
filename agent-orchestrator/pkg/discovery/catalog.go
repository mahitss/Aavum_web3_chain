package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Service struct {
	Name          string `json:"name"`
	Price         string `json:"price"`
	PriceUnits    string `json:"priceUnits"`
	TimeoutBlocks uint64 `json:"timeoutBlocks"`
	Input         string `json:"input"`
}

type Manifest struct {
	Worker             string    `json:"worker"`
	Network            string    `json:"network"`
	Protocol           string    `json:"protocol"`
	Services           []Service `json:"services"`
	Healthy            bool      `json:"healthy"`
	URL                string    `json:"url"`
	TasksCompleted     uint64    `json:"tasksCompleted"`
	TasksRefunded      uint64    `json:"tasksRefunded"`
	TotalVolumeSettled string    `json:"totalVolumeSettled"`
	ReliabilityBps     uint64    `json:"reliabilityBps"`
	ReputationKnown    bool      `json:"reputationKnown"`
}

type Directory struct {
	mu      sync.RWMutex
	workers []Manifest
	minBps  uint64
}

func New(minBps uint64) *Directory {
	return &Directory{minBps: minBps}
}

func (d *Directory) Refresh(ctx context.Context, rawURLs string) {
	client := &http.Client{Timeout: 8 * time.Second}
	var found []Manifest
	for _, raw := range strings.Split(rawURLs, ",") {
		base := strings.TrimRight(strings.TrimSpace(raw), "/")
		if base == "" {
			continue
		}
		manifest := Manifest{URL: base}
		healthReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
		if err == nil {
			if response, err := client.Do(healthReq); err == nil {
				manifest.Healthy = response.StatusCode == http.StatusOK
				response.Body.Close()
			}
		}
		catalogReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/.well-known/nexus402.json", nil)
		if err == nil {
			if response, err := client.Do(catalogReq); err == nil {
				_ = json.NewDecoder(response.Body).Decode(&manifest)
				response.Body.Close()
				manifest.URL = base
			}
		}
		found = append(found, manifest)
	}
	d.mu.Lock()
	d.workers = found
	d.mu.Unlock()
}

func (d *Directory) List() []Manifest {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Manifest, len(d.workers))
	copy(out, d.workers)
	return out
}

func (d *Directory) SetReputation(url string, completed uint64, refunded uint64, volume string, bps uint64, known bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for index := range d.workers {
		if d.workers[index].URL == url {
			d.workers[index].TasksCompleted = completed
			d.workers[index].TasksRefunded = refunded
			d.workers[index].TotalVolumeSettled = volume
			d.workers[index].ReliabilityBps = bps
			d.workers[index].ReputationKnown = known
		}
	}
}

func (d *Directory) Endpoint(service string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	bestURL := ""
	bestScore := uint64(0)
	found := false
	for _, worker := range d.workers {
		if !worker.Healthy || !offers(worker, service) {
			continue
		}
		if worker.ReputationKnown && worker.ReliabilityBps < d.minBps {
			continue
		}
		score := uint64(10_000)
		if worker.ReputationKnown {
			score = worker.ReliabilityBps
		}
		if !found || score > bestScore {
			bestURL = worker.URL
			bestScore = score
			found = true
		}
	}
	if found {
		return bestURL, nil
	}
	if len(d.workers) == 1 && d.workers[0].URL != "" && (d.workers[0].ReputationKnown == false || d.workers[0].ReliabilityBps >= d.minBps) {
		return d.workers[0].URL, nil
	}
	return "", fmt.Errorf("no healthy worker offers %s above %d bps", service, d.minBps)
}

func offers(worker Manifest, service string) bool {
	for _, offered := range worker.Services {
		if offered.Name == service {
			return true
		}
	}
	return false
}
