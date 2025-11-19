package services

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
)

type EmailService struct{}

func (s *EmailService) SendEmail(to string, subject string, htmlBody string) error {
	username := os.Getenv("EMAIL_USERNAME")
	password := os.Getenv("EMAIL_PASSWORD")
	host := getenv("EMAIL_HOST", "smtp.gmail.com")
	port := getenv("EMAIL_PORT", "587")
	from := getenv("EMAIL_FROM", username)
	fromName := getenv("EMAIL_FROM_NAME", "WhatsApp Defender")

	if username == "" || password == "" {
		return fmt.Errorf("email credentials not configured. Please set EMAIL_USERNAME and EMAIL_PASSWORD environment variables")
	}

	// Log email configuration (without password)
	fmt.Printf("Attempting to send email via %s:%s from %s to %s\n", host, port, username, to)

	addr := net.JoinHostPort(host, port)

	headers := map[string]string{
		"From":         fmt.Sprintf("%s<%s>", safeName(fromName), from),
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=\"UTF-8\"",
	}

	var msg strings.Builder
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	auth := smtp.PlainAuth("", username, password, host)

	// Try STARTTLS on 587
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	if err = client.Hello("localhost"); err != nil {
		return err
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		config := &tls.Config{ServerName: host}
		if err = client.StartTLS(config); err != nil {
			return err
		}
	}
	if ok, _ := client.Extension("AUTH"); ok {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	wc, err := client.Data()
	if err != nil {
		return err
	}
	_, err = wc.Write([]byte(msg.String()))
	if err != nil {
		return err
	}
	if err = wc.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (s *EmailService) SendOTPEmail(to string, code string, expiryMinutes int) error {
	body := fmt.Sprintf(`<h2>Verifikasi Email</h2><p>Kode OTP Anda: <strong>%s</strong></p><p>Berlaku %d menit.</p>`, code, expiryMinutes)
	return s.SendEmail(to, "Kode OTP Verifikasi", body)
}

func (s *EmailService) SendPasswordResetEmail(to string, token string, expiryMinutes int) error {
	baseURL := getenv("APP_BASE_URL", "http://localhost:3000")
	resetLink := baseURL + "/reset-password?token=" + token
	body := fmt.Sprintf(`<h2>Reset Password</h2><p>Klik tautan untuk reset password:</p><p><a href="%s">Reset Password</a></p><p>Link berlaku %d menit.</p>`, resetLink, expiryMinutes)
	return s.SendEmail(to, "Reset Password", body)
}

func safeName(name string) string {
	return strings.ReplaceAll(name, "\n", " ")
}

func getenv(k, d string) string {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	return v
}
