package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"strings"

	"back_wa/internal/database"
	"back_wa/internal/handlers"
	"back_wa/internal/services"
	"back_wa/internal/whatsapp"

	"github.com/gorilla/mux"
)

// loadEnvFile loads environment variables from a file
func loadEnvFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return // File doesn't exist, skip silently
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}

	log.Printf("DEBUG: Loaded environment from %s", filename)
}

// CORS middleware
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, ngrok-skip-browser-warning")
		w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	log.Println("DEBUG: Starting WhatsApp API server...")

	// Load environment variables from .env file
	loadEnvFile(".env")
	loadEnvFile("env.production")
	loadEnvFile("env.local")

	// Initialize database
	log.Println("DEBUG: Initializing database...")
	database.InitDatabase()
	log.Println("DEBUG: Database initialized successfully")

	// Initialize user handler
	userHandler := handlers.NewUserHandler()

	// Initialize multi-user WhatsApp handler
	waHandler := whatsapp.NewMultiUserWhatsAppHandler()

	// Initialize payment handler
	paymentService := services.NewPaymentService(database.GetDB())
	paymentHandler := handlers.NewPaymentHandler(paymentService)
	webhookHandler := handlers.NewWebhookHandler(paymentService)

	r := mux.NewRouter()

	// User management endpoints
	r.HandleFunc("/api/auth/register", userHandler.Register).Methods("POST")
	r.HandleFunc("/api/auth/login", userHandler.Login).Methods("POST")
	r.HandleFunc("/api/auth/check-phone", userHandler.CheckPhoneNumber).Methods("GET")
	r.HandleFunc("/api/auth/profile", userHandler.GetProfile).Methods("GET")
	// OTP & Password reset
	r.HandleFunc("/api/auth/send-otp", userHandler.SendOTP).Methods("POST")
	r.HandleFunc("/api/auth/verify-otp", userHandler.VerifyOTP).Methods("POST")
	r.HandleFunc("/api/auth/forgot-password", userHandler.ForgotPassword).Methods("POST")
	r.HandleFunc("/api/auth/reset-password", userHandler.ResetPassword).Methods("POST")
	// Analysis endpoints
	r.HandleFunc("/api/analysis/history", userHandler.GetAnalysisHistory).Methods("GET")
	// Register static and collection routes BEFORE parameterized routes to avoid conflicts
	r.HandleFunc("/api/analysis", userHandler.DeleteAllAnalyses).Methods("DELETE")
	r.HandleFunc("/api/analysis/bulk", userHandler.DeleteAnalysesBulk).Methods("DELETE")
	r.HandleFunc("/api/analysis/{id}", userHandler.GetAnalysisDetail).Methods("GET")
	r.HandleFunc("/api/analysis/{id}", userHandler.DeleteAnalysis).Methods("DELETE")

	// User settings endpoints
	r.HandleFunc("/api/user/change-password", userHandler.ChangePassword).Methods("POST")
	r.HandleFunc("/api/user/change-username", userHandler.ChangeUsername).Methods("POST")

	// WhatsApp endpoints (multi-user)
	r.HandleFunc("/api/wa/qr", waHandler.HandleQR).Methods("GET")
	r.HandleFunc("/api/wa/status", waHandler.HandleStatus).Methods("GET")
	r.HandleFunc("/api/wa/analyze", waHandler.HandleAnalyze).Methods("GET")
	r.HandleFunc("/api/wa/analyze/force", waHandler.HandleForceAnalysis).Methods("POST")
	r.HandleFunc("/api/wa/logout", waHandler.HandleLogout).Methods("POST")
	r.HandleFunc("/api/wa/qr/refresh", waHandler.HandleRefreshQR).Methods("POST")
	r.HandleFunc("/api/wa/debug", waHandler.HandleDebug).Methods("GET")
	r.HandleFunc("/api/wa/reconnect", waHandler.HandleManualReconnect).Methods("POST")

	// Payment endpoints
	r.HandleFunc("/api/payments/create", paymentHandler.CreatePayment).Methods("POST")
	r.HandleFunc("/api/payments/{external_id}/status", paymentHandler.GetPaymentStatus).Methods("GET")
	r.HandleFunc("/api/transactions", paymentHandler.GetTransactionHistory).Methods("GET")

	// Webhook endpoints
	r.HandleFunc("/api/webhooks/xendit", webhookHandler.HandleXenditWebhook).Methods("POST")
	r.HandleFunc("/api/webhooks/test", webhookHandler.HandleWebhookTest).Methods("GET")

	// Health check endpoint
	r.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","message":"Backend is running"}`))
	}).Methods("GET")

	// Apply CORS middleware
	handler := corsMiddleware(r)

	log.Println("🚀 WhatsApp Defender Backend started on :9090")
	log.Println("📡 Available endpoints:")
	log.Println("   🔐 AUTH:")
	log.Println("      POST /api/auth/register     - User registration")
	log.Println("      POST /api/auth/login        - User login")
	log.Println("      GET  /api/auth/check-phone  - Check phone number")
	log.Println("      GET  /api/auth/profile      - Get user profile")
	log.Println("   📱 WHATSAPP:")
	log.Println("      GET  /api/wa/qr             - Get QR code")
	log.Println("      GET  /api/wa/status         - Get WhatsApp status")
	log.Println("      GET  /api/wa/analyze        - Analyze WhatsApp data")
	log.Println("      POST /api/wa/analyze/force  - Force analysis")
	log.Println("      POST /api/wa/logout         - Logout WhatsApp")
	log.Println("      POST /api/wa/qr/refresh     - Refresh QR code")
	log.Println("      GET  /api/wa/debug          - Debug status")
	log.Println("      POST /api/wa/reconnect      - Manual reconnect")
	log.Println("   💳 PAYMENT:")
	log.Println("      POST /api/payments/create   - Create payment")
	log.Println("      GET  /api/payments/{id}/status - Get payment status")
	log.Println("      GET  /api/transactions     - Get transaction history")
	log.Println("   🔗 WEBHOOK:")
	log.Println("      POST /api/webhooks/xendit   - Xendit webhook")
	log.Println("      GET  /api/webhooks/test     - Test webhook")

	log.Fatal(http.ListenAndServe(":9090", handler))
}
