package services

import (
	"fmt"
)

type DevEmailService struct{}

func (s *DevEmailService) SendEmail(to string, subject string, htmlBody string) error {
	fmt.Printf("=== DEVELOPMENT EMAIL ===\n")
	fmt.Printf("To: %s\n", to)
	fmt.Printf("Subject: %s\n", subject)
	fmt.Printf("Body: %s\n", htmlBody)
	fmt.Printf("=======================\n")
	return nil
}

func (s *DevEmailService) SendOTPEmail(to string, code string, expiryMinutes int) error {
	fmt.Printf("=== OTP EMAIL (DEV MODE) ===\n")
	fmt.Printf("To: %s\n", to)
	fmt.Printf("OTP Code: %s\n", code)
	fmt.Printf("Expires in: %d minutes\n", expiryMinutes)
	fmt.Printf("==========================\n")
	return nil
}

func (s *DevEmailService) SendPasswordResetEmail(to string, token string, expiryMinutes int) error {
	fmt.Printf("=== PASSWORD RESET EMAIL (DEV MODE) ===\n")
	fmt.Printf("To: %s\n", to)
	fmt.Printf("Reset Token: %s\n", token)
	fmt.Printf("Expires in: %d minutes\n", expiryMinutes)
	fmt.Printf("=====================================\n")
	return nil
}
