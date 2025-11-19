package whatsapp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"back_wa/internal/database"
	"back_wa/internal/services"
)

// MultiUserWhatsAppHandler handles WhatsApp operations for multiple users
type MultiUserWhatsAppHandler struct {
	waManager       *MultiUserWhatsAppManager
	authService     *services.AuthService
	analysisService *services.AnalysisService
}

// NewMultiUserWhatsAppHandler creates a new multi-user WhatsApp handler
func NewMultiUserWhatsAppHandler() *MultiUserWhatsAppHandler {
	return &MultiUserWhatsAppHandler{
		waManager:       NewMultiUserWhatsAppManager(),
		authService:     &services.AuthService{},
		analysisService: &services.AnalysisService{},
	}
}

// extractUserIDFromToken extracts user ID from JWT token
func (h *MultiUserWhatsAppHandler) extractUserIDFromToken(r *http.Request) (uint, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return 0, fmt.Errorf("authorization header required")
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		return 0, fmt.Errorf("invalid authorization header format")
	}

	claims, err := h.authService.ValidateToken(tokenString)
	if err != nil {
		return 0, fmt.Errorf("invalid token: %v", err)
	}

	return claims.UserID, nil
}

// HandleQR returns QR code for specific user
func (h *MultiUserWhatsAppHandler) HandleQR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response := map[string]interface{}{
			"error": "Method not allowed",
			"success": false,
			"qr": "",
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Extract user ID from token
	userID, err := h.extractUserIDFromToken(r)
	if err != nil {
		response := map[string]interface{}{
			"error": err.Error(),
			"success": false,
			"qr": "",
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"authenticated": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Get QR code for user
	qrCode, err := h.waManager.GetQRCode(userID)
	if err != nil {
		log.Printf("ERROR: User %d - Failed to get QR code: %v", userID, err)
		response := map[string]interface{}{
			"error": "Failed to get QR code",
			"success": false,
			"qr": "",
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"qr_available": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if qrCode == "" {
		// QR code belum tersedia
		json.NewEncoder(w).Encode(map[string]interface{}{
			"qr":      "",
			"message": "QR Code sedang dibuat, silakan coba lagi dalam beberapa detik.",
			"ready":   h.waManager.IsReady(userID),
		})
		return
	}

	// Return QR code untuk user
	json.NewEncoder(w).Encode(map[string]string{"qr": qrCode})
}

// HandleStatus returns status for specific user
func (h *MultiUserWhatsAppHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response := map[string]interface{}{
			"error": "Method not allowed",
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Extract user ID from token
	userID, err := h.extractUserIDFromToken(r)
	if err != nil {
		response := map[string]interface{}{
			"error": err.Error(),
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"authenticated": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	status := h.waManager.IsReady(userID)
	waStatus, err := h.waManager.GetStatus(userID)
	if err != nil {
		waStatus = "disconnected"
	}

	// Check if analysis is ready
	analysisReady := h.waManager.IsAnalysisReady(userID)

	response := map[string]interface{}{
		"ready":           status,
		"whatsapp_status": waStatus,
		"analysis_ready":  analysisReady,
		"user_id":         userID,
		"timestamp":       time.Now().Format(time.RFC3339),
	}

	// If WhatsApp is connected, check for phone number mismatch
	if status {
		client := h.waManager.GetClient(userID)
		if client != nil && client.Store.ID != nil {
			whatsappPhoneNumber := client.Store.ID.User
			if whatsappPhoneNumber != "" {
				log.Printf("DEBUG: User %d - Checking phone number %s in status endpoint", userID, whatsappPhoneNumber)
				
				// Check payment for this phone number
				db := database.GetDB()
				if db != nil {
					paymentService := services.NewPaymentService(db)
					if paymentService != nil {
						hasPaidForPhone, err := paymentService.CheckIfUserPaidForPhone(int(userID), whatsappPhoneNumber)
						if err != nil {
							log.Printf("ERROR: User %d - Failed to check payment for phone %s in status: %v", userID, whatsappPhoneNumber, err)
						} else if !hasPaidForPhone {
							// Check if user has any paid transactions for other phone numbers
							hasAnyPaidTransaction, err := paymentService.CheckIfUserHasAnyPaidTransaction(int(userID))
							if err != nil {
								log.Printf("ERROR: User %d - Failed to check if user has any paid transactions in status: %v", userID, err)
								hasAnyPaidTransaction = false
							}

							if hasAnyPaidTransaction {
								// User has paid for different phone number
								response["phone_mismatch"] = map[string]interface{}{
									"error_type":    "wrong_phone_number",
									"scanned_phone": whatsappPhoneNumber,
									"message":       fmt.Sprintf("Anda sudah membayar untuk nomor lain, tapi mencoba scan nomor %s. Silakan bayar untuk nomor ini atau scan nomor yang sudah dibayar.", whatsappPhoneNumber),
								}
								log.Printf("DEBUG: User %d - Phone number mismatch detected in status: %s", userID, whatsappPhoneNumber)
							} else {
								// User has no paid transactions at all
								response["phone_mismatch"] = map[string]interface{}{
									"error_type":   "no_payment",
									"phone_number": whatsappPhoneNumber,
									"message":      fmt.Sprintf("Pembayaran diperlukan untuk nomor %s. Silakan lakukan pembayaran terlebih dahulu.", whatsappPhoneNumber),
								}
								log.Printf("DEBUG: User %d - No payment detected in status for phone: %s", userID, whatsappPhoneNumber)
							}
						}
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleAnalyze analyzes WhatsApp data for specific user
func (h *MultiUserWhatsAppHandler) HandleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response := map[string]interface{}{
			"error": "Method not allowed",
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Extract user ID from token
	userID, err := h.extractUserIDFromToken(r)
	if err != nil {
		response := map[string]interface{}{
			"error": err.Error(),
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"authenticated": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("DEBUG: User %d - HandleAnalyze called - starting analysis... (VERSION: FIXED) - REQUEST ID: %d", userID, time.Now().UnixNano())

	// Get WhatsApp phone number from client
	client := h.waManager.GetClient(userID)
	if client == nil || client.Store.ID == nil {
		log.Printf("ERROR: User %d - WhatsApp client not available for phone number check", userID)
		response := map[string]interface{}{
			"error": "WhatsApp client not available",
			"success": false,
			"user_id": userID,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"client_available": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Extract phone number from WhatsApp client
	whatsappPhoneNumber := client.Store.ID.User
	if whatsappPhoneNumber == "" {
		log.Printf("ERROR: User %d - Could not extract phone number from WhatsApp client", userID)
		response := map[string]interface{}{
			"error": "Could not extract phone number from WhatsApp",
			"success": false,
			"user_id": userID,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"phone_extracted": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("DEBUG: User %d - WhatsApp phone number: %s", userID, whatsappPhoneNumber)

	// Enforce payment: user must have PAID transaction for this specific phone number
	db := database.GetDB()
	if db == nil {
		log.Printf("ERROR: User %d - Database connection is nil", userID)
		response := map[string]interface{}{
			"error": "Database not initialized",
			"success": false,
			"user_id": userID,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"database_ready": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}
	log.Printf("DEBUG: User %d - Database connection obtained successfully", userID)

	// Initialize PaymentService with proper database connection
	log.Printf("DEBUG: User %d - Initializing PaymentService with database connection", userID)
	paymentService := services.NewPaymentService(db)
	if paymentService == nil {
		log.Printf("ERROR: User %d - Failed to initialize PaymentService", userID)
		response := map[string]interface{}{
			"error": "Failed to initialize payment service",
			"success": false,
			"user_id": userID,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"payment_service_ready": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}
	log.Printf("DEBUG: User %d - PaymentService initialized successfully", userID)

	// Debug: PaymentService initialized, proceeding with payment check
	log.Printf("DEBUG: User %d - PaymentService initialized, proceeding with payment check", userID)

	hasPaidForPhone, err := paymentService.CheckIfUserPaidForPhone(int(userID), whatsappPhoneNumber)
	if err != nil {
		log.Printf("ERROR: User %d - Failed to check payment for phone %s: %v", userID, whatsappPhoneNumber, err)
		response := map[string]interface{}{
			"error": "Failed to verify payment status",
			"success": false,
			"user_id": userID,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"payment_verified": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if !hasPaidForPhone {
		log.Printf("DEBUG: User %d - No payment found for phone number %s", userID, whatsappPhoneNumber)

		// Check if user has any paid transactions for other phone numbers
		hasAnyPaidTransaction, err := paymentService.CheckIfUserHasAnyPaidTransaction(int(userID))
		if err != nil {
			log.Printf("ERROR: User %d - Failed to check if user has any paid transactions: %v", userID, err)
			// If we can't check, assume no payment to be safe
			hasAnyPaidTransaction = false
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)

		if hasAnyPaidTransaction {
			// User has paid for different phone number
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":         "Payment required for different phone number",
				"success":       false,
				"user_id":       userID,
				"scanned_phone": whatsappPhoneNumber,
				"message":       fmt.Sprintf("Anda sudah membayar untuk nomor lain, tapi mencoba scan nomor %s. Silakan bayar untuk nomor ini atau scan nomor yang sudah dibayar.", whatsappPhoneNumber),
				"error_type":    "wrong_phone_number",
			})
		} else {
			// User has no paid transactions at all
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":        "Payment required",
				"success":      false,
				"user_id":      userID,
				"phone_number": whatsappPhoneNumber,
				"message":      fmt.Sprintf("Pembayaran diperlukan untuk nomor %s. Silakan lakukan pembayaran terlebih dahulu.", whatsappPhoneNumber),
				"error_type":   "no_payment",
			})
		}
		log.Printf("DEBUG: User %d - Payment validation failed, returning error 402 - REQUEST ID: %d", userID, time.Now().UnixNano())
		return
	}

	log.Printf("DEBUG: User %d - Payment verified for phone number %s - REQUEST ID: %d", userID, whatsappPhoneNumber, time.Now().UnixNano())

	// Check if we have cached analysis result first (but only after payment validation)
	if cachedResult, exists := h.waManager.GetCachedAnalysis(userID); exists {
		log.Printf("DEBUG: User %d - Payment verified, returning cached analysis result", userID)
		response := map[string]interface{}{
			"success": true,
			"message": "Cached analysis result",
			"user_id": userID,
			"result":  cachedResult,
			"cached":  true,
			"status": map[string]interface{}{
				"whatsapp_ready": true,
				"timestamp":      time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Check if WhatsApp is ready for user
	if !h.waManager.IsReady(userID) {
		log.Printf("DEBUG: User %d - WhatsApp not ready, cannot analyze", userID)
		response := map[string]interface{}{
			"error": "WhatsApp not logged in. Please scan QR code first",
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"user_id":        userID,
				"timestamp":      time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Client already validated above for phone number check

	// Additional client validation
	if !client.IsConnected() {
		log.Printf("DEBUG: User %d - WhatsApp client not connected", userID)
		response := map[string]interface{}{
			"error": "WhatsApp client not connected. Please reconnect and try again",
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"user_id":        userID,
				"timestamp":      time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("DEBUG: User %d - Client validation passed, starting analysis...", userID)

	// Get session and perform analysis using the SAME logic as single-user
	session, err := h.waManager.GetOrCreateSession(userID)
	if err != nil {
		log.Printf("ERROR: User %d - Failed to get session: %v", userID, err)
		response := map[string]interface{}{
			"error":   err.Error(),
			"user_id": userID,
			"status": map[string]interface{}{
				"whatsapp_ready": true,
				"timestamp":      time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Use the SAME analysis method as single-user
	analysisResult, err := session.Analyze()
	if err != nil {
		log.Printf("ERROR: User %d - Analysis failed: %v", userID, err)
		response := map[string]interface{}{
			"error":   err.Error(),
			"user_id": userID,
			"status": map[string]interface{}{
				"whatsapp_ready": true,
				"timestamp":      time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("DEBUG: User %d - Analysis completed successfully", userID)

	// Return analysis result
	response := map[string]interface{}{
		"success": true,
		"message": "Analysis completed successfully",
		"user_id": userID,
		"result":  analysisResult,
		"cached":  false,
		"status": map[string]interface{}{
			"whatsapp_ready": true,
			"timestamp":      time.Now().Format(time.RFC3339),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleLogout logs out WhatsApp for specific user
func (h *MultiUserWhatsAppHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response := map[string]interface{}{
			"error": "Method not allowed",
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Extract user ID from token
	userID, err := h.extractUserIDFromToken(r)
	if err != nil {
		response := map[string]interface{}{
			"error": err.Error(),
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"authenticated": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("DEBUG: User %d - Logout request received", userID)

	// Check if user has an active session before logout
	client := h.waManager.GetClient(userID)
	if client == nil {
		log.Printf("DEBUG: User %d - No active session found, logout successful", userID)
		response := map[string]interface{}{
			"success": true,
			"message": "No active session found",
			"user_id": userID,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Logout WhatsApp for user
	if err := h.waManager.Logout(userID); err != nil {
		log.Printf("ERROR: User %d - Failed to logout: %v", userID, err)
		response := map[string]interface{}{
			"error":   err.Error(),
			"success": false,
			"user_id": userID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("DEBUG: User %d - Logout completed successfully", userID)

	response := map[string]interface{}{
		"success": true,
		"message": "WhatsApp logged out successfully",
		"user_id": userID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleRefreshQR refreshes QR code for specific user
func (h *MultiUserWhatsAppHandler) HandleRefreshQR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response := map[string]interface{}{
			"error": "Method not allowed",
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Extract user ID from token
	userID, err := h.extractUserIDFromToken(r)
	if err != nil {
		response := map[string]interface{}{
			"error": err.Error(),
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"authenticated": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("DEBUG: User %d - Refresh QR request received", userID)

	// TODO: Implement QR refresh logic
	// For now, return success response
	response := map[string]interface{}{
		"success": true,
		"message": "QR code refresh initiated",
		"user_id": userID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleManualReconnect manually reconnects WhatsApp for specific user
func (h *MultiUserWhatsAppHandler) HandleManualReconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response := map[string]interface{}{
			"error": "Method not allowed",
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Extract user ID from token
	userID, err := h.extractUserIDFromToken(r)
	if err != nil {
		response := map[string]interface{}{
			"error": err.Error(),
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"authenticated": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("DEBUG: User %d - Manual reconnect request received", userID)

	// Connect WhatsApp for user
	if err := h.waManager.Connect(userID); err != nil {
		log.Printf("ERROR: User %d - Failed to reconnect: %v", userID, err)
		http.Error(w, "Failed to reconnect WhatsApp", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": "WhatsApp reconnection initiated",
		"user_id": userID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleForceAnalysis forces analysis for specific user
func (h *MultiUserWhatsAppHandler) HandleForceAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response := map[string]interface{}{
			"error": "Method not allowed",
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Extract user ID from token
	userID, err := h.extractUserIDFromToken(r)
	if err != nil {
		response := map[string]interface{}{
			"error": err.Error(),
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"authenticated": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("DEBUG: User %d - Force analysis request received", userID)

	// Check if WhatsApp is ready
	if !h.waManager.IsReady(userID) {
		response := map[string]interface{}{
			"error":   "WhatsApp not ready. Please connect first.",
			"user_id": userID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Get client and force analysis
	client := h.waManager.GetClient(userID)
	if client == nil {
		response := map[string]interface{}{
			"error":   "WhatsApp client not available",
			"user_id": userID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Get session and perform analysis using the SAME logic as single-user
	session, err := h.waManager.GetOrCreateSession(userID)
	if err != nil {
		response := map[string]interface{}{
			"error":   err.Error(),
			"user_id": userID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Use the SAME analysis method as single-user
	result, err := session.Analyze()
	if err != nil {
		response := map[string]interface{}{
			"error":   err.Error(),
			"user_id": userID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Analysis completed successfully",
		"result":  result,
		"user_id": userID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleDebug returns debug information for specific user
func (h *MultiUserWhatsAppHandler) HandleDebug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response := map[string]interface{}{
			"error": "Method not allowed",
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Extract user ID from token
	userID, err := h.extractUserIDFromToken(r)
	if err != nil {
		response := map[string]interface{}{
			"error": err.Error(),
			"success": false,
			"status": map[string]interface{}{
				"whatsapp_ready": false,
				"authenticated": false,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	client := h.waManager.GetClient(userID)
	status := h.waManager.IsReady(userID)
	waStatus, _ := h.waManager.GetStatus(userID)

	debugInfo := map[string]interface{}{
		"user_id":         userID,
		"client_exists":   client != nil,
		"client_ready":    status,
		"whatsapp_status": waStatus,
		"timestamp":       time.Now().Format(time.RFC3339),
	}

	if client != nil {
		debugInfo["client_connected"] = client.IsConnected()
		if client.Store.ID != nil {
			debugInfo["client_id"] = client.Store.ID.String()
		}
		// Add more detailed client info
		debugInfo["client_store_exists"] = client.Store != nil
		if client.Store != nil {
			debugInfo["contacts_store_exists"] = client.Store.Contacts != nil
		}
	}

	// Add session info
	sessionInfo := h.waManager.GetSessionInfo(userID)
	for key, value := range sessionInfo {
		debugInfo["session_"+key] = value
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(debugInfo)
}
