package function

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"sync"
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
	if time.Since(cacheAt) < ttl && cache.Timestamp.Unix() != 0 {
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

// Handle an HTTP Request.
func Handle(w http.ResponseWriter, r *http.Request) {
	/*
	 * YOUR CODE HERE
	 *
	 * Try running `go test`.  Add more test as you code in `handle_test.go`.
	 */

	p, _ := getPower() // tolerate errors; LastError will be set
	degraded := p.Undervoltage || p.FreqCapped || p.Throttled || p.TempC > 70.0

	dump, err := httputil.DumpRequest(r, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"degraded": degraded,
		"power":    p,
		"request":  string(dump),
	})

	fmt.Println("Received request")
	fmt.Printf("%q\n", dump)
	fmt.Fprintf(w, "%q", dump)
}
