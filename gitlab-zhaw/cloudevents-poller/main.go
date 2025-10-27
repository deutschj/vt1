package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

type PowerStatus struct {
	NodeName       string `json:"node_name"`
	BatteryPercent int    `json:"battery_percent"`
	IsCharging     bool   `json:"is_charging"`
	TimeOfDay      string `json:"time_of_day"`
	SolarAvailable bool   `json:"solar_available"`
	LastUpdated    string `json:"last_updated"`
}

func main() {
	target := getenv("POWER_STATUS_URL", "http://power-api.monitoring.svc.cluster.local:8080/status")
	period := getenv("PERIOD", "30s")

	p, err := time.ParseDuration(period)
	if err != nil {
		p = 30 * time.Second
	}

	c, err := cloudevents.NewClientHTTP()
	if err != nil {
		log.Fatal(err)
	}

	for {
		status, err := fetch(target)
		if err != nil {
			log.Printf("fetch error: %v", err)
			time.Sleep(p)
			continue
		}

		// YOUR policy — example: run only when battery < 30%, night, not charging
		shouldRun := status.BatteryPercent < 30 && status.TimeOfDay == "night" && !status.IsCharging
		powerState := "ok"
		if shouldRun {
			powerState = "low"
		}

		data, _ := json.Marshal(status)

		event := cloudevents.NewEvent()
		event.SetSource("power-poller")
		event.SetType("dev.juliand.power.status")
		event.SetTime(time.Now())
		// extensions that Triggers can filter on (strings/bools are fine)
		event.SetExtension("should_run", shouldRun)
		event.SetExtension("power_state", powerState)
		event.SetExtension("node", status.NodeName)

		if err := event.SetData(cloudevents.ApplicationJSON, data); err != nil {
			log.Printf("set data: %v", err)
		}

		if result := c.Send(context.Background(), event); cloudevents.IsUndelivered(result) {
			log.Printf("send failed: %v", result)
		}
		time.Sleep(p)
	}
}

func fetch(url string) (PowerStatus, error) {
	var ps PowerStatus
	resp, err := http.Get(url)
	if err != nil {
		return ps, err
	}
	defer resp.Body.Close()
	return ps, json.NewDecoder(resp.Body).Decode(&ps)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
