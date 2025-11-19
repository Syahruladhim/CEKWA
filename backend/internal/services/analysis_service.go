package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"back_wa/internal/database"
	"back_wa/internal/models"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// AnalysisService handles WhatsApp analysis for multiple users
type AnalysisService struct{}

// NewAnalysisService creates a new analysis service
func NewAnalysisService() *AnalysisService {
	return &AnalysisService{}
}

// AnalyzeWhatsApp analyzes WhatsApp data for a specific user
func (as *AnalysisService) AnalyzeWhatsApp(userID uint, client *whatsmeow.Client) (*models.AnalysisResult, error) {
	log.Printf("DEBUG: User %d - Starting WhatsApp analysis...", userID)

	if client == nil {
		return nil, fmt.Errorf("WhatsApp client not available")
	}

	// Check if WhatsApp client is ready
	if !client.IsConnected() {
		return nil, fmt.Errorf("WhatsApp not connected")
	}

	// Get contacts with timeout (reduced from 10s to 5s like single-user)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("DEBUG: User %d - Fetching contacts with timeout...", userID)
	allContacts, err := client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		log.Printf("DEBUG: User %d - Error getting contacts: %v", userID, err)
		return nil, fmt.Errorf("failed to get contacts: %v", err)
	}

	log.Printf("DEBUG: User %d - Total contacts found: %d", userID, len(allContacts))

	// If no contacts found, return specific error message
	if len(allContacts) == 0 {
		log.Printf("DEBUG: User %d - No contacts found, cannot analyze empty contact list", userID)
		return nil, fmt.Errorf("contacts not loaded yet. Please wait a moment and try again")
	}

	// Filter saved contacts and count unsaved contacts
	savedContacts := make(map[types.JID]types.ContactInfo)
	unsavedContacts := make(map[types.JID]types.ContactInfo)
	contactCount := 0
	groupCount := 0
	unsavedCount := 0

	for jid, contact := range allContacts {
		// Separate saved and unsaved contacts
		if contact.FullName != "" && contact.FullName != "Unknown" {
			savedContacts[jid] = contact
			contactCount++
			if contactCount <= 10 { // Log first 10 saved contacts
				log.Printf("DEBUG: User %d - Saved Contact %d: %s (Server: %s, Name: %s, BusinessName: %s)",
					userID, contactCount, jid.String(), jid.Server, contact.FullName, contact.BusinessName)
			}
			if jid.Server == "g.us" {
				groupCount++
				log.Printf("DEBUG: User %d - Group found: %s (Name: %s)", userID, jid.String(), contact.FullName)
			}
		} else {
			// Count unsaved contacts (no name or unknown name)
			unsavedContacts[jid] = contact
			unsavedCount++
			if unsavedCount <= 5 { // Log first 5 unsaved contacts
				log.Printf("DEBUG: User %d - Unsaved Contact %d: %s (Server: %s)",
					userID, unsavedCount, jid.String(), jid.Server)
			}
		}
	}

	log.Printf("DEBUG: User %d - Total saved contacts: %d, Total unsaved contacts: %d, Total groups found: %d",
		userID, len(savedContacts), len(unsavedContacts), groupCount)

	// Use savedContacts for main analysis
	contacts := savedContacts

	// Calculate the 8 required parameters
	totalContacts := len(contacts)
	totalChats := as.estimateTotalChats(contacts)
	totalGroups := as.calculateTotalGroups(contacts)
	totalChatWithContact := as.estimateChatsWithContacts(contacts)
	totalUnsavedChats := len(unsavedContacts)
	unknownNumberChats := len(unsavedContacts)

	// Estimate sensitive content (for now, using a reasonable default)
	sensitiveContentCount := as.estimateSensitiveContent(contacts)

	// Get account age
	accountAgeDays := as.estimateAccountAge(client)

	log.Printf("DEBUG: User %d - Calculated parameters:", userID)
	log.Printf("  Total Chats: %d", totalChats)
	log.Printf("  Total Contacts: %d", totalContacts)
	log.Printf("  Account Age: %d days", accountAgeDays)
	log.Printf("  Total Groups: %d", totalGroups)
	log.Printf("  Chat with Contact: %d", totalChatWithContact)
	log.Printf("  Sensitive Content: %d", sensitiveContentCount)
	log.Printf("  Total Unsaved Chats: %d", totalUnsavedChats)
	log.Printf("  Unknown Number Chats: %d", unknownNumberChats)

	// Calculate strength dengan parameter baru sesuai tabel indikator
	log.Printf("DEBUG: User %d - Calling CalculateStrength...", userID)
	rating, summary := models.CalculateStrength(totalChats, totalContacts, accountAgeDays, totalGroups, totalChatWithContact, sensitiveContentCount, totalUnsavedChats, unknownNumberChats)

	result := models.AnalysisResult{
		UserID:                userID,
		TotalChats:            totalChats,
		TotalContacts:         totalContacts,
		AccountAgeDays:        accountAgeDays,
		TotalGroups:           totalGroups,
		TotalChatWithContact:  totalChatWithContact,
		SensitiveContentCount: sensitiveContentCount,
		TotalUnsavedChats:     totalUnsavedChats,
		UnknownNumberChats:    unknownNumberChats,
		Strength:              rating,
		Summary:               summary,
		ScanDate:              time.Now(),
	}

	log.Printf("DEBUG: User %d - Analysis result - Strength: %s", userID, rating)

	// Save analysis result to database
	if err := as.saveAnalysisResult(&result); err != nil {
		log.Printf("WARNING: User %d - Failed to save analysis result: %v", userID, err)
	}

	return &result, nil
}

