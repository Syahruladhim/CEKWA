package models

import (
	"time"

	"gorm.io/gorm"
)

// WhatsAppSession represents a WhatsApp session for a specific user
type WhatsAppSession struct {
	ID           uint           `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID       uint           `json:"user_id" gorm:"not null"`
	SessionData  string         `json:"session_data" gorm:"type:text"` // encrypted session data
	QRCode       string         `json:"qr_code" gorm:"type:text"`
	Status       string         `json:"status" gorm:"type:varchar(20);default:'disconnected';check:status IN ('connected','disconnected','scanning')"`
	DeviceID     string         `json:"device_id" gorm:"size:100"`
	LastActivity time.Time      `json:"last_activity" gorm:"autoUpdateTime"`
	CreatedAt    time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`

	// Relationship
	User User `json:"user" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for WhatsAppSession
func (WhatsAppSession) TableName() string {
	return "whatsapp_sessions"
}
