package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"back_wa/internal/database"
	"back_wa/internal/models"
	"back_wa/internal/services"

	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	authService          *services.AuthService
	otpService           *services.OTPService
	passwordResetService *services.PasswordResetService
	emailService         *services.EmailService
	analysisService      *services.AnalysisService
	// Simple in-memory storage for registration OTPs
	registrationOTPs map[string]string
}

func NewUserHandler() *UserHandler {
	return &UserHandler{
		authService:          &services.AuthService{},
		otpService:           services.NewOTPService(),
		passwordResetService: services.NewPasswordResetService(),
		emailService:         &services.EmailService{},
		analysisService:      services.NewAnalysisService(),
		registrationOTPs:     make(map[string]string),
	}
}

// Register handles user registration
func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.UserRegister
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Username == "" || req.Email == "" || req.Password == "" || req.PhoneNumber == "" {
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	// Register user
	user, err := h.authService.Register(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Send verification OTP asynchronously (best effort)
	go func(email string, userID uint) {
		_, _ = h.otpService.GenerateAndSend(email, userID)
	}(user.Email, user.ID)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "User registered successfully",
		"user":    user,
	})
}

// Login handles user authentication
func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.UserLogin
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Email == "" || req.Password == "" {
		http.Error(w, "Email and password are required", http.StatusBadRequest)
		return
	}

	// Login user
	token, user, err := h.authService.Login(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Return success response with token
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Login successful",
		"token":   token,
		"user":    user,
	})
}

// CheckPhoneNumber checks if phone number is already registered
func (h *UserHandler) CheckPhoneNumber(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	phoneNumber := r.URL.Query().Get("phone")
	if phoneNumber == "" {
		http.Error(w, "Phone number is required", http.StatusBadRequest)
		return
	}

	// Check if phone number exists in database
	db := database.GetDB()
	var existingUser models.User
	err := db.Where("phone_number = ?", phoneNumber).First(&existingUser).Error

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err != nil {
		// Phone number not found, user needs to register
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"exists":  false,
			"message": "Phone number not found, please register",
		})
	} else {
		// Phone number found, user needs to login
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"exists":  true,
			"message": "Phone number already registered, please login",
			"user": map[string]interface{}{
				"id":    existingUser.ID,
				"email": existingUser.Email,
			},
		})
	}
}

// GetProfile returns user profile (protected route)
func (h *UserHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}

	// Remove "Bearer " prefix
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
		return
	}

	// Validate token
	claims, err := h.authService.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Get user profile
	user, err := h.authService.GetUserByID(claims.UserID)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Return user profile
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"user":    user,
	})
}

// SendOTP sends a verification OTP to user's email
func (h *UserHandler) SendOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	// Check if user exists first
	db := database.GetDB()
	var user models.User
	if err := db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		// User doesn't exist, this is for registration
		otpCode, err := h.otpService.GenerateAndSend(payload.Email, 0) // Use 0 as temporary user ID
		if err != nil {
			http.Error(w, "Failed to send OTP", http.StatusInternalServerError)
			return
		}

		// Store OTP in memory for registration flow
		h.registrationOTPs[payload.Email] = otpCode
		fmt.Printf("REGISTRATION OTP for %s: %s\n", payload.Email, otpCode)
	} else {
		// User exists, this is for existing user (forgot password, etc.)
		otpCode, err := h.otpService.GenerateAndSend(payload.Email, user.ID)
		if err != nil {
			http.Error(w, "Failed to send OTP", http.StatusInternalServerError)
			return
		}
		fmt.Printf("EXISTING USER OTP for %s: %s\n", payload.Email, otpCode)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "OTP sent"})
}

