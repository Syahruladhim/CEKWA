package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

func (w *WhatsApp) HandleQR(wr http.ResponseWriter, r *http.Request) {
	qrCode := w.GetQRCode()

	log.Printf("QR request received, QR available: %v", qrCode != "")

	if qrCode == "" {
		// QR code belum tersedia
		json.NewEncoder(wr).Encode(map[string]interface{}{
			"qr":      "",
			"message": "QR Code sedang dibuat, silakan coba lagi dalam beberapa detik.",
			"ready":   w.IsReady(),
		})
		return
	}

	// Return QR code langsung
	json.NewEncoder(wr).Encode(map[string]string{"qr": qrCode})
}

func (w *WhatsApp) HandleStatus(wr http.ResponseWriter, r *http.Request) {
	client := w.GetClient()
	status := w.IsReady()

	// Check if contacts are loaded with faster response
	contactsReady := false
	contactCount := 0

	if client != nil && status {
		// Use context with timeout for faster response
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Try to get contacts to check if they're loaded
		contacts, err := client.Store.Contacts.GetAllContacts(ctx)
		if err == nil {
			contactCount = len(contacts)
			contactsReady = contactCount > 0
		} else {
			log.Printf("DEBUG: Error getting contacts in status check: %v", err)
		}
	}

	response := map[string]interface{}{
		"ready":            status,
		"contacts_ready":   contactsReady,
		"contact_count":    contactCount,
		"client_available": client != nil,
		"timestamp":        time.Now().Format(time.RFC3339),
	}

	log.Printf("DEBUG: Status request - ready: %v, contacts_ready: %v, contact_count: %d",
		status, contactsReady, contactCount)

	wr.Header().Set("Content-Type", "application/json")
	json.NewEncoder(wr).Encode(response)
}

func (w *WhatsApp) HandleAnalyze(wr http.ResponseWriter, r *http.Request) {
	log.Println("DEBUG: HandleAnalyze called - starting analysis...")

	// Check if WhatsApp client exists
	if w.GetClient() == nil {
		log.Println("DEBUG: WhatsApp client not available")
		response := map[string]interface{}{
			"error": "WhatsApp client not available",
			"status": map[string]interface{}{
				"whatsapp_ready":   false,
				"client_available": false,
				"timestamp":        time.Now().Format(time.RFC3339),
			},
		}
		wr.Header().Set("Content-Type", "application/json")
		wr.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(wr).Encode(response)
		return
	}

	// Check if WhatsApp is ready
	if !w.IsReady() {
		log.Println("DEBUG: WhatsApp not ready, cannot analyze")
		response := map[string]interface{}{
			"error": "WhatsApp not logged in. Please scan QR code first",
			"status": map[string]interface{}{
				"whatsapp_ready":   false,
				"client_available": true,
				"timestamp":        time.Now().Format(time.RFC3339),
			},
		}
		wr.Header().Set("Content-Type", "application/json")
		wr.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(wr).Encode(response)
		return
	}

	// Check if contacts are ready
	client := w.GetClient()
	if client != nil {
		contacts, err := client.Store.Contacts.GetAllContacts(context.Background())
		if err != nil || len(contacts) == 0 {
			log.Println("DEBUG: Contacts not ready, cannot analyze")
			response := map[string]interface{}{
				"error": "Contacts not loaded yet. Please wait a moment and try again",
				"status": map[string]interface{}{
					"whatsapp_ready":   true,
					"contacts_ready":   false,
					"client_available": true,
					"timestamp":        time.Now().Format(time.RFC3339),
				},
			}
			wr.Header().Set("Content-Type", "application/json")
			wr.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(wr).Encode(response)
			return
		}
	}

	res, err := w.Analyze()
	if err != nil {
		log.Printf("DEBUG: Analysis error: %v", err)
		response := map[string]interface{}{
			"error": err.Error(),
			"status": map[string]interface{}{
				"whatsapp_ready":   w.IsReady(),
				"client_available": w.GetClient() != nil,
				"timestamp":        time.Now().Format(time.RFC3339),
			},
		}
		wr.Header().Set("Content-Type", "application/json")
		wr.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(wr).Encode(response)
		return
	}

	log.Printf("DEBUG: Analysis completed successfully")
	log.Printf("DEBUG: Response data - Strength: %s", res.Strength)
	log.Printf("DEBUG: Response data - Total Chats: %d", res.TotalChats)
	log.Printf("DEBUG: Response data - Total Contacts: %d", res.TotalContacts)
	log.Printf("DEBUG: Response data - Account Age: %d days", res.AccountAgeDays)
	log.Printf("DEBUG: Response data - Total Groups: %d", res.TotalGroups)
	log.Printf("DEBUG: Response data - Chat with Contact: %d", res.TotalChatWithContact)
	log.Printf("DEBUG: Response data - Sensitive Content: %d", res.SensitiveContentCount)
	log.Printf("DEBUG: Response data - Total Unsaved Chats: %d", res.TotalUnsavedChats)
	log.Printf("DEBUG: Response data - Unknown Number Chats: %d", res.UnknownNumberChats)

	// Add status information to response
	response := map[string]interface{}{
		"analysis": res,
		"status": map[string]interface{}{
			"whatsapp_ready":   w.IsReady(),
			"client_available": w.GetClient() != nil,
			"timestamp":        time.Now().Format(time.RFC3339),
		},
	}

	wr.Header().Set("Content-Type", "application/json")
	json.NewEncoder(wr).Encode(response)
}

