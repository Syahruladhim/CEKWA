package whatsapp

import (
	"back_wa/internal/models"
	"context"
	"fmt"
	"log"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func (w *WhatsApp) Analyze() (models.AnalysisResult, error) {
	log.Println("DEBUG: Starting WhatsApp analysis...")

	client := w.GetClient()
	if client == nil {
		return models.AnalysisResult{}, fmt.Errorf("WhatsApp not connected")
	}

	// Check if WhatsApp client is ready (logged in)
	if !w.IsReady() {
		log.Println("DEBUG: WhatsApp client not ready, cannot analyze without login")
		return models.AnalysisResult{}, fmt.Errorf("WhatsApp not logged in. Please scan QR code first")
	}

	// Clear cache if client ID changed (new session)
	w.analysisMu.RLock()
	cachedData, exists := w.analysisData["current_session"]
	w.analysisMu.RUnlock()

	if exists {
		if result, ok := cachedData.(models.AnalysisResult); ok {
			// Check if this is from the same session
			if w.isValidCache(result) {
				log.Println("DEBUG: Using valid cached analysis data")
				return result, nil
			} else {
				log.Println("DEBUG: Cache invalid, clearing and re-analyzing")
				w.ClearAnalysisCache()
			}
		}
	}

	log.Println("DEBUG: Getting contacts from WhatsApp...")

	// Get contacts with timeout (reduced from 10s to 5s)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allContacts, err := client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		log.Printf("DEBUG: Error getting contacts: %v", err)
		return models.AnalysisResult{}, fmt.Errorf("failed to get contacts: %v", err)
	}

	log.Printf("DEBUG: Total contacts found: %d", len(allContacts))

	// If no contacts found, return specific error message
	if len(allContacts) == 0 {
		log.Println("DEBUG: No contacts found, cannot analyze empty contact list")
		return models.AnalysisResult{}, fmt.Errorf("contacts not loaded yet. Please wait a moment and try again")
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
				log.Printf("DEBUG: Saved Contact %d: %s (Server: %s, Name: %s, BusinessName: %s)",
					contactCount, jid.String(), jid.Server, contact.FullName, contact.BusinessName)
			}
			if jid.Server == "g.us" {
				groupCount++
				log.Printf("DEBUG: Group found: %s (Name: %s)", jid.String(), contact.FullName)
			}
		} else {
			// Count unsaved contacts (no name or unknown name)
			unsavedContacts[jid] = contact
			unsavedCount++
			if unsavedCount <= 5 { // Log first 5 unsaved contacts
				log.Printf("DEBUG: Unsaved Contact %d: %s (Server: %s)",
					unsavedCount, jid.String(), jid.Server)
			}
		}
	}

	log.Printf("DEBUG: Total saved contacts: %d, Total unsaved contacts: %d, Total groups found: %d",
		len(savedContacts), len(unsavedContacts), groupCount)

	// Use savedContacts for main analysis
	contacts := savedContacts

	// Calculate the 8 required parameters
	totalContacts := len(contacts)
	totalChats := w.estimateTotalChats(contacts)
	totalGroups := w.calculateTotalGroups(contacts)
	totalChatWithContact := w.estimateChatsWithContacts(contacts)
	totalUnsavedChats := len(unsavedContacts)
	unknownNumberChats := len(unsavedContacts)

	// Estimate sensitive content (for now, using a reasonable default)
	sensitiveContentCount := w.estimateSensitiveContent(contacts)

	// Get account age
	accountAgeDays := w.estimateAccountAge(client)

	log.Printf("DEBUG: Calculated parameters:")
	log.Printf("  Total Chats: %d", totalChats)
	log.Printf("  Total Contacts: %d", totalContacts)
	log.Printf("  Account Age: %d days", accountAgeDays)
	log.Printf("  Total Groups: %d", totalGroups)
	log.Printf("  Chat with Contact: %d", totalChatWithContact)
	log.Printf("  Sensitive Content: %d", sensitiveContentCount)
	log.Printf("  Total Unsaved Chats: %d", totalUnsavedChats)
	log.Printf("  Unknown Number Chats: %d", unknownNumberChats)

	// Calculate strength dengan parameter baru sesuai tabel indikator
	log.Println("DEBUG: Calling CalculateStrength...")
	rating, summary := models.CalculateStrength(totalChats, totalContacts, accountAgeDays, totalGroups, totalChatWithContact, sensitiveContentCount, totalUnsavedChats, unknownNumberChats)

	result := models.AnalysisResult{
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
	}

	log.Printf("DEBUG: Analysis result - Strength: %s", rating)

	// Cache the analysis result for current session
	w.analysisMu.Lock()
	w.analysisData["current_session"] = result
	w.analysisMu.Unlock()
	log.Println("DEBUG: Analysis data cached for current session")

	return result, nil
}