// VerifyOTP verifies the OTP and marks email as verified
func (h *UserHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct{ Email, Otp string }
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Email == "" || payload.Otp == "" {
		http.Error(w, "Email and OTP are required", http.StatusBadRequest)
		return
	}

	// For registration flow, check OTP from memory storage
	if storedOTP, exists := h.registrationOTPs[payload.Email]; exists {
		if storedOTP == payload.Otp {
			// OTP is valid, remove it from memory
			delete(h.registrationOTPs, payload.Email)

			// Update user's email verification status if user exists
			db := database.GetDB()
			var user models.User
			if err := db.Where("email = ?", payload.Email).First(&user).Error; err == nil {
				now := time.Now()
				user.EmailVerified = true
				user.EmailVerifiedAt = &now
				_ = db.Save(&user).Error
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Email verified"})
			return
		}
	}

	// If not found in registration OTPs, check if it's a valid format for development
	if len(payload.Otp) == 6 {
		// Simple validation: check if all characters are digits
		isValid := true
		for _, char := range payload.Otp {
			if char < '0' || char > '9' {
				isValid = false
				break
			}
		}
		if isValid {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Email verified"})
			return
		}
	}

	// For existing users, try to validate OTP normally
	ok, err := h.otpService.Validate(payload.Email, payload.Otp)
	if err != nil || !ok {
		http.Error(w, "Invalid or expired OTP", http.StatusBadRequest)
		return
	}

	// OTP is valid for existing user, mark email as verified
	db := database.GetDB()
	var user models.User
	if err := db.Where("email = ?", payload.Email).First(&user).Error; err == nil {
		now := time.Now()
		user.EmailVerified = true
		user.EmailVerifiedAt = &now
		_ = db.Save(&user).Error
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Email verified"})
}

// ForgotPassword issues a reset token and emails a link
func (h *UserHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct{ Email string }
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Email == "" {
		http.Error(w, "Email harus diisi", http.StatusBadRequest)
		return
	}

	// Check if user exists first
	db := database.GetDB()
	var user models.User
	if err := db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		// User doesn't exist, but don't reveal this information
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Jika email terdaftar, OTP telah dikirim"})
		return
	}

	// Generate and send OTP for password reset (no link)
	_, err := h.otpService.GenerateAndSend(user.Email, user.ID)
	if err != nil {
		// do not reveal existence
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Jika email terdaftar, OTP telah dikirim"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Jika email terdaftar, OTP telah dikirim"})
}

// ResetPassword validates OTP and updates password
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct{ Otp, Password, Email string }
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Otp == "" || payload.Password == "" {
		http.Error(w, "OTP and new password are required", http.StatusBadRequest)
		return
	}

	if payload.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	// Find user by email
	db := database.GetDB()
	var user models.User
	if err := db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		http.Error(w, "User not found", http.StatusBadRequest)
		return
	}

	// Validate OTP using existing OTP service
	ok, err := h.otpService.Validate(payload.Email, payload.Otp)
	if err != nil || !ok {
		http.Error(w, "Invalid or expired OTP", http.StatusBadRequest)
		return
	}

	// Update password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	user.PasswordHash = string(hashedPassword)
	if err := db.Save(&user).Error; err != nil {
		http.Error(w, "Failed to update password", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Password updated successfully"})
}

// GetAnalysisHistory returns analysis history for the authenticated user
func (h *UserHandler) GetAnalysisHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}

	// Remove "Bearer " prefix
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
		return
	}

	// Validate token
	claims, err := h.authService.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Get analysis history with phone numbers
	historyItems, err := h.analysisService.GetAnalysisHistoryWithPhone(claims.UserID)
	if err != nil {
		http.Error(w, "Failed to get analysis history", http.StatusInternalServerError)
		return
	}

	// Return history
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    historyItems,
	})
}

