package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	ntfyURL   = envOr("NTFY_URL", "https://ntfy.sh")
	ntfyTopic = os.Getenv("NTFY_TOPIC")
	client    = &http.Client{Timeout: 5 * time.Second}
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type alertmanagerWebhook struct {
	Status string  `json:"status"`
	Alerts []alert `json:"alerts"`
}

type alert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

func main() {
	if ntfyTopic == "" {
		log.Fatal("NTFY_TOPIC is required")
	}

	http.HandleFunc("/", handleWebhook)

	port := envOr("PORT", "8080")
	log.Printf("ntfy-adapter listening on :%s, publishing to %s/%s", port, ntfyURL, ntfyTopic)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var payload alertmanagerWebhook
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "parsing payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	for _, a := range payload.Alerts {
		if err := publish(a); err != nil {
			log.Printf("publish failed: %v", err)
			http.Error(w, "publish failed: "+err.Error(), http.StatusBadGateway)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func publish(a alert) error {
	firing := a.Status == "firing"
	severity := a.Labels["severity"]
	alertname := a.Labels["alertname"]
	instanceName := a.Labels["instance_name"]

	title := fmt.Sprintf("%s resolved", alertname)
	priority := "default"
	tags := "white_check_mark"
	if firing {
		title = alertname
		priority = "default"
		tags = "warning"
		if severity == "critical" {
			priority = "urgent"
			tags = "rotating_light"
		}
	}

	message := a.Annotations["summary"]
	if message == "" {
		message = fmt.Sprintf("%s is %s", instanceName, a.Status)
	}
	if !firing {
		message = fmt.Sprintf("%s has recovered", instanceName)
	}

	req, err := http.NewRequest(http.MethodPost, ntfyURL+"/"+ntfyTopic, strings.NewReader(message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", priority)
	req.Header.Set("Tags", tags)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ntfy returned status %s", resp.Status)
	}
	return nil
}
