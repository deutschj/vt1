package main

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"os"
	"time"
)

// PowerStatus represents the node's (simulated) power data
type PowerStatus struct {
	NodeName       string `json:"node_name"`
	BatteryPercent int    `json:"battery_percent"`
	IsCharging     bool   `json:"is_charging"`
	TimeOfDay      string `json:"time_of_day"`
	SolarAvailable bool   `json:"solar_available"`
	LastUpdated    string `json:"last_updated"`
}

func main() {
	rand.Seed(time.Now().UnixNano())

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		status := generatePowerStatus()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Power metrics API daemon — GET /status\n"))
	})

	addr := ":8080"
	println("Serving power metrics on", addr)
	http.ListenAndServe(addr, nil)
}

func generatePowerStatus() PowerStatus {
	battery := rand.Intn(60) + 40 // 40–100%
	isCharging := rand.Intn(2) == 0
	hour := time.Now().Hour()

	timeOfDay := "day"
	if hour >= 18 || hour < 6 {
		timeOfDay = "night"
	}

	solar := timeOfDay == "day" && !isCharging

	return PowerStatus{
		NodeName:       getNodeName(),
		BatteryPercent: battery,
		IsCharging:     isCharging,
		TimeOfDay:      timeOfDay,
		SolarAvailable: solar,
		LastUpdated:    time.Now().Format(time.RFC3339),
	}
}

func getNodeName() string {
	// When running in Kubernetes, this env var is automatically injected
	node := os.Getenv("NODE_NAME")

	if node != "" {
		return node
	}
	return "unknown-node"
}
