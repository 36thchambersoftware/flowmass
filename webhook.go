package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
)

var DISCORD_WEBHOOK_URL string

func initWebhook() {
	webhook, ok := os.LookupEnv("DISCORD_WEBHOOK_URL")
	if !ok {
		log.Printf("Could not get DISCORD_WEBHOOK_URL. Notifications disabled.")
	}

	webhookURL, err := url.Parse(webhook)
	if err != nil {
		log.Fatalf("Invalid webhook url %v", err)
	}

	DISCORD_WEBHOOK_URL = webhookURL.String()
}

func Webhook(message string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	data := discordgo.WebhookParams{Content: message, Username: "Flowmass Mint Bot"}
	params, err := json.Marshal(data)
	if err != nil {
		log.Panicf("could not marshal content: %v", err)
	}

	// Create a new request
	request, err := http.NewRequest(http.MethodPost, DISCORD_WEBHOOK_URL, bytes.NewBuffer(params))
	if err != nil {
		log.Printf("request error: %v", err)
		return
	}

	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		log.Printf("response error: %v", err)
		return
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNoContent {
		log.Printf("You are not waiting for a response. Add ?wait=true to webhook url")
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("error reading body: %v %v", err, body)
		return
	}

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		log.Printf("status: %d, error: %s", response.StatusCode, string(body))
		return
	}
}
