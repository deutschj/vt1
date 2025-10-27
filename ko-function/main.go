package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Power struct {
	Timestamp    time.Time `json:"timestamp"`
	TempC        float64   `json:"temp_c"`
	VoltV        float64   `json:"volt_v"`
	ClockArmMHz  float64   `json:"clock_arm_mhz"`
	Undervoltage bool      `json:"undervoltage"`
	FreqCapped   bool      `json:"freq_capped"`
	Throttled    bool      `json:"throttled"`
	LastError    string    `json:"last_error,omitempty"`
}

var (
	powerURL string
	cli      = &http.Client{Timeout: 600 * time.Millisecond}

	mu      sync.Mutex
	cache   Power
	cacheAt time.Time
	ttl     = 5 * time.Second
)

func init() {
	// Prefer explicit URL, else build from HOST_IP
	powerURL = os.Getenv("POWER_API_URL")
	if powerURL == "" {
		if host := os.Getenv("HOST_IP"); host != "" {
			powerURL = "http://" + host + ":8085/power"
		}
	}
}

func getPower() (Power, error) {
	mu.Lock()
	defer mu.Unlock()

	if time.Since(cacheAt) < ttl && !cache.Timestamp.IsZero() {
		return cache, nil
	}
	if powerURL == "" {
		return Power{LastError: "POWER_API_URL/HOST_IP not set"}, nil
	}

	req, _ := http.NewRequest("GET", powerURL, nil)
	resp, err := cli.Do(req)
	if err != nil {
		cache = Power{LastError: err.Error()}
		cacheAt = time.Now()
		return cache, nil
	}
	defer resp.Body.Close()

	var p Power
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		cache = Power{LastError: err.Error()}
		cacheAt = time.Now()
		return cache, nil
	}
	cache, cacheAt = p, time.Now()
	return p, nil
}

func Handle(w http.ResponseWriter, r *http.Request) {
	p, _ := getPower() // tolerate errors; LastError will be set
	degraded := p.Undervoltage || p.FreqCapped || p.Throttled || p.TempC > 70.0

	// Dump the request for debugging (to logs, not to the client).
	if dump, err := httputil.DumpRequest(r, true); err == nil {
		log.Printf("request dump:\n%s", dump)
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{
		"degraded": degraded,
		"power":    p,
		"server": map[string]any{
			"time": time.Now().UTC(),
		},
	})
}

// health/readiness endpoints (handy for k8s)
func healthz(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
func readyz(w http.ResponseWriter, _ *http.Request)  { w.WriteHeader(http.StatusOK) }

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", Handle)
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/readyz", readyz)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// graceful shutdown
	go func() {
		log.Printf("listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	fmt.Println("server shut down")
}