// saveAnalysisResult saves analysis result to database
func (as *AnalysisService) saveAnalysisResult(result *models.AnalysisResult) error {
	// Check and reconnect database if needed
	if err := database.CheckAndReconnect(); err != nil {
		log.Printf("WARNING: Failed to check database connection: %v", err)
	}

	db := database.GetDB()
	return db.Create(result).Error
}

// SaveAnalysisResult saves analysis result to database (public method)
func (as *AnalysisService) SaveAnalysisResult(result *models.AnalysisResult) error {
	return as.saveAnalysisResult(result)
}

// HistoryItem represents a history item with phone number
type HistoryItem struct {
	ID          uint      `json:"id"`
	PhoneNumber string    `json:"phone_number"`
	ScanDate    time.Time `json:"scan_date"`
	Strength    string    `json:"strength"`
}

// GetAnalysisHistory returns analysis history for a user
func (as *AnalysisService) GetAnalysisHistory(userID uint) ([]models.AnalysisResult, error) {
	db := database.GetDB()

	var results []models.AnalysisResult
	err := db.Where("user_id = ?", userID).
		Order("scan_date DESC").
		Find(&results).Error

	return results, err
}

// GetAnalysisHistoryWithPhone returns analysis history with phone numbers for a user
func (as *AnalysisService) GetAnalysisHistoryWithPhone(userID uint) ([]HistoryItem, error) {
	db := database.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	var historyItems []HistoryItem
	err := db.Table("analysis_results ar").
		Select("ar.id, COALESCE(sh.phone_number, '') as phone_number, ar.scan_date, ar.strength").
		Joins("LEFT JOIN scan_history sh ON ar.scan_history_id = sh.id").
		Where("ar.user_id = ?", userID).
		Order("ar.scan_date DESC").
		Scan(&historyItems).Error

	return historyItems, err
}

// GetLatestAnalysis returns the latest analysis for a user
func (as *AnalysisService) GetLatestAnalysis(userID uint) (*models.AnalysisResult, error) {
	db := database.GetDB()

	var result models.AnalysisResult
	err := db.Where("user_id = ?", userID).
		Order("scan_date DESC").
		First(&result).Error

	if err != nil {
		return nil, err
	}

	return &result, nil
}

// GetAnalysisDetail returns analysis details by ID for a specific user
func (as *AnalysisService) GetAnalysisDetail(analysisID uint, userID uint) (*models.AnalysisResult, error) {
	db := database.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	var result models.AnalysisResult
	err := db.Preload("ScanHistory").
		Where("id = ? AND user_id = ?", analysisID, userID).
		First(&result).Error

	if err != nil {
		return nil, err
	}

	return &result, nil
}

// DeleteAnalysisByID deletes a single analysis result by ID for a specific user
func (as *AnalysisService) DeleteAnalysisByID(userID uint, analysisID uint) (int64, error) {
	db := database.GetDB()
	if db == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	// Load the analysis to get its scan_history_id
	var analysisResult models.AnalysisResult
	if err := db.Where("id = ? AND user_id = ?", analysisID, userID).First(&analysisResult).Error; err != nil {
		return 0, err
	}

	// Hard delete the analysis result
	res := db.Unscoped().Where("id = ? AND user_id = ?", analysisID, userID).Delete(&models.AnalysisResult{})
	if res.Error != nil {
		return res.RowsAffected, res.Error
	}

	// If this analysis had a scan_history reference, remove the scan_history
	// only if no other analysis_results still reference it
	if analysisResult.ScanHistoryID != nil {
		var remaining int64
		db.Model(&models.AnalysisResult{}).Where("scan_history_id = ?", *analysisResult.ScanHistoryID).Count(&remaining)
		if remaining == 0 {
			// Hard delete scan_history
			db.Unscoped().Where("id = ? AND user_id = ?", *analysisResult.ScanHistoryID, userID).Delete(&models.ScanHistory{})
		}
	}

	return res.RowsAffected, res.Error
}

