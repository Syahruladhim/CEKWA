package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"back_wa/internal/database"
	"back_wa/internal/models"
	"back_wa/internal/services"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

// MultiUserWhatsAppManager manages multiple WhatsApp sessions for different users
type MultiUserWhatsAppManager struct {
	userSessions map[uint]*UserWhatsAppSession
	mu           sync.RWMutex
	authService  *services.AuthService
}

// UserWhatsAppSession represents a WhatsApp session for a specific user
// Using the SAME structure as single-user WhatsApp
type UserWhatsAppSession struct {
	UserID             uint
	Client             *whatsmeow.Client
	SessionDB          *sqlstore.Container
	QRCode             string
	Ready              bool
	Status             string
	LastActivity       time.Time
	LastConnectAttempt time.Time

	// SAME caching system as single-user
	AnalysisCache map[string]interface{}
	AnalysisMu    sync.RWMutex

	// SAME groups storage as single-user
	Groups   map[types.JID]types.GroupInfo
	GroupsMu sync.RWMutex

	mu sync.RWMutex
}

// NewMultiUserWhatsAppManager creates a new multi-user WhatsApp manager
func NewMultiUserWhatsAppManager() *MultiUserWhatsAppManager {
	return &MultiUserWhatsAppManager{
		userSessions: make(map[uint]*UserWhatsAppSession),
		authService:  &services.AuthService{},
	}
}

// GetOrCreateSession gets existing session or creates new one for user
func (m *MultiUserWhatsAppManager) GetOrCreateSession(userID uint) (*UserWhatsAppSession, error) {
	m.mu.RLock()
	session, exists := m.userSessions[userID]
	m.mu.RUnlock()

	if exists {
		return session, nil
	}

	// Create new session
	return m.createNewSession(userID)
}

// createNewSession creates a new WhatsApp session for user
func (m *MultiUserWhatsAppManager) createNewSession(userID uint) (*UserWhatsAppSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if user already has a session
	if existing, exists := m.userSessions[userID]; exists {
		return existing, nil
	}

	// Create new session with SAME structure as single-user
	session := &UserWhatsAppSession{
		UserID:        userID,
		Status:        "disconnected",
		AnalysisCache: make(map[string]interface{}),        // SAME as single-user
		Groups:        make(map[types.JID]types.GroupInfo), // SAME as single-user
		LastActivity:  time.Now(),
	}

	// Initialize database connection for this user
	if err := session.initializeDatabase(); err != nil {
		return nil, fmt.Errorf("failed to initialize database for user %d: %v", userID, err)
	}

	// Store session
	m.userSessions[userID] = session

	// Save to database
	if err := m.saveOrUpdateSessionInDatabase(session); err != nil {
		log.Printf("Warning: Failed to save session to database: %v", err)
	}

	return session, nil
}

