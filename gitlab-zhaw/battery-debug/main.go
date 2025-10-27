package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type PowerStatus struct {
	NodeName       string `json:"node_name"`
	BatteryPercent int    `json:"battery_percent"`
	IsCharging     bool   `json:"is_charging"`
	TimeOfDay      string `json:"time_of_day"` // "day" | "night"
	SolarAvailable bool   `json:"solar_available"`
	LastUpdated    string `json:"last_updated"`
	LastError      string `json:"last_error,omitempty"` // local field to surface fetch errors
}

var (
	powerURL string
	client   = &http.Client{Timeout: 800 * time.Millisecond}

	mu      sync.Mutex
	cache   PowerStatus
	cacheAt time.Time
	ttl     = 2 * time.Second
)

func init() {
	// Prefer explicit URL. Fallback to HOST_IP (simulator on :8080/status). Final fallback localhost.
	powerURL = os.Getenv("POWER_STATUS_URL")
	if powerURL == "" {
		if host := os.Getenv("HOST_IP"); host != "" {
			powerURL = "http://" + host + ":8080/status"
		}
	}
	if powerURL == "" {
		powerURL = "http://localhost:8080/status"
	}
}

func getStatus() (PowerStatus, error) {
	mu.Lock()
	defer mu.Unlock()

	if time.Since(cacheAt) < ttl && cache.LastUpdated != "" {
		return cache, nil
	}

	req, _ := http.NewRequest("GET", powerURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		cache = PowerStatus{LastError: err.Error()}
		cacheAt = time.Now()
		return cache, nil
	}
	defer resp.Body.Close()

	var ps PowerStatus
	if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
		cache = PowerStatus{LastError: err.Error()}
		cacheAt = time.Now()
		return cache, nil
	}

	cache, cacheAt = ps, time.Now()
	return ps, nil
}

// health/readiness
func healthz(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
func readyz(w http.ResponseWriter, _ *http.Request)  { w.WriteHeader(http.StatusOK) }

func handle(w http.ResponseWriter, r *http.Request) {
	ps, _ := getStatus() // errors show up in ps.LastError

	// Simple derived state: OK if battery >=30% OR charging OR solar is available.
	powerState := "low"
	if ps.BatteryPercent >= 30 || ps.IsCharging || ps.SolarAvailable {
		powerState = "ok"
	}
	shouldRun := powerState == "ok"

	out := map[string]any{
		"source_url":  powerURL,
		"power":       ps,         // raw simulator payload
		"power_state": powerState, // "ok" | "low"
		"should_run":  shouldRun,  // bool
		"cached_at":   cacheAt,    // last fetch time
		"cache_ttl_s": int(ttl.Seconds()),
		"server_time": time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handle)
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/readyz", readyz)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// run server
	go func() {
		log.Printf("listening on :%s, fetching simulator at %s", port, powerURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("server shut down")
}