func (w *WhatsApp) HandleLogout(wr http.ResponseWriter, r *http.Request) {
	log.Println("DEBUG: Logout request received - clearing session and analysis data")

	// Clear analysis cache before reset
	w.ClearAnalysisCache()

	// Reset session untuk user baru
	w.Reset()

	log.Println("DEBUG: Logout completed successfully - session cleared and ready for new login")

	response := map[string]interface{}{
		"message": "Logged out successfully and analysis data cleared",
		"status": map[string]interface{}{
			"whatsapp_ready":   false,
			"client_available": w.GetClient() != nil,
			"timestamp":        time.Now().Format(time.RFC3339),
		},
	}

	wr.Header().Set("Content-Type", "application/json")
	json.NewEncoder(wr).Encode(response)
}

// HandleRefreshQR triggers QR regeneration without full logout
func (w *WhatsApp) HandleRefreshQR(wr http.ResponseWriter, r *http.Request) {
	log.Println("QR refresh request received")
	if err := w.RefreshQR(); err != nil {
		log.Printf("QR refresh error: %v", err)
		http.Error(wr, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(wr).Encode(map[string]string{"message": "QR refresh triggered"})
}

// ✅ NEW: Manual reconnect endpoint untuk frontend control
func (w *WhatsApp) HandleManualReconnect(wr http.ResponseWriter, r *http.Request) {
	log.Println("DEBUG: Manual reconnect request received from frontend")

	if r.Method != "POST" {
		http.Error(wr, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Attempt manual reconnect
	err := w.ManualReconnect()
	if err != nil {
		log.Printf("DEBUG: Manual reconnect failed: %v", err)
		response := map[string]interface{}{
			"error": fmt.Sprintf("Failed to reconnect: %v", err),
			"status": map[string]interface{}{
				"whatsapp_ready":   false,
				"client_available": false,
				"timestamp":        time.Now().Format(time.RFC3339),
			},
		}
		wr.Header().Set("Content-Type", "application/json")
		wr.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(wr).Encode(response)
		return
	}

	log.Println("DEBUG: Manual reconnect completed successfully")
	response := map[string]interface{}{
		"message": "Reconnected successfully",
		"status": map[string]interface{}{
			"whatsapp_ready":   w.IsReady(),
			"client_available": w.GetClient() != nil,
			"timestamp":        time.Now().Format(time.RFC3339),
		},
	}

	wr.Header().Set("Content-Type", "application/json")
	json.NewEncoder(wr).Encode(response)
}