// initializeDatabase initializes WhatsApp database for user session
func (s *UserWhatsAppSession) initializeDatabase() error {
	// Allow switching store to Postgres via env
	// WA_STORE_DRIVER: "postgres" or "sqlite" (default: sqlite)
	// WA_STORE_DSN:    e.g. postgres://user:pass@host:5432/db?sslmode=disable
	driver := os.Getenv("WA_STORE_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}

	var (
		db  *sqlstore.Container
		err error
	)

	switch driver {
	case "postgres", "pgx":
		dsn := os.Getenv("WA_STORE_DSN")
		if dsn == "" {
			return fmt.Errorf("WA_STORE_DSN is required when WA_STORE_DRIVER=postgres")
		}
		// Using pgx stdlib driver name "pgx"
		db, err = sqlstore.New(context.Background(), "pgx", dsn, nil)
	default:
		// sqlite per-user fallback (existing behavior)
		dbPath := fmt.Sprintf("whatsapp_session_user_%d.db", s.UserID)
		dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode=WAL&_pragma=synchronous=NORMAL", dbPath)
		db, err = sqlstore.New(context.Background(), "sqlite", dsn, nil)
	}

	if err != nil {
		return err
	}

	s.SessionDB = db
	return nil
}

// saveOrUpdateSessionInDatabase upserts session info to main database by user_id
func (m *MultiUserWhatsAppManager) saveOrUpdateSessionInDatabase(session *UserWhatsAppSession) error {
	// Check and reconnect database if needed
	if err := database.CheckAndReconnect(); err != nil {
		log.Printf("WARNING: Failed to check database connection: %v", err)
	}

	db := database.GetDB()

	waSession := models.WhatsAppSession{
		UserID:       session.UserID,
		Status:       session.Status,
		DeviceID:     fmt.Sprintf("user_%d", session.UserID),
		LastActivity: session.LastActivity,
	}

	var existing models.WhatsAppSession
	if err := db.Where("user_id = ?", session.UserID).First(&existing).Error; err == nil {
		// Update existing
		existing.Status = waSession.Status
		existing.DeviceID = waSession.DeviceID
		existing.LastActivity = waSession.LastActivity
		return db.Save(&existing).Error
	}

	return db.Create(&waSession).Error
}

// Connect connects user's WhatsApp session
func (m *MultiUserWhatsAppManager) Connect(userID uint) error {
	session, err := m.GetOrCreateSession(userID)
	if err != nil {
		return err
	}

	return session.connect()
}

// connect establishes WhatsApp connection for user session
func (s *UserWhatsAppSession) connect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Guard: avoid connect storms
	if s.Status == "connected" {
		return nil
	}
	if s.Status == "scanning" || s.Status == "connecting" {
		return nil
	}
	if time.Since(s.LastConnectAttempt) < 5*time.Second {
		return nil
	}
	s.LastConnectAttempt = time.Now()
	s.Status = "connecting"
	s.LastActivity = time.Now()
	go func(userID uint, status string, ts time.Time) {
		_ = (&MultiUserWhatsAppManager{}).saveOrUpdateSessionInDatabase(&UserWhatsAppSession{UserID: userID, Status: status, LastActivity: ts})
	}(s.UserID, s.Status, s.LastActivity)

	// Get device store
	deviceStore, err := s.SessionDB.GetFirstDevice(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get device store: %v", err)
	}

	// Create client
	client := whatsmeow.NewClient(deviceStore, nil)

	// Check if we have stored session
	if deviceStore.ID != nil {
		log.Printf("DEBUG: User %d - Found existing session, attempting to restore...", s.UserID)

		if err := client.Connect(); err != nil {
			log.Printf("DEBUG: User %d - Failed to restore session: %v", s.UserID, err)
			// Clear invalid session and generate new QR
			if err := s.clearInvalidSession(); err != nil {
				log.Printf("DEBUG: User %d - Error clearing invalid session: %v", s.UserID, err)
			}
		} else {
			// Session restored successfully
			s.Client = client
			s.Status = "connected"
			s.Ready = true
			s.QRCode = ""
			s.LastActivity = time.Now()
			go func(userID uint, status string, ts time.Time) {
				_ = (&MultiUserWhatsAppManager{}).saveOrUpdateSessionInDatabase(&UserWhatsAppSession{UserID: userID, Status: status, LastActivity: ts})
			}(s.UserID, s.Status, s.LastActivity)

			log.Printf("DEBUG: User %d - Session restored successfully", s.UserID)

			// Skip automatic analysis - user must pay first
			log.Printf("DEBUG: User %d - Session restored, but skipping automatic analysis - payment validation required", s.UserID)
			return nil
		}
	}

	// No valid session, generate QR code
	log.Printf("DEBUG: User %d - No valid session found, generating QR code...", s.UserID)

	qrChan, _ := client.GetQRChannel(context.Background())
	if err := client.Connect(); err != nil {
		return fmt.Errorf("failed to connect client: %v", err)
	}

	s.Client = client
	s.Status = "scanning"
	s.LastActivity = time.Now()
	go func(userID uint, status string, ts time.Time) {
		_ = (&MultiUserWhatsAppManager{}).saveOrUpdateSessionInDatabase(&UserWhatsAppSession{UserID: userID, Status: status, LastActivity: ts})
	}(s.UserID, s.Status, s.LastActivity)

	// Wait for QR code
	go s.waitForQR(qrChan)

	return nil
}