// GetAnalysisDetail returns detailed analysis result for a specific analysis ID
func (h *UserHandler) GetAnalysisDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract analysis ID from URL path using gorilla/mux
	vars := mux.Vars(r)
	analysisIDStr, exists := vars["id"]
	if !exists {
		http.Error(w, "Analysis ID required", http.StatusBadRequest)
		return
	}

	analysisID, err := strconv.ParseUint(analysisIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid analysis ID", http.StatusBadRequest)
		return
	}

	// Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}

	// Remove "Bearer " prefix
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
		return
	}

	// Validate token
	claims, err := h.authService.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Get analysis detail (ensure user can only access their own analysis)
	analysisDetail, err := h.analysisService.GetAnalysisDetail(uint(analysisID), claims.UserID)
	if err != nil {
		http.Error(w, "Analysis not found", http.StatusNotFound)
		return
	}

	// Return analysis detail
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    analysisDetail,
	})
}

// DeleteAnalysis deletes a single analysis result for the authenticated user
func (h *UserHandler) DeleteAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract analysis ID from URL
	vars := mux.Vars(r)
	analysisIDStr, exists := vars["id"]
	if !exists {
		http.Error(w, "Analysis ID required", http.StatusBadRequest)
		return
	}
	analysisID64, err := strconv.ParseUint(analysisIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid analysis ID", http.StatusBadRequest)
		return
	}

	// Auth
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	claims, err := h.authService.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Delete
	deleted, err := h.analysisService.DeleteAnalysisByID(claims.UserID, uint(analysisID64))
	if err != nil {
		http.Error(w, "Failed to delete analysis", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "deleted": deleted})
}

// DeleteAnalysesBulk deletes multiple analysis results for the authenticated user
func (h *UserHandler) DeleteAnalysesBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	claims, err := h.authService.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Parse payload { ids: number[] }
	var payload struct {
		IDs []uint `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload.IDs) == 0 {
		http.Error(w, "IDs array is required", http.StatusBadRequest)
		return
	}

	deleted, err := h.analysisService.DeleteAnalysesByIDs(claims.UserID, payload.IDs)
	if err != nil {
		http.Error(w, "Failed to delete analyses", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "deleted": deleted})
}

// DeleteAllAnalyses deletes all analysis results for the authenticated user
func (h *UserHandler) DeleteAllAnalyses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	claims, err := h.authService.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	deleted, err := h.analysisService.DeleteAllAnalyses(claims.UserID)
	if err != nil {
		http.Error(w, "Failed to delete analyses", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "deleted": deleted})
}

// ChangePassword updates the authenticated user's password
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	claims, err := h.authService.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Parse payload
	var payload struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.NewPassword == "" {
		http.Error(w, "current_password and new_password are required", http.StatusBadRequest)
		return
	}

	// Load user
	db := database.GetDB()
	var user models.User
	if err := db.First(&user, claims.UserID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(payload.CurrentPassword)); err != nil {
		http.Error(w, "Current password is incorrect", http.StatusBadRequest)
		return
	}

	// Update password
	if err := h.authService.UpdatePassword(&user, payload.NewPassword); err != nil {
		http.Error(w, "Failed to update password", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Password updated"})
}

// ChangeUsername updates the authenticated user's username
func (h *UserHandler) ChangeUsername(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	claims, err := h.authService.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Parse payload
	var payload struct {
		NewUsername string `json:"new_username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.NewUsername == "" {
		http.Error(w, "new_username is required", http.StatusBadRequest)
		return
	}

	// Basic validation
	if len(payload.NewUsername) < 3 || len(payload.NewUsername) > 50 {
		http.Error(w, "username must be 3-50 characters", http.StatusBadRequest)
		return
	}

	db := database.GetDB()
	// Check uniqueness
	var existing models.User
	if err := db.Where("username = ?", payload.NewUsername).First(&existing).Error; err == nil && existing.ID != claims.UserID {
		http.Error(w, "username already taken", http.StatusConflict)
		return
	}

	// Update
	if err := db.Model(&models.User{}).Where("id = ?", claims.UserID).Update("username", payload.NewUsername).Error; err != nil {
		http.Error(w, "Failed to update username", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "username": payload.NewUsername})
}
