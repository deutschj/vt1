package main

import (
	"context"
	"log"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

func main() {
	c, err := cloudevents.NewClientHTTP()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("listening for events…")
	if err := c.StartReceiver(context.Background(), handle); err != nil {
		log.Fatal(err)
	}
}

func handle(ctx context.Context, event cloudevents.Event) (*cloudevents.Event, cloudevents.Result) {
	// Optional: read extensions (they’re present because the Trigger matched)
	if ps, ok := event.Extensions()["power_state"]; ok {
		log.Printf("power_state=%v", ps)
	}
	// Do your work here. Return 2xx to ack.
	return nil, cloudevents.ResultACK
}