// waitForQR waits for QR code and updates session
func (s *UserWhatsAppSession) waitForQR(qrChan <-chan whatsmeow.QRChannelItem) {
	for {
		select {
		case item := <-qrChan:
			if item.Event == "code" {
				// Generate QR code image
				qrCode, err := qrcode.Encode(item.Code, qrcode.Medium, 256)
				if err != nil {
					log.Printf("ERROR: User %d - Failed to generate QR code: %v", s.UserID, err)
					continue
				}

				// Convert to base64
				qrBase64 := base64.StdEncoding.EncodeToString(qrCode)

				s.mu.Lock()
				s.QRCode = "data:image/png;base64," + qrBase64
				s.mu.Unlock()

				log.Printf("DEBUG: User %d - QR code generated", s.UserID)
			} else if item.Event == "success" {
				s.mu.Lock()
				s.Status = "connected"
				s.Ready = true
				s.QRCode = ""
				s.LastActivity = time.Now()
				s.mu.Unlock()

				// persist status
				_ = (&MultiUserWhatsAppManager{}).saveOrUpdateSessionInDatabase(&UserWhatsAppSession{UserID: s.UserID, Status: s.Status, LastActivity: s.LastActivity})

				log.Printf("DEBUG: User %d - WhatsApp connected successfully", s.UserID)

				// Skip automatic analysis - user must pay first
				log.Printf("DEBUG: User %d - WhatsApp connected, but skipping automatic analysis - payment validation required", s.UserID)

				return
			}
		case <-time.After(2 * time.Minute):
			log.Printf("DEBUG: User %d - QR code timeout", s.UserID)
			s.mu.Lock()
			s.Status = "disconnected"
			s.QRCode = ""
			s.mu.Unlock()
			_ = (&MultiUserWhatsAppManager{}).saveOrUpdateSessionInDatabase(&UserWhatsAppSession{UserID: s.UserID, Status: s.Status, LastActivity: time.Now()})
			return
		}
	}
}

// triggerAutomaticAnalysis triggers analysis automatically after WhatsApp connects
// Using the SAME logic as single-user
func (s *UserWhatsAppSession) triggerAutomaticAnalysis() {
	// Wait for WhatsApp to fully load contacts (poll until available)
	log.Printf("DEBUG: User %d - Waiting for contacts to load...", s.UserID)
	time.Sleep(5 * time.Second)

	if s.Client == nil || !s.Client.IsConnected() {
		log.Printf("DEBUG: User %d - Client not connected, skipping automatic analysis", s.UserID)
		return
	}

	// Proactively poll contacts before running full analysis
	// Keep it close to single-user behavior: try once now, and retry once after 5s (~10s total)
	for attempt := 1; attempt <= 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		allContacts, err := s.Client.Store.Contacts.GetAllContacts(ctx)
		cancel()

		if err == nil && len(allContacts) > 0 {
			log.Printf("DEBUG: User %d - Contacts loaded (count=%d) on attempt %d, but skipping automatic analysis - payment validation required", s.UserID, len(allContacts), attempt)
			// Skip automatic analysis - user must pay first
			return
		}

		log.Printf("DEBUG: User %d - Contacts not ready yet (attempt %d/2). Retrying in 5s...", s.UserID, attempt)
		time.Sleep(5 * time.Second)
		if s.Client == nil || !s.Client.IsConnected() {
			log.Printf("DEBUG: User %d - Client disconnected during contact wait. Aborting analysis.", s.UserID)
			return
		}
	}

	log.Printf("ERROR: User %d - Contacts did not load after quick retries. Skipping automatic analysis.", s.UserID)
}