// isValidCache checks if the cached data is still valid for current session
func (w *WhatsApp) isValidCache(result models.AnalysisResult) bool {
	// Check if we have meaningful data (not all zeros)
	if result.TotalContacts == 0 && result.TotalChats == 0 {
		log.Println("DEBUG: Cache contains zero data, invalid")
		return false
	}

	// Check if client ID matches (basic session validation)
	if w.client != nil && w.client.Store.ID != nil {
		log.Println("DEBUG: Cache validation passed")
		return true
	}

	log.Println("DEBUG: Cache validation failed - no client ID")
	return false
}

// ClearAnalysisCache clears the analysis data cache
func (w *WhatsApp) ClearAnalysisCache() {
	w.analysisMu.Lock()
	w.analysisData = make(map[string]interface{})
	w.analysisMu.Unlock()
	log.Println("DEBUG: Analysis cache cleared")
}

func (w *WhatsApp) estimateTotalChats(contacts map[types.JID]types.ContactInfo) int {
	// Estimate total chats based on saved contacts only
	savedContactsCount := len(contacts)

	// Estimate total chats as saved contacts + some additional chats
	totalChats := savedContactsCount + int(float64(savedContactsCount)*0.3) // 30% additional chats

	log.Printf("DEBUG: Estimated total chats: %d (from %d saved contacts)", totalChats, savedContactsCount)
	return totalChats
}

func (w *WhatsApp) calculateTotalGroups(contacts map[types.JID]types.ContactInfo) int {
	totalGroups := 0

	// 1. Hitung grup berdasarkan contacts (backup method)
	contactGroups := 0
	for jid := range contacts {
		if jid.Server == "g.us" {
			contactGroups++
			log.Printf("DEBUG: Found group in contacts: %s", jid.String())
		}
	}

	// 2. Coba ambil daftar grup langsung dari client
	if w.client != nil {
		groups, err := w.client.GetJoinedGroups()
		if err != nil {
			log.Printf("DEBUG: Error getting groups from client: %v", err)
		} else {
			totalGroups = len(groups)
			log.Printf("DEBUG: Found %d groups from GetJoinedGroups()", totalGroups)
		}
	}

	// 3. Cek data grup yang disimpan secara lokal (jika ada)
	w.groupsMu.RLock()
	storedGroups := len(w.groups)
	w.groupsMu.RUnlock()

	if storedGroups > 0 {
		log.Printf("DEBUG: Found %d groups in stored data", storedGroups)
		if storedGroups > totalGroups {
			totalGroups = storedGroups
		}
	}

	// 4. Fallback jika client belum bisa ambil grup
	if totalGroups == 0 {
		totalGroups = contactGroups
	}

	log.Printf("DEBUG: Final total groups count: %d (contacts: %d, stored: %d)", totalGroups, contactGroups, storedGroups)
	return totalGroups
}

func (w *WhatsApp) estimateChatsWithContacts(contacts map[types.JID]types.ContactInfo) int {
	// Estimate chats with saved contacts
	savedContactsCount := len(contacts)

	// Estimate that about 80% of saved contacts have active chats
	chatsWithContacts := int(float64(savedContactsCount) * 0.8)

	log.Printf("DEBUG: Estimated chats with contacts: %d (from %d saved contacts)", chatsWithContacts, savedContactsCount)
	return chatsWithContacts
}

func (w *WhatsApp) estimateSensitiveContent(contacts map[types.JID]types.ContactInfo) int {
	// For now, estimate sensitive content based on contacts
	// In real implementation, you would analyze message content
	totalContacts := len(contacts)

	// Estimate 5-15% of contacts might have sensitive content
	sensitiveEstimate := int(float64(totalContacts) * 0.1) // 10% average

	log.Printf("DEBUG: Estimated sensitive content count: %d (from %d contacts)", sensitiveEstimate, totalContacts)
	return sensitiveEstimate
}

