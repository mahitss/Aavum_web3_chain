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
	Worker   string    `json:"worker"`
	Network  string    `json:"network"`
	Protocol string    `json:"protocol"`
	Services []Service `json:"services"`
	Healthy  bool      `json:"healthy"`
	URL      string    `json:"url"`
}

type Directory struct {
	mu      sync.RWMutex
	workers []Manifest
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

func (d *Directory) Endpoint(service string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, worker := range d.workers {
		if !worker.Healthy {
			continue
		}
		for _, offered := range worker.Services {
			if offered.Name == service {
				return worker.URL, nil
			}
		}
	}
	if len(d.workers) == 1 && d.workers[0].URL != "" {
		return d.workers[0].URL, nil
	}
	return "", fmt.Errorf("no healthy worker offers %s", service)
}