// Analyze - SAME EXACT METHOD as single-user analyzer.go
func (s *UserWhatsAppSession) Analyze() (models.AnalysisResult, error) {
	log.Printf("DEBUG: User %d - Starting WhatsApp analysis...", s.UserID)

	client := s.GetClient()
	if client == nil {
		return models.AnalysisResult{}, fmt.Errorf("WhatsApp not connected")
	}

	// Check if WhatsApp client is ready (logged in)
	if !s.IsReady() {
		log.Printf("DEBUG: User %d - WhatsApp client not ready, cannot analyze without login", s.UserID)
		return models.AnalysisResult{}, fmt.Errorf("WhatsApp not logged in. Please scan QR code first")
	}

	// Clear cache if client ID changed (new session) - SAME as single-user
	s.AnalysisMu.RLock()
	cachedData, exists := s.AnalysisCache["current_session"]
	s.AnalysisMu.RUnlock()

	if exists {
		if result, ok := cachedData.(models.AnalysisResult); ok {
			// Check if this is from the same session
			if s.isValidCache(result) {
				log.Printf("DEBUG: User %d - Using valid cached analysis data", s.UserID)
				return result, nil
			} else {
				log.Printf("DEBUG: User %d - Cache invalid, clearing and re-analyzing", s.UserID)
				s.ClearAnalysisCache()
			}
		}
	}

	log.Printf("DEBUG: User %d - Getting contacts from WhatsApp...", s.UserID)

	// Get contacts with timeout (SAME as single-user)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allContacts, err := client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		log.Printf("DEBUG: User %d - Error getting contacts: %v", s.UserID, err)
		return models.AnalysisResult{}, fmt.Errorf("failed to get contacts: %v", err)
	}

	log.Printf("DEBUG: User %d - Total contacts found: %d", s.UserID, len(allContacts))

	// If no contacts found, return specific error message
	if len(allContacts) == 0 {
		log.Printf("DEBUG: User %d - No contacts found, cannot analyze empty contact list", s.UserID)
		return models.AnalysisResult{}, fmt.Errorf("contacts not loaded yet. Please wait a moment and try again")
	}

	// Filter saved contacts and count unsaved contacts - SAME as single-user
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
					s.UserID, contactCount, jid.String(), jid.Server, contact.FullName, contact.BusinessName)
			}
			if jid.Server == "g.us" {
				groupCount++
				log.Printf("DEBUG: User %d - Group found: %s (Name: %s)", s.UserID, jid.String(), contact.FullName)
			}
		} else {
			// Count unsaved contacts (no name or unknown name)
			unsavedContacts[jid] = contact
			unsavedCount++
			if unsavedCount <= 5 { // Log first 5 unsaved contacts
				log.Printf("DEBUG: User %d - Unsaved Contact %d: %s (Server: %s)",
					s.UserID, unsavedCount, jid.String(), jid.Server)
			}
		}
	}

	log.Printf("DEBUG: User %d - Total saved contacts: %d, Total unsaved contacts: %d, Total groups found: %d",
		s.UserID, len(savedContacts), len(unsavedContacts), groupCount)

	// Use savedContacts for main analysis
	contacts := savedContacts

	// Calculate the 8 required parameters - SAME as single-user
	totalContacts := len(contacts)
	totalChats := s.estimateTotalChats(contacts)
	totalGroups := s.calculateTotalGroups(contacts)
	totalChatWithContact := s.estimateChatsWithContacts(contacts)
	totalUnsavedChats := len(unsavedContacts)
	unknownNumberChats := len(unsavedContacts)

	// Estimate sensitive content (for now, using a reasonable default)
	sensitiveContentCount := s.estimateSensitiveContent(contacts)

	// Get account age
	accountAgeDays := s.estimateAccountAge(client)

	log.Printf("DEBUG: User %d - Calculated parameters:", s.UserID)
	log.Printf("  Total Chats: %d", totalChats)
	log.Printf("  Total Contacts: %d", totalContacts)
	log.Printf("  Account Age: %d days", accountAgeDays)
	log.Printf("  Total Groups: %d", totalGroups)
	log.Printf("  Chat with Contact: %d", totalChatWithContact)
	log.Printf("  Sensitive Content: %d", sensitiveContentCount)
	log.Printf("  Total Unsaved Chats: %d", totalUnsavedChats)
	log.Printf("  Unknown Number Chats: %d", unknownNumberChats)

	// Calculate strength dengan parameter baru sesuai tabel indikator
	log.Printf("DEBUG: User %d - Calling CalculateStrength...", s.UserID)
	rating, summary := models.CalculateStrength(totalChats, totalContacts, accountAgeDays, totalGroups, totalChatWithContact, sensitiveContentCount, totalUnsavedChats, unknownNumberChats)

	result := models.AnalysisResult{
		UserID:                s.UserID,
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

	log.Printf("DEBUG: User %d - Analysis result - Strength: %s", s.UserID, rating)

	// Cache the analysis result for current session - SAME as single-user
	s.AnalysisMu.Lock()
	s.AnalysisCache["current_session"] = result
	s.AnalysisMu.Unlock()
	log.Printf("DEBUG: User %d - Analysis data cached for current session", s.UserID)

	// Create scan history record first
	scanHistoryID, err := s.createScanHistory(client)
	if err != nil {
		log.Printf("WARNING: User %d - Failed to create scan history: %v", s.UserID, err)
	} else {
		// Set scan history ID to analysis result
		result.ScanHistoryID = &scanHistoryID
		log.Printf("DEBUG: User %d - Created scan history with ID: %d", s.UserID, scanHistoryID)
	}

	// Save to database
	analysisService := &services.AnalysisService{}
	if err := analysisService.SaveAnalysisResult(&result); err != nil {
		log.Printf("WARNING: User %d - Failed to save analysis result: %v", s.UserID, err)
	}

	return result, nil
}