func (w *WhatsApp) estimateAccountAge(client *whatsmeow.Client) int {
	// Estimate account age based on multiple data points for better accuracy
	if client.Store.ID == nil {
		log.Printf("DEBUG: No client ID, using default account age: 365 days")
		return 365 // Default to 1 year if no client ID
	}

	var estimatedAge int
	var confidenceScore int // 0-100, higher means more confident

	// Method 1: Estimate based on contact count and patterns
	contacts, err := client.Store.Contacts.GetAllContacts(context.Background())
	if err == nil && len(contacts) > 0 {
		contactCount := len(contacts)

		// Analyze contact patterns
		savedContacts := 0
		unsavedContacts := 0
		groupContacts := 0

		for jid, contact := range contacts {
			if contact.FullName != "" && contact.FullName != "Unknown" {
				savedContacts++
			} else {
				unsavedContacts++
			}

			if jid.Server == "g.us" {
				groupContacts++
			}
		}

		// Calculate age based on multiple factors
		baseAge := 0

		// Factor 1: Total contacts (more contacts = older account)
		if contactCount >= 1000 {
			baseAge = 1095 // 3+ years
		} else if contactCount >= 500 {
			baseAge = 730 // 2+ years
		} else if contactCount >= 200 {
			baseAge = 365 // 1+ year
		} else if contactCount >= 100 {
			baseAge = 180 // 6+ months
		} else if contactCount >= 50 {
			baseAge = 90 // 3+ months
		} else {
			baseAge = 30 // 1+ month
		}

		// Factor 2: Saved vs unsaved contacts ratio (higher ratio = older account)
		savedRatio := float64(savedContacts) / float64(contactCount)
		if savedRatio > 0.8 {
			baseAge += 60 // Bonus for high saved contact ratio
		} else if savedRatio > 0.6 {
			baseAge += 30 // Bonus for moderate saved contact ratio
		}

		// Factor 3: Group participation (more groups = older account)
		if groupContacts >= 50 {
			baseAge += 90 // Bonus for high group participation
		} else if groupContacts >= 20 {
			baseAge += 45 // Bonus for moderate group participation
		} else if groupContacts >= 5 {
			baseAge += 15 // Bonus for some group participation
		}

		// Factor 4: Account maturity indicators
		if contactCount > 0 && savedContacts > 0 {
			// Estimate daily contact growth rate
			estimatedDailyGrowth := float64(contactCount) / 365.0
			if estimatedDailyGrowth > 2.0 {
				baseAge += 120 // Bonus for very active account
			} else if estimatedDailyGrowth > 1.0 {
				baseAge += 60 // Bonus for active account
			} else if estimatedDailyGrowth > 0.5 {
				baseAge += 30 // Bonus for moderately active account
			}
		}

		estimatedAge = baseAge
		confidenceScore = 85 // High confidence for contact-based estimation

		log.Printf("DEBUG: Contact-based age estimation: %d days (contacts: %d, saved: %d, groups: %d)",
			estimatedAge, contactCount, savedContacts, groupContacts)

	} else {
		// Method 2: Fallback to client ID hash with more realistic range
		clientID := client.Store.ID.String()
		hash := 0
		for _, char := range clientID {
			hash += int(char)
		}

		// More realistic range: 90 days to 2 years
		estimatedAge = 90 + (hash % 640) // 90 days to ~2 years
		confidenceScore = 30             // Low confidence for hash-based estimation

		log.Printf("DEBUG: Hash-based fallback age estimation: %d days", estimatedAge)
	}

	// Apply confidence-based adjustments
	if confidenceScore >= 80 {
		// High confidence: keep as is
		log.Printf("DEBUG: High confidence estimation, keeping age: %d days", estimatedAge)
	} else if confidenceScore >= 50 {
		// Medium confidence: apply some randomization to avoid patterns
		variation := estimatedAge / 10 // ±10% variation
		estimatedAge = estimatedAge + (variation * 2) - variation
		log.Printf("DEBUG: Medium confidence, applied variation: %d days", estimatedAge)
	} else {
		// Low confidence: more randomization
		variation := estimatedAge / 5 // ±20% variation
		estimatedAge = estimatedAge + (variation * 2) - variation
		log.Printf("DEBUG: Low confidence, applied higher variation: %d days", estimatedAge)
	}

	// Ensure reasonable bounds (minimum 30 days, maximum 5 years)
	if estimatedAge < 30 {
		estimatedAge = 30
	} else if estimatedAge > 1825 { // 5 years
		estimatedAge = 1825
	}

	log.Printf("DEBUG: Final estimated account age: %d days (%.1f years) with confidence: %d%%",
		estimatedAge, float64(estimatedAge)/365.0, confidenceScore)

	return estimatedAge
}
