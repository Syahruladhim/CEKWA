package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"back_wa/internal/models"
	"back_wa/internal/services"
)

type WebhookHandler struct {
	paymentService *services.PaymentService
}

func NewWebhookHandler(paymentService *services.PaymentService) *WebhookHandler {
	return &WebhookHandler{
		paymentService: paymentService,
	}
}

// HandleXenditWebhook handles POST /api/webhooks/xendit
func (wh *WebhookHandler) HandleXenditWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the raw body for signature verification
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Verify webhook signature
	// Try multiple known headers from Xendit variants
	signature := r.Header.Get("X-Xendit-Signature")
	if signature == "" {
		signature = r.Header.Get("X-Callback-Signature")
	}
	if signature == "" {
		signature = r.Header.Get("X-Xendit-Callback-Signature")
	}
	legacyToken := r.Header.Get("X-Callback-Token")

	if !wh.verifyWebhookSignature(body, signature, legacyToken) {
		fmt.Printf("❌ Invalid webhook signature. Headers: X-Xendit-Signature='%s' X-Callback-Signature='%s' X-Xendit-Callback-Signature='%s' X-Callback-Token='%s'\n",
			r.Header.Get("X-Xendit-Signature"), r.Header.Get("X-Callback-Signature"), r.Header.Get("X-Xendit-Callback-Signature"), legacyToken)

		// Optionally bypass verification in sandbox if explicitly allowed
		if strings.EqualFold(os.Getenv("XENDIT_WEBHOOK_DISABLE_VERIFY"), "true") {
			fmt.Println("⚠️ Bypassing webhook verification due to XENDIT_WEBHOOK_DISABLE_VERIFY=true (sandbox only)")
		} else {
			http.Error(w, "Invalid webhook signature", http.StatusUnauthorized)
			return
		}
	}

	// Parse webhook payload
	var payload models.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid webhook payload", http.StatusBadRequest)
		return
	}

	// Log webhook for debugging with key details
	fmt.Printf("📣 Xendit webhook: ext=%s status=%s channel=%s amount=%.2f id=%s\n",
		payload.ExternalID, payload.Status, payload.PaymentChannel, payload.Amount, payload.ID)

	// Update transaction status
	err = wh.paymentService.UpdateTransactionStatus(
		payload.ExternalID,
		payload.Status,
		payload.PaymentChannel,
	)
	if err != nil {
		fmt.Printf("Failed to update transaction %s: %v\n", payload.ExternalID, err)
		http.Error(w, fmt.Sprintf("Failed to update transaction: %v", err), http.StatusInternalServerError)
		return
	}

	// Log successful update
	fmt.Printf("✅ Updated transaction %s to status %s (channel=%s)\n",
		payload.ExternalID, payload.Status, payload.PaymentChannel)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Webhook processed successfully"))
}

// verifyWebhookSignature verifies Xendit webhook authenticity
// - New style: HMAC SHA256 of raw body using XENDIT_WEBHOOK_TOKEN as key, compare to X-Xendit-Signature (hex)
// - Legacy: direct equality check of X-Callback-Token header to XENDIT_WEBHOOK_TOKEN
func (wh *WebhookHandler) verifyWebhookSignature(payload []byte, signature string, legacyToken string) bool {
	webhookToken := os.Getenv("XENDIT_WEBHOOK_TOKEN")
	if webhookToken == "" {
		// If no token is set, accept (useful for local sandbox testing)
		return true
	}

	// Legacy token path
	if legacyToken != "" && legacyToken == webhookToken {
		return true
	}

	// HMAC verification (preferred)
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(webhookToken))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	// Some implementations send base64; accept both hex and base64 (best-effort)
	if strings.EqualFold(signature, expected) {
		return true
	}
	// Try base64
	// Note: We won't import b64 unless needed; quick check for '=' padding
	// For strictness, we still primarily rely on hex match.
	return false
}

// HandleWebhookTest handles GET /api/webhooks/test for testing webhook endpoint
func (wh *WebhookHandler) HandleWebhookTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string]interface{}{
		"status":    "ok",
		"message":   "Webhook endpoint is working",
		"timestamp": time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