// DeleteAnalysesByIDs deletes multiple analysis results by IDs for a specific user
func (as *AnalysisService) DeleteAnalysesByIDs(userID uint, ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	db := database.GetDB()
	if db == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	// Collect referenced scan_history_ids prior to deletion
	var toDelete []models.AnalysisResult
	if err := db.Select("scan_history_id").Where("user_id = ? AND id IN ?", userID, ids).Find(&toDelete).Error; err != nil {
		return 0, err
	}
	refCounts := map[uint]int{}
	for _, ar := range toDelete {
		if ar.ScanHistoryID != nil {
			refCounts[*ar.ScanHistoryID]++
		}
	}

	// Hard delete analysis_results
	res := db.Unscoped().Where("user_id = ? AND id IN ?", userID, ids).Delete(&models.AnalysisResult{})
	if res.Error != nil {
		return res.RowsAffected, res.Error
	}

	// For each scan_history_id referenced, delete scan_history if no analysis_results remain
	for scanID := range refCounts {
		var remaining int64
		db.Model(&models.AnalysisResult{}).Where("scan_history_id = ?", scanID).Count(&remaining)
		if remaining == 0 {
			db.Unscoped().Where("id = ? AND user_id = ?", scanID, userID).Delete(&models.ScanHistory{})
		}
	}

	return res.RowsAffected, nil
}

// DeleteAllAnalyses deletes all analysis results for a specific user
func (as *AnalysisService) DeleteAllAnalyses(userID uint) (int64, error) {
	db := database.GetDB()
	if db == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	// Get distinct scan_history_ids for this user
	var scanIDs []uint
	if err := db.Model(&models.AnalysisResult{}).
		Where("user_id = ? AND scan_history_id IS NOT NULL", userID).
		Pluck("scan_history_id", &scanIDs).Error; err != nil {
		return 0, err
	}

	// Hard delete all analysis_results for this user
	res := db.Unscoped().Where("user_id = ?", userID).Delete(&models.AnalysisResult{})
	if res.Error != nil {
		return res.RowsAffected, res.Error
	}

	if len(scanIDs) > 0 {
		// After deleting, ensure no remaining references to these scan histories
		for _, scanID := range scanIDs {
			var remaining int64
			db.Model(&models.AnalysisResult{}).Where("scan_history_id = ?", scanID).Count(&remaining)
			if remaining == 0 {
				db.Unscoped().Where("id = ? AND user_id = ?", scanID, userID).Delete(&models.ScanHistory{})
			}
		}
	}

	return res.RowsAffected, nil
}

// estimateTotalChats estimates total chats based on saved contacts
func (as *AnalysisService) estimateTotalChats(contacts map[types.JID]types.ContactInfo) int {
	savedContactsCount := len(contacts)

	// Estimate total chats as saved contacts + some additional chats
	totalChats := savedContactsCount + int(float64(savedContactsCount)*0.3) // 30% additional chats

	log.Printf("DEBUG: Estimated total chats: %d (from %d saved contacts)", totalChats, savedContactsCount)
	return totalChats
}

// calculateTotalGroups counts total groups from contacts
func (as *AnalysisService) calculateTotalGroups(contacts map[types.JID]types.ContactInfo) int {
	totalGroups := 0

	for jid := range contacts {
		if jid.Server == "g.us" {
			totalGroups++
		}
	}

	return totalGroups
}

// estimateChatsWithContacts estimates chats with saved contacts
func (as *AnalysisService) estimateChatsWithContacts(contacts map[types.JID]types.ContactInfo) int {
	// Assume most saved contacts have chats
	return len(contacts)
}

// estimateSensitiveContent estimates sensitive content count
func (as *AnalysisService) estimateSensitiveContent(contacts map[types.JID]types.ContactInfo) int {
	// For now, return a reasonable default
	// TODO: Implement actual sensitive content detection
	return 0
}

// estimateAccountAge estimates account age in days
func (as *AnalysisService) estimateAccountAge(client *whatsmeow.Client) int {
	// For now, return a reasonable default
	// TODO: Implement actual account age calculation
	return 365 // 1 year default
}
