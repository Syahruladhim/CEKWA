package services

import (
	"fmt"
	"os"
	"strings"
	"time"

	"back_wa/internal/models"

	"gorm.io/gorm"
)

type PaymentService struct {
	xenditService *XenditService
	db            *gorm.DB
}

func NewPaymentService(db *gorm.DB) *PaymentService {
	return &PaymentService{
		xenditService: NewXenditService(),
		db:            db,
	}
}

func (ps *PaymentService) CreatePayment(req models.CreatePaymentRequest, userID int) (*models.CreatePaymentResponse, error) {
	fmt.Printf("💰 Creating payment for user %d: %+v\n", userID, req)

	// Generate external ID
	externalID := fmt.Sprintf("cekwa_%d_%d", userID, time.Now().Unix())
	fmt.Printf("🆔 Generated external ID: %s\n", externalID)

	// Create Xendit invoice request
	// Build redirect URLs from environment to avoid hardcoded localhost
	frontendBaseURL := os.Getenv("FRONTEND_BASE_URL")
	if frontendBaseURL == "" {
		frontendBaseURL = "http://localhost:3000"
	}

	// Map selected payment method; if empty/auto, let Xendit decide by omitting
	mappedMethods := ps.mapPaymentMethodToXendit(req.PaymentMethod)

	xenditReq := models.XenditInvoiceRequest{
		ExternalID:      externalID,
		Amount:          req.Amount,
		Description:     req.Category,
		InvoiceDuration: 24, // 24 hours
		Customer: models.XenditCustomer{
			GivenNames: "Customer",
			Email:      req.Email,
		},
		CustomerNotificationPreference: models.XenditNotificationPreference{
			InvoiceCreated:  []string{"email"},
			InvoiceReminder: []string{"email"},
			InvoicePaid:     []string{"email"},
			InvoiceExpired:  []string{"email"},
		},
		SuccessRedirectURL: fmt.Sprintf("%s/dashboard/transaksi?status=success", frontendBaseURL),
		FailureRedirectURL: fmt.Sprintf("%s/dashboard/transaksi?status=failed", frontendBaseURL),
		PaymentMethods:     mappedMethods,
		ShouldSendEmail:    true,
		Items: []models.XenditItem{
			{
				Name:     req.Category,
				Quantity: 1,
				Price:    req.Amount,
				Category: req.Category,
			},
		},
	}

	fmt.Printf("📋 Xendit request prepared: %+v\n", xenditReq)

	// Create invoice via Xendit
	fmt.Printf("🔄 Calling Xendit API...\n")
	invoiceResp, err := ps.xenditService.CreateInvoice(xenditReq)
	if err != nil {
		fmt.Printf("❌ Xendit API failed: %v\n", err)
		return nil, fmt.Errorf("xendit_error: %v", err)
	}
	fmt.Printf("✅ Xendit invoice created: %s\n", invoiceResp.ID)

	// Save transaction to database
	transaction := models.Transaction{
		UserID:        userID,
		ExternalID:    externalID,
		InvoiceID:     invoiceResp.ID,
		Amount:        req.Amount,
		Currency:      "IDR",
		Status:        "pending",
		PaymentMethod: req.PaymentMethod,
		Description:   req.Category,
		PhoneNumber:   req.PhoneNumber,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	fmt.Printf("💾 Saving transaction to database...\n")
	transactionID, err := ps.saveTransaction(transaction)
	if err != nil {
		fmt.Printf("❌ Failed to save transaction: %v\n", err)
		return nil, fmt.Errorf("failed to save transaction: %v", err)
	}

	fmt.Printf("✅ Transaction saved with ID: %d\n", transactionID)

	response := &models.CreatePaymentResponse{
		ID:            transactionID,
		ExternalID:    externalID,
		InvoiceID:     invoiceResp.ID,
		InvoiceURL:    invoiceResp.InvoiceURL,
		Amount:        req.Amount,
		Status:        "pending",
		PaymentMethod: req.PaymentMethod,
		CreatedAt:     time.Now(),
		ExpiryDate:    invoiceResp.ExpiryDate, // Now string type
	}

	fmt.Printf("🎉 Payment creation completed successfully: %+v\n", response)
	return response, nil
}

func (ps *PaymentService) GetTransactionByExternalID(externalID string) (*models.Transaction, error) {
	var transaction models.Transaction
	err := ps.db.Where("external_id = ?", externalID).First(&transaction).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("transaction not found")
		}
		return nil, fmt.Errorf("failed to get transaction: %v", err)
	}
	return &transaction, nil
}

