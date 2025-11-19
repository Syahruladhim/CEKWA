package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"back_wa/internal/models"
	"back_wa/internal/services"
)

type PaymentHandler struct {
	paymentService *services.PaymentService
}

func NewPaymentHandler(paymentService *services.PaymentService) *PaymentHandler {
	return &PaymentHandler{
		paymentService: paymentService,
	}
}

// CreatePayment handles POST /api/payments/create
func (ph *PaymentHandler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("🚀 Payment creation request received: %s %s\n", r.Method, r.URL.Path)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.CreatePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Printf("❌ Invalid request body: %v\n", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	fmt.Printf("📋 Request data: %+v\n", req)

	// Get user ID from JWT token (you'll need to implement this)
	userID := ph.getUserIDFromToken(r)
	if userID == 0 {
		fmt.Printf("❌ Unauthorized: No valid user ID from token\n")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	fmt.Printf("👤 User ID: %d\n", userID)

	// Validate request
	if req.Email == "" || req.Category == "" || req.PaymentMethod == "" || req.Amount <= 0 {
		fmt.Printf("❌ Missing required fields: email=%s, category=%s, payment_method=%s, amount=%f\n",
			req.Email, req.Category, req.PaymentMethod, req.Amount)
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Set default amount if not provided
	if req.Amount == 0 {
		req.Amount = 50000 // Default amount for WhatsApp analysis
		fmt.Printf("💰 Using default amount: %f\n", req.Amount)
	}

	// Create payment service request (using models.CreatePaymentRequest directly)
	paymentReq := models.CreatePaymentRequest{
		Email:         req.Email,
		Amount:        req.Amount,
		Category:      req.Category,
		PaymentMethod: req.PaymentMethod,
		PhoneNumber:   req.PhoneNumber,
	}

	// Create payment
	fmt.Printf("🔄 Creating payment for user %d with data: %+v\n", userID, paymentReq)
	paymentResp, err := ph.paymentService.CreatePayment(paymentReq, userID)
	if err != nil {
		fmt.Printf("❌ Payment creation failed: %v\n", err)
		// Map common Xendit errors to clearer HTTP responses
		msg := err.Error()
		switch {
		case strings.Contains(msg, "xendit_error"):
			http.Error(w, "Gagal membuat invoice di Xendit. Periksa XENDIT_SECRET_KEY/BASE_URL dan gunakan kunci sesuai environment (sandbox/live).", http.StatusBadGateway)
			return
		case strings.Contains(msg, "not configured"):
			http.Error(w, "Konfigurasi payment service belum lengkap.", http.StatusServiceUnavailable)
			return
		default:
			http.Error(w, fmt.Sprintf("Failed to create payment: %v", err), http.StatusInternalServerError)
			return
		}
	}
	fmt.Printf("✅ Payment created successfully: %+v\n", paymentResp)

	// Prepare response
	response := models.CreatePaymentResponse{
		ID:            paymentResp.ID,
		ExternalID:    paymentResp.ExternalID,
		InvoiceID:     paymentResp.InvoiceID,
		InvoiceURL:    paymentResp.InvoiceURL,
		Amount:        paymentResp.Amount,
		Status:        paymentResp.Status,
		PaymentMethod: paymentResp.PaymentMethod,
		CreatedAt:     paymentResp.CreatedAt,
		ExpiryDate:    paymentResp.ExpiryDate,
		Message:       "Payment created successfully",
	}

	fmt.Printf("📤 Sending response: %+v\n", response)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	fmt.Printf("✅ Payment creation completed successfully\n")
}

// GetPaymentStatus handles GET /api/payments/:external_id/status
func (ph *PaymentHandler) GetPaymentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract external_id from URL path
	externalID := r.URL.Path[len("/api/payments/"):]
	if len(externalID) > 0 && externalID[len(externalID)-7:] == "/status" {
		externalID = externalID[:len(externalID)-7]
	}

	if externalID == "" {
		http.Error(w, "External ID is required", http.StatusBadRequest)
		return
	}

	// Get user ID from JWT token
	userID := ph.getUserIDFromToken(r)
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get transaction (with reconciliation if still pending)
	transaction, err := ph.paymentService.ReconcileTransactionStatusByExternalID(externalID)
	if err != nil {
		http.Error(w, "Transaction not found", http.StatusNotFound)
		return
	}

	// Check if user owns this transaction
	if transaction.UserID != userID {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Prepare response
	response := models.PaymentStatusResponse{
		ID:             transaction.ID,
		ExternalID:     transaction.ExternalID,
		InvoiceID:      transaction.InvoiceID,
		Amount:         transaction.Amount,
		Status:         transaction.Status,
		PaymentMethod:  transaction.PaymentMethod,
		PaymentChannel: transaction.PaymentChannel,
		CreatedAt:      transaction.CreatedAt,
		UpdatedAt:      transaction.UpdatedAt,
		PaidAt:         transaction.PaidAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetTransactionHistory handles GET /api/transactions
func (ph *PaymentHandler) GetTransactionHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user ID from JWT token
	userID := ph.getUserIDFromToken(r)
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get transactions
	transactions, err := ph.paymentService.GetUserTransactions(userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get transactions: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to response format
	var response []models.TransactionHistoryResponse
	for _, transaction := range transactions {
		response = append(response, models.TransactionHistoryResponse{
			ID:             transaction.ID,
			ExternalID:     transaction.ExternalID,
			Amount:         transaction.Amount,
			Currency:       transaction.Currency,
			Status:         transaction.Status,
			PaymentMethod:  transaction.PaymentMethod,
			PaymentChannel: transaction.PaymentChannel,
			Description:    transaction.Description,
			PhoneNumber:    transaction.PhoneNumber,
			CreatedAt:      transaction.CreatedAt,
			UpdatedAt:      transaction.UpdatedAt,
			PaidAt:         transaction.PaidAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    response,
	})
}

// HandleWebhook handles POST /api/webhooks/xendit
func (ph *PaymentHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload models.WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid webhook payload", http.StatusBadRequest)
		return
	}

	// Verify webhook signature (implement this based on Xendit's webhook verification)
	// For now, we'll skip verification in sandbox mode

	// Update transaction status
	err := ph.paymentService.UpdateTransactionStatus(
		payload.ExternalID,
		payload.Status,
		payload.PaymentChannel,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update transaction: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Webhook processed successfully"))
}

// Helper function to get user ID from JWT token
func (ph *PaymentHandler) getUserIDFromToken(r *http.Request) int {
	// Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return 0
	}

	// Remove "Bearer " prefix
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		authHeader = authHeader[7:]
	}

	// Validate JWT token using auth service
	authService := &services.AuthService{}
	claims, err := authService.ValidateToken(authHeader)
	if err != nil {
		return 0
	}

	return int(claims.UserID)
}
