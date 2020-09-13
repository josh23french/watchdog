package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/alertmanager/template"
)

// webhookMessage is the message we expect to see from Alertmanager.
type webhookMessage struct {
	*template.Data

	Version string `json:"version"`
}

var lastReceivedTime int64

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	var webhookData webhookMessage
	gotWatchdog := false

	err := json.NewDecoder(r.Body).Decode(&webhookData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	alerts := webhookData.Alerts

	// Label the loop so we can break out all the way
alertLoop:
	for _, a := range alerts {
		for k, v := range a.Labels {
			if k == "alertname" && v == "Watchdog" {
				gotWatchdog = true
				break alertLoop
			}
		}
	}

	labels := webhookData.CommonLabels
	for k, v := range labels {
		if k == "alertname" && v == "Watchdog" {
			lastReceivedTime = time.Now().UnixNano()
			break
		}
	}

	if gotWatchdog {
		lastReceivedTime = time.Now().UnixNano()
		log.Printf("Got webhook from %s (%s); reset watchdog\n", r.RemoteAddr, webhookData.ExternalURL)
	} else {
		log.Printf("Got webhook, but no alerts labeled alertname=Watchdog")
	}
}

func main() {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Signals
	go func() {
		sig := <-sigs
		fmt.Println()
		log.Println(sig)
		done <- true
	}()

	listen := flag.String("listen", ":8080", "ip:port to listen on")
	watchdogTTLSeconds := flag.Int64("ttl", 30, "seconds to wait before alerting")
	resendSeconds := flag.Int64("resend", 900, "seconds to wait before re-sending an alert")
	reattemptSeconds := flag.Int64("reattempt", 10, "seconds to wait before re-attempting to send an alert that previously failed")

	flag.Parse()

	// Gets set to startup time
	lastReceivedTime = time.Now().UnixNano()

	// HTTP
	go func() {
		http.HandleFunc("/webhook", handleWebhook)
		log.Printf("server starting on %s...\n", *listen)
		log.Fatal(http.ListenAndServe(*listen, nil))
	}()

	// Watchdog
	go func() {
		var lastSentTime int64 = 0
		var lastSendAttemptTime int64 = 0
		for {
			now := time.Now().UnixNano()
			if lastReceivedTime < now-(*watchdogTTLSeconds*int64(time.Second)) {
				// Watchdog alert has not come in time
				if lastSentTime < now-(*resendSeconds*int64(time.Second)) &&
					lastSendAttemptTime < now-(*reattemptSeconds*int64(time.Second)) {

					lastSendAttemptTime = now
					log.Println("Woof! WOOF!")
					cmd := exec.Command("say", "-v", "Xander", "woof", "woof", "woof")
					err := cmd.Start()
					if err != nil {
						log.Printf("Error sending alert: %v", err)
					} else {
						lastSentTime = now
					}
				}
			}
			time.Sleep(1 * time.Second)
		}
	}()

	<-done
	fmt.Println("quitting")
}
