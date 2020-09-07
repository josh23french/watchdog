package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/alertmanager/template"
)

type WebhookMessage struct {
	*template.Data

	Version string `json:"version"`
}

var lastReceivedTime int64

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	var webhookData WebhookMessage

	err := json.NewDecoder(r.Body).Decode(&webhookData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var alertname string

	alerts := webhookData.Alerts
	for _, a := range alerts {
		for k, v := range a.Labels {
			if k == "alertname" {
				alertname = v
			}
		}
	}

	labels := webhookData.CommonLabels
	for k, v := range labels {
		if k == "alertname" {
			alertname = v
		}
	}
	lastReceivedTime = time.Now().UnixNano()
	log.Printf("Got alerts: %s\n", alertname)
}

func main() {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	listen := flag.String("listen", ":8080", "ip:port to listen on")
	watchdogTTLSeconds := flag.Int64("ttl", 30, "seconds to wait before alerting")
	resendSeconds := flag.Int64("resend", 300, "seconds to wait before re-sending an alert")

	flag.Parse()

	lastReceivedTime = time.Now().UnixNano()

	go func() {
		sig := <-sigs
		fmt.Println()
		log.Println(sig)
		done <- true
	}()

	go func() {
		http.HandleFunc("/webhook", handleWebhook)
		log.Printf("server starting on %s...\n", *listen)
		log.Fatal(http.ListenAndServe(*listen, nil))
	}()

	go func() {
		var lastSentTime int64 = 0
		for {
			now := time.Now().UnixNano()
			if lastReceivedTime < now-(*watchdogTTLSeconds*int64(time.Second)) {
				// Watchdog alert has not come in time
				if lastSentTime < now-(*resendSeconds*int64(time.Second)) {
					log.Println("Woof! WOOF!")
					lastSentTime = now
				}
			}
			time.Sleep(1 * time.Second)
		}
	}()

	<-done
	fmt.Println("quitting")
}
