package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	_ "modernc.org/sqlite"
)

type WhatsApp struct {
	client   *whatsmeow.Client
	qrCode   string
	ready    bool
	mu       sync.RWMutex
	qrChan   <-chan whatsmeow.QRChannelItem
	stopChan chan bool
	db       *sqlstore.Container
	// Analysis data cache
	analysisData map[string]interface{}
	analysisMu   sync.RWMutex
	// Group data storage
	groups   map[types.JID]string
	groupsMu sync.RWMutex
}

func NewWhatsApp() *WhatsApp {
	return &WhatsApp{
		ready:        false,
		stopChan:     make(chan bool),
		analysisData: make(map[string]interface{}),
		groups:       make(map[types.JID]string),
	}
}

func (w *WhatsApp) Connect() error {
	// Create database connection with proper configuration
	db, err := sqlstore.New(context.Background(), "sqlite", "file:whatsapp_session.db?_pragma=foreign_keys(1)&_pragma=journal_mode=WAL&_pragma=synchronous=NORMAL", nil)
	if err != nil {
		log.Printf("Database error: %v", err)
		return err
	}

	w.db = db

	// Get the first device (will be empty for new session)
	deviceStore, err := db.GetFirstDevice(context.Background())
	if err != nil {
		log.Printf("Device store error: %v", err)
		return err
	}

	// Create client
	client := whatsmeow.NewClient(deviceStore, nil)

	// Check if we have a stored session
	if deviceStore.ID != nil {
		log.Println("DEBUG: Found existing session, attempting to restore...")
		// Try to restore existing session
		err = client.Connect()
		if err != nil {
			log.Printf("DEBUG: Failed to restore session: %v", err)
			// Clear the invalid session
			if clearErr := w.clearInvalidSession(); clearErr != nil {
				log.Printf("DEBUG: Error clearing invalid session: %v", clearErr)
			}
			// Fall through to QR generation
		} else {
			// Session restored successfully, but wait for contacts to be loaded
			w.client = client
			log.Println("DEBUG: Session restored successfully, waiting for contacts to load...")

			// Clear any existing cache since we have a new session
			w.ClearAnalysisCache()

			// Start status monitoring in background
			go w.monitorStatus()

			// Wait for contacts to be loaded before setting ready
			go w.waitForContactsAndSetReady()
			return nil
		}
	}

	// No valid session found, start QR generation
	log.Println("DEBUG: No valid session found, starting QR generation...")
	qrChan, _ := client.GetQRChannel(context.Background())
	err = client.Connect()
	if err != nil {
		log.Printf("Client connect error: %v", err)
		return err
	}

	w.client = client
	w.qrChan = qrChan
	w.ready = false

	log.Println("WhatsApp client connected successfully")

	// Start QR monitoring in background
	go w.monitorQR()

	// Start status monitoring in background
	go w.monitorStatus()

	return nil
}

// waitForContactsAndSetReady waits for contacts to be loaded before setting ready status
func (w *WhatsApp) waitForContactsAndSetReady() {
	log.Println("DEBUG: Starting contact loading check...")

	// Wait for client to be fully connected (reduced from 3s to 1s)
	time.Sleep(1 * time.Second)

	maxAttempts := 5 // Reduced from 10 to 5
	attempt := 0

	for attempt < maxAttempts {
		if w.client == nil {
			log.Println("DEBUG: Client is nil, stopping contact check")
			return
		}

		// Try to get contacts
		contacts, err := w.client.Store.Contacts.GetAllContacts(context.Background())
		if err != nil {
			log.Printf("DEBUG: Error getting contacts (attempt %d): %v", attempt+1, err)
		} else {
			log.Printf("DEBUG: Found %d contacts (attempt %d)", len(contacts), attempt+1)

			// If we have contacts, set ready and stop checking
			if len(contacts) > 0 {
				w.mu.Lock()
				w.ready = true
				w.mu.Unlock()
				log.Println("DEBUG: Contacts loaded successfully, WhatsApp is now ready!")
				return
			}
		}

		attempt++
		log.Printf("DEBUG: Waiting for contacts to load... (attempt %d/%d)", attempt, maxAttempts)
		time.Sleep(1 * time.Second) // Reduced from 2s to 1s
	}

	log.Println("DEBUG: Contact loading timeout, setting ready anyway")
	w.mu.Lock()
	w.ready = true
	w.mu.Unlock()
}

