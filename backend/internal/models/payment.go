package models

import (
	"time"
)

type Transaction struct {
	ID             int        `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID         int        `json:"user_id" gorm:"not null"`
	ExternalID     string     `json:"external_id" gorm:"uniqueIndex;not null"`
	InvoiceID      string     `json:"invoice_id" gorm:"not null"`
	Amount         float64    `json:"amount" gorm:"not null"`
	Currency       string     `json:"currency" gorm:"default:IDR"`
	Status         string     `json:"status" gorm:"default:pending"`
	PaymentMethod  string     `json:"payment_method" gorm:"not null"`
	PaymentChannel string     `json:"payment_channel"`
	Description    string     `json:"description"`
	PhoneNumber    string     `json:"phone_number" gorm:"not null"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	PaidAt         *time.Time `json:"paid_at"`
}

type CreatePaymentRequest struct {
	Email         string  `json:"email" validate:"required,email"`
	Category      string  `json:"category" validate:"required"`
	PaymentMethod string  `json:"payment_method" validate:"required"`
	Amount        float64 `json:"amount" validate:"required,min=1000"`
	PhoneNumber   string  `json:"phone_number" validate:"required"`
}

type CreatePaymentResponse struct {
	ID            int       `json:"id"`
	ExternalID    string    `json:"external_id"`
	InvoiceID     string    `json:"invoice_id"`
	InvoiceURL    string    `json:"invoice_url"`
	Amount        float64   `json:"amount"`
	Status        string    `json:"status"`
	PaymentMethod string    `json:"payment_method"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiryDate    string    `json:"expiry_date"` // Changed to string to match Xendit response
	Message       string    `json:"message"`
}

type PaymentStatusResponse struct {
	ID             int        `json:"id"`
	ExternalID     string     `json:"external_id"`
	InvoiceID      string     `json:"invoice_id"`
	Amount         float64    `json:"amount"`
	Status         string     `json:"status"`
	PaymentMethod  string     `json:"payment_method"`
	PaymentChannel string     `json:"payment_channel"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	PaidAt         *time.Time `json:"paid_at"`
}

type TransactionHistoryResponse struct {
	ID             int        `json:"id"`
	ExternalID     string     `json:"external_id"`
	Amount         float64    `json:"amount"`
	Currency       string     `json:"currency"`
	Status         string     `json:"status"`
	PaymentMethod  string     `json:"payment_method"`
	PaymentChannel string     `json:"payment_channel"`
	Description    string     `json:"description"`
    PhoneNumber    string     `json:"phone_number"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	PaidAt         *time.Time `json:"paid_at"`
}

type WebhookPayload struct {
	ID                 string     `json:"id"`
	ExternalID         string     `json:"external_id"`
	UserID             string     `json:"user_id"`
	Status             string     `json:"status"`
	MerchantName       string     `json:"merchant_name"`
	Amount             float64    `json:"amount"`
	Description        string     `json:"description"`
	InvoiceURL         string     `json:"invoice_url"`
	ExpiryDate         time.Time  `json:"expiry_date"`
	Created            time.Time  `json:"created"`
	Updated            time.Time  `json:"updated"`
	Currency           string     `json:"currency"`
	PaidAt             *time.Time `json:"paid_at"`
	PaymentMethod      string     `json:"payment_method"`
	PaymentChannel     string     `json:"payment_channel"`
	PaymentDestination string     `json:"payment_destination"`
}

type PaymentMethod struct {
	ID       int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Name     string `json:"name" gorm:"not null"`
	Type     string `json:"type" gorm:"not null"`
	IsActive bool   `json:"is_active" gorm:"default:true"`
}

type PaymentCategory struct {
	ID       int     `json:"id" gorm:"primaryKey;autoIncrement"`
	Name     string  `json:"name" gorm:"not null"`
	Price    float64 `json:"price" gorm:"not null"`
	IsActive bool    `json:"is_active" gorm:"default:true"`
}

// Xendit API Models
type XenditInvoiceRequest struct {
	ExternalID                     string                       `json:"external_id"`
	Amount                         float64                      `json:"amount"`
	Description                    string                       `json:"description"`
	InvoiceDuration                int                          `json:"invoice_duration"`
	Customer                       XenditCustomer               `json:"customer"`
	CustomerNotificationPreference XenditNotificationPreference `json:"customer_notification_preference"`
	SuccessRedirectURL             string                       `json:"success_redirect_url"`
	FailureRedirectURL             string                       `json:"failure_redirect_url"`
	PaymentMethods                 []string                     `json:"payment_methods,omitempty"`
	ShouldSendEmail                bool                         `json:"should_send_email"`
	Items                          []XenditItem                 `json:"items"`
}

type XenditCustomer struct {
	GivenNames string `json:"given_names"`
	Email      string `json:"email"`
}

type XenditNotificationPreference struct {
	InvoiceCreated  []string `json:"invoice_created"`
	InvoiceReminder []string `json:"invoice_reminder"`
	InvoicePaid     []string `json:"invoice_paid"`
	InvoiceExpired  []string `json:"invoice_expired"`
}

type XenditItem struct {
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
	Category string  `json:"category"`
}

type XenditInvoiceResponse struct {
	ID         string    `json:"id"`
	ExternalID string    `json:"external_id"`
	InvoiceURL string    `json:"invoice_url"`
	Amount     float64   `json:"amount"`
	Status     string    `json:"status"`
	ExpiryDate string    `json:"expiry_date"` // Changed to string to handle different formats
	Created    time.Time `json:"created"`
	Updated    time.Time `json:"updated"`
}
