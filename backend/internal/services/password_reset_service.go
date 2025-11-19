package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"back_wa/internal/database"
	"back_wa/internal/models"

	"golang.org/x/crypto/bcrypt"
)

type PasswordResetService struct {
	email interface {
		SendPasswordResetEmail(to string, token string, expiryMinutes int) error
	}
}

func NewPasswordResetService() *PasswordResetService {
	// Check if email credentials are configured
	username := os.Getenv("EMAIL_USERNAME")
	password := os.Getenv("EMAIL_PASSWORD")

	var emailService interface {
		SendPasswordResetEmail(to string, token string, expiryMinutes int) error
	}
	if username == "" || password == "" {
		fmt.Println("Email credentials not configured, using development mode")
		emailService = &DevEmailService{}
	} else {
		emailService = &EmailService{}
	}

	return &PasswordResetService{email: emailService}
}

func (s *PasswordResetService) GenerateAndSend(email string) (string, error) {
	token := generateResetToken()
	expiry := time.Now().Add(60 * time.Minute) // 60 minutes default

	// Find user by email
	db := database.GetDB()
	var user models.User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		return "", err
	}

	// Update user with new reset token
	if err := db.Model(&user).Updates(map[string]interface{}{
		"reset_token":            token,
		"reset_token_expires_at": expiry,
	}).Error; err != nil {
		return "", err
	}

	// send email
	if err := s.email.SendPasswordResetEmail(email, token, 60); err != nil {
		return "", err
	}
	return token, nil
}

func (s *PasswordResetService) ValidateToken(email string, token string) (bool, error) {
	db := database.GetDB()
	var user models.User

	// Find user by email and check reset token
	if err := db.Where("email = ? AND reset_token = ? AND reset_token_expires_at > ?",
		email, token, time.Now()).First(&user).Error; err != nil {
		return false, err
	}

	return true, nil
}

func (s *PasswordResetService) ResetPassword(email string, token string, newPassword string) error {
	db := database.GetDB()
	var user models.User

	// Find user by email and check reset token
	if err := db.Where("email = ? AND reset_token = ? AND reset_token_expires_at > ?",
		email, token, time.Now()).First(&user).Error; err != nil {
		return err
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// Update password and clear reset token
	if err := db.Model(&user).Updates(map[string]interface{}{
		"password_hash":          hashedPassword,
		"reset_token":            nil,
		"reset_token_expires_at": nil,
	}).Error; err != nil {
		return err
	}

	return nil
}

func generateResetToken() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