// ReconcileTransactionStatusByExternalID checks Xendit for latest invoice status
// and updates local transaction if it has changed. Returns the latest transaction.
func (ps *PaymentService) ReconcileTransactionStatusByExternalID(externalID string) (*models.Transaction, error) {
	// Load current transaction
	current, err := ps.GetTransactionByExternalID(externalID)
	if err != nil {
		return nil, err
	}

	// If not pending, nothing to do
	if strings.ToLower(current.Status) != "pending" {
		return current, nil
	}

	// Query Xendit invoice
	invoice, err := ps.xenditService.GetInvoice(current.InvoiceID)
	if err != nil {
		// Non-fatal: return current transaction, caller can still see current DB state
		fmt.Printf("⚠️ Reconcile skip: fetch invoice failed for %s: %v\n", externalID, err)
		return current, nil
	}

	// Update local status (mapping done inside UpdateTransactionStatus)
	if err := ps.UpdateTransactionStatus(externalID, invoice.Status, current.PaymentChannel); err != nil {
		return current, err
	}

	// Return refreshed transaction
	return ps.GetTransactionByExternalID(externalID)
}

func (ps *PaymentService) UpdateTransactionStatus(externalID, status string, paymentChannel string) error {
	normalized := strings.ToLower(status)
	switch normalized {
	case "paid", "settled", "success", "successful":
		normalized = "paid"
	case "expired":
		normalized = "expired"
	case "failed", "voided", "canceled", "cancelled":
		normalized = "failed"
	case "pending", "unpaid", "open":
		normalized = "pending"
	default:
		// keep as-is but lowercase
	}

	updates := map[string]interface{}{
		"status":          normalized,
		"payment_channel": paymentChannel,
		"updated_at":      time.Now(),
	}

	if normalized == "paid" {
		updates["paid_at"] = time.Now()
	}

	err := ps.db.Model(&models.Transaction{}).Where("external_id = ?", externalID).Updates(updates).Error
	if err != nil {
		return fmt.Errorf("failed to update transaction status: %v", err)
	}
	return nil
}

func (ps *PaymentService) GetUserTransactions(userID int) ([]models.Transaction, error) {
	var transactions []models.Transaction
	err := ps.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&transactions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions: %v", err)
	}
	return transactions, nil
}

// CheckIfUserPaidForPhone checks if user has a paid transaction for specific phone number
func (ps *PaymentService) CheckIfUserPaidForPhone(userID int, phoneNumber string) (bool, error) {
	var count int64
	err := ps.db.Model(&models.Transaction{}).
		Where("user_id = ? AND phone_number = ? AND status = ?", userID, phoneNumber, "paid").
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check payment for phone: %v", err)
	}
	return count > 0, nil
}

// CheckIfUserHasAnyPaidTransaction checks if user has any paid transaction (regardless of phone number)
func (ps *PaymentService) CheckIfUserHasAnyPaidTransaction(userID int) (bool, error) {
	var count int64
	err := ps.db.Model(&models.Transaction{}).
		Where("user_id = ? AND status = ?", userID, "paid").
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check if user has any paid transactions: %v", err)
	}
	return count > 0, nil
}

func (ps *PaymentService) saveTransaction(transaction models.Transaction) (int, error) {
	fmt.Printf("💾 Saving transaction to database: %+v\n", transaction)

	err := ps.db.Create(&transaction).Error
	if err != nil {
		fmt.Printf("❌ Database error: %v\n", err)
		return 0, fmt.Errorf("failed to insert transaction: %v", err)
	}

	fmt.Printf("✅ Transaction saved with ID: %d\n", transaction.ID)
	return transaction.ID, nil
}

func (ps *PaymentService) mapPaymentMethodToXendit(paymentMethod string) []string {
	// Always return nil to let Xendit show all available payment methods
	// This allows users to choose payment method on Xendit's hosted page
	return nil
}
