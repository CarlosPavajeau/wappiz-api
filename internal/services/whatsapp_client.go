package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

type WhatsAppMessageReq struct {
	MessagingProduct string      `json:"messaging_product"`
	To               string      `json:"to"`
	Type             string      `json:"type"`
	Text             *TextObject `json:"text,omitempty"`
}

type TextObject struct {
	Body string `json:"body"`
}

// httpClient is shared across all calls to reuse TCP connections via keep-alive.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// waConfig holds values read once from the environment at first use.
var (
	waOnce   sync.Once
	waAPIURL string
	waToken  string
)

func initWAConfig() {
	waOnce.Do(func() {
		phoneID := os.Getenv("PHONE_NUMBER_ID")
		waAPIURL = fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", phoneID)
		waToken = os.Getenv("WHATSAPP_TOKEN")
	})
}

func SendWhatsAppMessage(to, body string) error {
	initWAConfig()

	payload := WhatsAppMessageReq{
		MessagingProduct: "whatsapp",
		To:               to,
		Type:             "text",
		Text:             &TextObject{Body: body},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, waAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+waToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("meta api error (status %d): %s", resp.StatusCode, bodyBytes)
	}

	// Drain body so the underlying TCP connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
