package services

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"time"

	"back_wa/internal/database"
	"back_wa/internal/models"
)

type OTPService struct {
	email EmailServiceInterface
}

type EmailServiceInterface interface {
	SendOTPEmail(to string, code string, expiryMinutes int) error
	SendPasswordResetEmail(to string, token string, expiryMinutes int) error
}

func NewOTPService() *OTPService {
	// Check if email credentials are configured
	username := os.Getenv("EMAIL_USERNAME")
	password := os.Getenv("EMAIL_PASSWORD")

	var emailService EmailServiceInterface
	if username == "" || password == "" {
		fmt.Println("Email credentials not configured, using development mode")
		emailService = &DevEmailService{}
	} else {
		emailService = &EmailService{}
	}

	return &OTPService{email: emailService}
}

func (s *OTPService) GenerateAndSend(email string, userID uint) (string, error) {
	code := generateNumericCode(getIntEnv("OTP_LENGTH", 6))
	expiry := time.Now().Add(time.Duration(getIntEnv("OTP_EXPIRY_MINUTES", 10)) * time.Minute)

	// For registration flow (userID = 0), we don't update user record
	// For existing users, update user with new OTP
	if userID > 0 {
		db := database.GetDB()
		if err := db.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"otp_code":       code,
			"otp_expires_at": expiry,
		}).Error; err != nil {
			return "", err
		}
	}

	// Try to send email, but don't fail if email sending fails
	if err := s.email.SendOTPEmail(email, code, int(getIntEnv("OTP_EXPIRY_MINUTES", 10))); err != nil {
		// Log the error but don't return it, so OTP is still saved in database
		fmt.Printf("Failed to send OTP email to %s: %v\n", email, err)
		// In development, you might want to print the OTP to console
		fmt.Printf("DEVELOPMENT: OTP for %s is: %s\n", email, code)
	} else {
		fmt.Printf("OTP email sent successfully to %s\n", email)
	}

	return code, nil
}

func (s *OTPService) Validate(email string, code string) (bool, error) {
	db := database.GetDB()
	var user models.User

	// Find user by email and check OTP
	if err := db.Where("email = ? AND otp_code = ? AND otp_expires_at > ?",
		email, code, time.Now()).First(&user).Error; err != nil {
		// For registration flow, user might not exist yet, so just return false
		return false, err
	}

	// Mark email as verified and clear OTP
	now := time.Now()
	if err := db.Model(&user).Updates(map[string]interface{}{
		"email_verified":    true,
		"email_verified_at": &now,
		"otp_code":          nil,
		"otp_expires_at":    nil,
	}).Error; err != nil {
		return false, err
	}

	return true, nil
}

func generateNumericCode(length int) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	seed := binary.LittleEndian.Uint64(buf)
	mod := uint64(1)
	for i := 0; i < length; i++ {
		mod *= 10
	}
	val := seed % mod
	format := fmt.Sprintf("%%0%dd", length)
	return fmt.Sprintf(format, val)
}

func getIntEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		_, err := fmt.Sscanf(v, "%d", &n)
		if err == nil {
			return n
		}
	}
	return def
}
