package models

import (
	"time"

	"gorm.io/gorm"
)

// ScanHistory represents a scan operation history for a user
type ScanHistory struct {
	ID          uint           `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID      uint           `json:"user_id" gorm:"not null"`
	PhoneNumber string         `json:"phone_number" gorm:"size:20;not null"`
	ScanDate    time.Time      `json:"scan_date" gorm:"autoCreateTime"`
	Status      string         `json:"status" gorm:"type:varchar(20);default:'pending';check:status IN ('success','failed','pending')"`
	ResultData  string         `json:"result_data" gorm:"type:text"` // JSON string of scan results
	ErrorMsg    string         `json:"error_msg" gorm:"size:500"`
	CreatedAt   time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`

	// Relationship
	User User `json:"user" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for ScanHistory
func (ScanHistory) TableName() string {
	return "scan_history"
}