// SAME methods as single-user analyzer.go
func (s *UserWhatsAppSession) isValidCache(result models.AnalysisResult) bool {
	// Check if we have meaningful data (not all zeros)
	if result.TotalContacts == 0 && result.TotalChats == 0 {
		log.Printf("DEBUG: User %d - Cache contains zero data, invalid", s.UserID)
		return false
	}

	// Check if client ID matches (basic session validation)
	if s.Client != nil && s.Client.Store.ID != nil {
		log.Printf("DEBUG: User %d - Cache validation passed", s.UserID)
		return true
	}

	log.Printf("DEBUG: User %d - Cache validation failed - no client ID", s.UserID)
	return false
}

// ClearAnalysisCache clears the analysis data cache
func (s *UserWhatsAppSession) ClearAnalysisCache() {
	s.AnalysisMu.Lock()
	s.AnalysisCache = make(map[string]interface{})
	s.AnalysisMu.Unlock()
	log.Printf("DEBUG: User %d - Analysis cache cleared", s.UserID)
}

func (s *UserWhatsAppSession) estimateTotalChats(contacts map[types.JID]types.ContactInfo) int {
	// Estimate total chats based on saved contacts only
	savedContactsCount := len(contacts)

	// Estimate total chats as saved contacts + some additional chats
	totalChats := savedContactsCount + int(float64(savedContactsCount)*0.3) // 30% additional chats

	log.Printf("DEBUG: User %d - Estimated total chats: %d (from %d saved contacts)", s.UserID, totalChats, savedContactsCount)
	return totalChats
}