// clearInvalidSession removes invalid session data
func (w *WhatsApp) clearInvalidSession() error {
	if w.db == nil {
		return fmt.Errorf("database not initialized")
	}

	ctx := context.Background()
	devices, err := w.db.GetAllDevices(ctx)
	if err != nil {
		return fmt.Errorf("error getting devices: %w", err)
	}

	for _, device := range devices {
		log.Printf("DEBUG: Clearing invalid device: %s", device.ID)
		err := w.db.DeleteDevice(ctx, device)
		if err != nil {
			log.Printf("DEBUG: Error deleting device: %v", err)
		}
	}

	return nil
}

func (w *WhatsApp) monitorQR() {
	log.Println("DEBUG: Starting QR monitoring...")
	timeoutCount := 0
	maxTimeouts := 5 // Maximum QR timeouts before giving up

	for {
		select {
		case evt := <-w.qrChan:
			log.Printf("DEBUG: QR event received: %s", evt.Event)
			if evt.Event == "code" {
				// Reset timeout count on successful QR generation
				timeoutCount = 0

				// Generate QR code using the library with the actual data
				qrBytes, err := qrcode.Encode(evt.Code, qrcode.Medium, 256)
				if err != nil {
					log.Printf("DEBUG: Error generating QR code: %v", err)
					continue
				}

				w.mu.Lock()
				w.qrCode = "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrBytes)
				w.mu.Unlock()
				log.Printf("DEBUG: New QR code generated successfully with data: %s", evt.Code[:20]+"...")
			} else if evt.Event == "timeout" {
				timeoutCount++
				log.Printf("DEBUG: QR code timeout #%d, will generate new one", timeoutCount)

				w.mu.Lock()
				w.qrCode = ""
				w.mu.Unlock()

				// Check if we've hit the maximum timeout limit
				if timeoutCount >= maxTimeouts {
					log.Printf("DEBUG: Maximum QR timeouts reached (%d), stopping QR generation", maxTimeouts)
					return
				}

				// Exponential backoff: wait longer between each retry
				backoffDelay := time.Duration(timeoutCount*timeoutCount) * time.Second
				if backoffDelay > 30*time.Second {
					backoffDelay = 30 * time.Second // Cap at 30 seconds
				}

				log.Printf("DEBUG: Waiting %v before retrying QR generation", backoffDelay)

				// Attempt automatic QR refresh with exponential backoff
				go func() {
					time.Sleep(backoffDelay)
					if err := w.RefreshQR(); err != nil {
						log.Printf("DEBUG: Error refreshing QR after timeout: %v", err)
					}
				}()
			} else if evt.Event == "error" {
				log.Printf("DEBUG: QR error event received: %v", evt)
				// Don't increment timeout count for errors, just log them
			}
		case <-w.stopChan:
			log.Println("DEBUG: QR monitoring stopped")
			return
		}
	}
}

func (w *WhatsApp) monitorStatus() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if w.client != nil {
				w.mu.Lock()
				oldReady := w.ready
				// Check if user is actually logged in (has valid session)
				w.ready = w.client.Store.ID != nil && w.client.IsConnected()
				if oldReady != w.ready {
					log.Printf("DEBUG: Status changed: %v -> %v", oldReady, w.ready)
					if w.ready {
						log.Println("DEBUG: WhatsApp login detected!")
						// Clear QR code when logged in
						w.qrCode = ""

						// Clear cache when status changes to ready
						w.ClearAnalysisCache()
					}
				}
				w.mu.Unlock()
			}
		case <-w.stopChan:
			log.Println("DEBUG: Status monitoring stopped")
			return
		}
	}
}

