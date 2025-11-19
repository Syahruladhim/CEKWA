package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"back_wa/internal/models"
)

type XenditService struct {
	BaseURL      string
	SecretKey    string
	PublicKey    string
	WebhookToken string
}

func NewXenditService() *XenditService {
	baseURL := os.Getenv("XENDIT_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.xendit.co" // Default Xendit URL
	}

	secretKey := os.Getenv("XENDIT_SECRET_KEY")
	if secretKey == "" {
		// Use example key for development
		secretKey = "xnd_development_0sqmWoZ0M92vBFx8krDMheUrZuP4db33ftRDtAfTBuFpCUSIGl5a3CdMsV2UI"
	}

	publicKey := os.Getenv("XENDIT_PUBLIC_KEY")
	if publicKey == "" {
		// Use example key for development
		publicKey = "xnd_public_development_4EpOejfbXBMA8LtFBXKey6OE2mBzx5ijnGS84ba3n7E0dPz4_3hYzPq1jonXP"
	}

	webhookToken := os.Getenv("XENDIT_WEBHOOK_TOKEN")
	if webhookToken == "" {
		// Use example token for development
		webhookToken = "xCnhRJ9iL88UsunRryV15uWu5K1e1PNRJrJ3ZXMDT817oGsI"
	}

	return &XenditService{
		BaseURL:      baseURL,
		SecretKey:    secretKey,
		PublicKey:    publicKey,
		WebhookToken: webhookToken,
	}
}

func (xs *XenditService) CreateInvoice(req models.XenditInvoiceRequest) (*models.XenditInvoiceResponse, error) {
	// Validate Xendit service configuration
	if xs.SecretKey == "" {
		return nil, fmt.Errorf("xendit secret key is not configured")
	}
	if xs.BaseURL == "" {
		return nil, fmt.Errorf("xendit base URL is not configured")
	}

	url := fmt.Sprintf("%s/v2/invoices", xs.BaseURL)
	fmt.Printf("🔗 Creating Xendit invoice at: %s\n", url)

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	fmt.Printf("📤 Xendit request data: %s\n", string(jsonData))

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(xs.SecretKey+":")))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Xendit: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Xendit response: %v", err)
	}

	fmt.Printf("📥 Xendit response status: %d\n", resp.StatusCode)
	fmt.Printf("📥 Xendit response body: %s\n", string(body))

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xendit API error (status %d): %s", resp.StatusCode, string(body))
	}

	var invoiceResp models.XenditInvoiceResponse
	if err := json.Unmarshal(body, &invoiceResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Xendit response: %v", err)
	}

	fmt.Printf("✅ Xendit invoice created successfully: %s\n", invoiceResp.ID)
	return &invoiceResp, nil
}

func (xs *XenditService) GetInvoice(invoiceID string) (*models.XenditInvoiceResponse, error) {
	url := fmt.Sprintf("%s/v2/invoices/%s", xs.BaseURL, invoiceID)

	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(xs.SecretKey+":")))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xendit API error: %s", string(body))
	}

	var invoiceResp models.XenditInvoiceResponse
	if err := json.Unmarshal(body, &invoiceResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return &invoiceResp, nil
}

func (xs *XenditService) VerifyWebhookSignature(payload []byte, signature string) bool {
	// In production, you should implement proper webhook signature verification
	// For now, we'll use a simple token-based verification
	return signature == xs.WebhookToken
}