func (s *UserWhatsAppSession) calculateTotalGroups(contacts map[types.JID]types.ContactInfo) int {
	totalGroups := 0

	// 1. Hitung grup berdasarkan contacts (backup method)
	contactGroups := 0
	for jid := range contacts {
		if jid.Server == "g.us" {
			contactGroups++
			log.Printf("DEBUG: User %d - Found group in contacts: %s", s.UserID, jid.String())
		}
	}

	// 2. Coba ambil daftar grup langsung dari client
	if s.Client != nil {
		groups, err := s.Client.GetJoinedGroups()
		if err != nil {
			log.Printf("DEBUG: User %d - Error getting groups from client: %v", s.UserID, err)
		} else {
			totalGroups = len(groups)
			log.Printf("DEBUG: User %d - Found %d groups from GetJoinedGroups()", s.UserID, totalGroups)
		}
	}

	// 3. Cek data grup yang disimpan secara lokal (jika ada)
	s.GroupsMu.RLock()
	storedGroups := len(s.Groups)
	s.GroupsMu.RUnlock()

	if storedGroups > 0 {
		log.Printf("DEBUG: User %d - Found %d groups in stored data", s.UserID, storedGroups)
		if storedGroups > totalGroups {
			totalGroups = storedGroups
		}
	}

	// 4. Fallback jika client belum bisa ambil grup
	if totalGroups == 0 {
		totalGroups = contactGroups
	}

	log.Printf("DEBUG: User %d - Final total groups count: %d (contacts: %d, stored: %d)", s.UserID, totalGroups, contactGroups, storedGroups)
	return totalGroups
}

func (s *UserWhatsAppSession) estimateChatsWithContacts(contacts map[types.JID]types.ContactInfo) int {
	// Estimate chats with saved contacts
	savedContactsCount := len(contacts)

	// Estimate that about 80% of saved contacts have active chats
	chatsWithContacts := int(float64(savedContactsCount) * 0.8)

	log.Printf("DEBUG: User %d - Estimated chats with contacts: %d (from %d saved contacts)", s.UserID, chatsWithContacts, savedContactsCount)
	return chatsWithContacts
}

func (s *UserWhatsAppSession) estimateSensitiveContent(contacts map[types.JID]types.ContactInfo) int {
	// For now, estimate sensitive content based on contacts
	// In real implementation, you would analyze message content
	totalContacts := len(contacts)

	// Estimate 5-15% of contacts might have sensitive content
	sensitiveEstimate := int(float64(totalContacts) * 0.1) // 10% average

	log.Printf("DEBUG: User %d - Estimated sensitive content count: %d (from %d contacts)", s.UserID, sensitiveEstimate, totalContacts)
	return sensitiveEstimate
}

func (s *UserWhatsAppSession) estimateAccountAge(client *whatsmeow.Client) int {
	// Estimate account age based on multiple data points for better accuracy
	if client.Store.ID == nil {
		log.Printf("DEBUG: User %d - No client ID, using default account age: 365 days", s.UserID)
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

		log.Printf("DEBUG: User %d - Contact-based age estimation: %d days (contacts: %d, saved: %d, groups: %d)",
			s.UserID, estimatedAge, contactCount, savedContacts, groupContacts)

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

		log.Printf("DEBUG: User %d - Hash-based fallback age estimation: %d days", s.UserID, estimatedAge)
	}

	// Apply confidence-based adjustments
	if confidenceScore >= 80 {
		// High confidence: keep as is
		log.Printf("DEBUG: User %d - High confidence estimation, keeping age: %d days", s.UserID, estimatedAge)
	} else if confidenceScore >= 50 {
		// Medium confidence: apply some randomization to avoid patterns
		variation := estimatedAge / 10 // ±10% variation
		estimatedAge = estimatedAge + (variation * 2) - variation
		log.Printf("DEBUG: User %d - Medium confidence, applied variation: %d days", s.UserID, estimatedAge)
	} else {
		// Low confidence: more randomization
		variation := estimatedAge / 5 // ±20% variation
		estimatedAge = estimatedAge + (variation * 2) - variation
		log.Printf("DEBUG: User %d - Low confidence, applied higher variation: %d days", s.UserID, estimatedAge)
	}

	// Ensure reasonable bounds (minimum 30 days, maximum 5 years)
	if estimatedAge < 30 {
		estimatedAge = 30
	} else if estimatedAge > 1825 { // 5 years
		estimatedAge = 1825
	}

	log.Printf("DEBUG: User %d - Final estimated account age: %d days (%.1f years) with confidence: %d%%",
		s.UserID, estimatedAge, float64(estimatedAge)/365.0, confidenceScore)

	return estimatedAge
}