func (w *WhatsApp) GetQRCode() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.qrCode
}

func (w *WhatsApp) IsReady() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.ready
}

func (w *WhatsApp) GetClient() *whatsmeow.Client {
	return w.client
}

func (w *WhatsApp) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()

	log.Println("DEBUG: WhatsApp session reset - clearing all data")

	// Stop monitoring
	close(w.stopChan)
	w.stopChan = make(chan bool)

	// Disconnect client
	if w.client != nil {
		log.Println("DEBUG: Disconnecting WhatsApp client...")
		w.client.Disconnect()
	}

	// Clear session from database
	if w.db != nil {
		ctx := context.Background()
		devices, err := w.db.GetAllDevices(ctx)
		if err != nil {
			log.Printf("DEBUG: Error getting devices: %v", err)
		} else {
			for _, device := range devices {
				log.Printf("DEBUG: Deleting device: %s", device.ID)
				err := w.db.DeleteDevice(ctx, device)
				if err != nil {
					log.Printf("DEBUG: Error deleting device: %v", err)
				}
			}
		}
	}

	// Clear analysis data cache
	w.analysisMu.Lock()
	w.analysisData = make(map[string]interface{})
	w.analysisMu.Unlock()
	log.Println("DEBUG: Analysis data cache cleared")

	// Clear group data cache
	w.groupsMu.Lock()
	w.groups = make(map[types.JID]string)
	w.groupsMu.Unlock()
	log.Println("DEBUG: Group data cache cleared")

	// Reset state
	w.client = nil
	w.qrCode = ""
	w.ready = false
	w.db = nil

	log.Println("DEBUG: WhatsApp session reset completed - waiting for frontend to reconnect")

	// ❌ REMOVED: Auto-restart yang menyebabkan conflict
	// Sekarang backend akan menunggu frontend untuk reconnect
	// go func() {
	// 	log.Println("DEBUG: Restarting WhatsApp connection...")
	// 	if err := w.Connect(); err != nil {
	// 		log.Printf("DEBUG: Error reconnecting: %v", err)
	// 	}
	// }()
}

// ✅ NEW: Manual reconnect function untuk frontend control
func (w *WhatsApp) ManualReconnect() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	log.Println("DEBUG: Manual reconnect requested by frontend...")

	// Check if already connected
	if w.client != nil && w.ready {
		log.Println("DEBUG: Already connected, no need to reconnect")
		return nil
	}

	// Start fresh connection
	log.Println("DEBUG: Starting fresh WhatsApp connection...")
	return w.Connect()
}

// RefreshQR forces re-initialization of the QR flow without restarting the server.
// If the client is already logged in (ready), this will be a no-op.
func (w *WhatsApp) RefreshQR() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.client == nil {
		return fmt.Errorf("client not initialized")
	}

	// If already logged in and connected, no QR is needed
	if w.client.Store.ID != nil && w.client.IsConnected() {
		log.Println("DEBUG: RefreshQR skipped: client already logged in and connected")
		return nil
	}

	log.Println("DEBUG: Starting QR refresh...")

	// Disconnect if connected (to restart QR flow)
	if w.client.IsConnected() {
		log.Println("DEBUG: Disconnecting client for QR refresh...")
		w.client.Disconnect()
		time.Sleep(1 * time.Second) // Give more time for disconnect
	}

	// Clear any existing QR code
	w.qrCode = ""

	// Re-acquire QR channel and reconnect to trigger new QR code
	ctx := context.Background()
	qrChan, err := w.client.GetQRChannel(ctx)
	if err != nil {
		return fmt.Errorf("failed to get QR channel: %w", err)
	}
	w.qrChan = qrChan

	// Wait a bit before reconnecting to avoid rate limiting
	time.Sleep(2 * time.Second)

	if err := w.client.Connect(); err != nil {
		return fmt.Errorf("failed to reconnect for QR: %w", err)
	}

	log.Println("DEBUG: QR refresh triggered successfully")
	return nil
}