// Helper methods
func (s *UserWhatsAppSession) GetClient() *whatsmeow.Client {
	return s.Client
}

func (s *UserWhatsAppSession) IsReady() bool {
	return s.Ready
}

// clearInvalidSession clears invalid session data
func (s *UserWhatsAppSession) clearInvalidSession() error {
	// Clear session database
	if s.SessionDB != nil {
		// TODO: Implement session clearing logic
	}
	return nil
}

// GetQRCode returns QR code for user
func (m *MultiUserWhatsAppManager) GetQRCode(userID uint) (string, error) {
	session, err := m.GetOrCreateSession(userID)
	if err != nil {
		return "", err
	}

	// If there is no QR yet and not connected, attempt to connect to generate QR
	session.mu.RLock()
	qrAvailable := session.QRCode != ""
	status := session.Status
	session.mu.RUnlock()

	if !qrAvailable && status != "connected" && status != "scanning" && status != "connecting" {
		// Fire-and-forget connect to trigger QR generation
		go func() {
			if err := m.Connect(userID); err != nil {
				log.Printf("ERROR: User %d - Connect failed while fetching QR: %v", userID, err)
			}
		}()
	}

	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.QRCode, nil
}

// GetStatus returns status for user
func (m *MultiUserWhatsAppManager) GetStatus(userID uint) (string, error) {
	session, err := m.GetOrCreateSession(userID)
	if err != nil {
		return "", err
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	return session.Status, nil
}

// IsReady checks if user's WhatsApp is ready
func (m *MultiUserWhatsAppManager) IsReady(userID uint) bool {
	session, err := m.GetOrCreateSession(userID)
	if err != nil {
		return false
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	return session.Ready
}

// Logout disconnects user's WhatsApp session
func (m *MultiUserWhatsAppManager) Logout(userID uint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.userSessions[userID]
	if !exists {
		log.Printf("DEBUG: User %d - No session found in memory, updating database only", userID)
		// Update database even if no session in memory
		db := database.GetDB()
		db.Model(&models.WhatsAppSession{}).
			Where("user_id = ?", userID).
			Update("status", "disconnected")
		return nil
	}

	log.Printf("DEBUG: User %d - Logging out session", userID)

	// Fully logout & disconnect client
	if session.Client != nil {
		log.Printf("DEBUG: User %d - Logging out & disconnecting WhatsApp client", userID)
		// Ignore panics/errors from underlying client
		func() { defer func() { recover() }(); _ = session.Client.Logout(context.Background()) }()
		func() { defer func() { recover() }(); session.Client.Disconnect() }()
	}

	// If using sqlite store, remove local persisted files so session cannot auto-restore
	if os.Getenv("WA_STORE_DRIVER") == "" || os.Getenv("WA_STORE_DRIVER") == "sqlite" {
		storeFile := fmt.Sprintf("whatsapp_session_user_%d.db", session.UserID)
		_ = os.Remove(storeFile)
		_ = os.Remove(storeFile + "-wal")
		_ = os.Remove(storeFile + "-shm")
	}

	// Clear in-memory session data
	session.Status = "disconnected"
	session.Ready = false
	session.QRCode = ""
	session.ClearAnalysisCache()

	// Remove from memory cache
	delete(m.userSessions, userID)

	// Remove persisted session record to avoid auto-restore semantics
	db := database.GetDB()
	if err := db.Where("user_id = ?", userID).Delete(&models.WhatsAppSession{}).Error; err != nil {
		log.Printf("WARNING: User %d - Failed to delete WhatsAppSession row: %v", userID, err)
	}

	log.Printf("DEBUG: User %d - WhatsApp session fully logged out and wiped", userID)
	return nil
}

// GetClient returns WhatsApp client for user
func (m *MultiUserWhatsAppManager) GetClient(userID uint) *whatsmeow.Client {
	session, err := m.GetOrCreateSession(userID)
	if err != nil {
		return nil
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	return session.Client
}

// GetSessionInfo returns session information for debugging
func (m *MultiUserWhatsAppManager) GetSessionInfo(userID uint) map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.userSessions[userID]
	if !exists {
		return map[string]interface{}{
			"exists": false,
		}
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	return map[string]interface{}{
		"exists":        true,
		"status":        session.Status,
		"ready":         session.Ready,
		"has_qr":        session.QRCode != "",
		"has_client":    session.Client != nil,
		"last_activity": session.LastActivity,
	}
}

// GetCachedAnalysis returns cached analysis result for user
func (m *MultiUserWhatsAppManager) GetCachedAnalysis(userID uint) (*models.AnalysisResult, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.userSessions[userID]
	if !exists {
		return nil, false
	}

	session.AnalysisMu.RLock()
	defer session.AnalysisMu.RUnlock()

	if cachedData, exists := session.AnalysisCache["current_session"]; exists {
		if result, ok := cachedData.(models.AnalysisResult); ok {
			// Check if the cached result is valid (not empty)
			if result.TotalContacts > 0 || result.TotalChats > 0 {
				return &result, true
			}
		}
	}

	return nil, false
}

// IsAnalysisReady checks if analysis is ready for user
func (m *MultiUserWhatsAppManager) IsAnalysisReady(userID uint) bool {
	_, exists := m.GetCachedAnalysis(userID)
	return exists
}

// createScanHistory creates a scan history record for the current WhatsApp session
func (s *UserWhatsAppSession) createScanHistory(client *whatsmeow.Client) (uint, error) {
	// Check and reconnect database if needed
	if err := database.CheckAndReconnect(); err != nil {
		log.Printf("WARNING: Failed to check database connection: %v", err)
	}

	db := database.GetDB()

	// Extract phone number from WhatsApp client
	var phoneNumber string
	if client.Store.ID != nil {
		// Extract phone number from JID format (e.g., "6288226369359:69@s.w" -> "6288226369359")
		clientID := client.Store.ID.String()
		// Split by ':' and take the first part (phone number)
		parts := strings.Split(clientID, ":")
		if len(parts) > 0 {
			// Add '+' prefix to phone number
			phoneNumber = "+" + parts[0]
		} else {
			phoneNumber = "+" + clientID
		}
		log.Printf("DEBUG: User %d - Extracted phone number: %s (from JID: %s)", s.UserID, phoneNumber, clientID)
	} else {
		phoneNumber = "Unknown"
		log.Printf("WARNING: User %d - Could not extract phone number from client", s.UserID)
	}

	// Create scan history record
	scanHistory := models.ScanHistory{
		UserID:      s.UserID,
		PhoneNumber: phoneNumber,
		ScanDate:    time.Now(),
		Status:      "success",
		ResultData:  "{}", // Empty JSON for now
		ErrorMsg:    "",
	}

	// Save to database
	if err := db.Create(&scanHistory).Error; err != nil {
		return 0, fmt.Errorf("failed to create scan history: %v", err)
	}

	log.Printf("DEBUG: User %d - Created scan history record with ID: %d, Phone: %s", s.UserID, scanHistory.ID, phoneNumber)
	return scanHistory.ID, nil
}
