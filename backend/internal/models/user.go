package models

import (
	"time"

	"gorm.io/gorm"
)

// User represents a user account
type User struct {
	ID              uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	Username        string     `json:"username" gorm:"uniqueIndex;size:50;not null"`
	Email           string     `json:"email" gorm:"uniqueIndex;size:100;not null"`
	PasswordHash    string     `json:"-" gorm:"size:255;not null"` // "-" means don't include in JSON
	PhoneNumber     string     `json:"phone_number" gorm:"size:20;not null"`
	Role            string     `json:"role" gorm:"type:varchar(20);default:'user';check:role IN ('admin','user')"`
	IsActive        bool       `json:"is_active" gorm:"default:true"`
	EmailVerified   bool       `json:"email_verified" gorm:"default:false"`
	EmailVerifiedAt *time.Time `json:"email_verified_at" gorm:"default:null"`

	// OTP fields
	OTPCode      string     `json:"-" gorm:"size:10;default:null"`
	OTPExpiresAt *time.Time `json:"-" gorm:"default:null"`

	// Password reset token fields
	ResetToken          string     `json:"-" gorm:"size:255;default:null"`
	ResetTokenExpiresAt *time.Time `json:"-" gorm:"default:null"`

	CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	// Relationships
	WhatsAppSessions []WhatsAppSession `json:"whatsapp_sessions" gorm:"foreignKey:UserID"`
	AnalysisResults  []AnalysisResult  `json:"analysis_results" gorm:"foreignKey:UserID"`
	ScanHistory      []ScanHistory     `json:"scan_history" gorm:"foreignKey:UserID"`
}

// UserLogin represents login request
type UserLogin struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// UserRegister represents registration request
type UserRegister struct {
	Username    string `json:"username" binding:"required,min=3,max=50"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6"`
	PhoneNumber string `json:"phone_number" binding:"required"`
}

// UserResponse represents user data in responses
type UserResponse struct {
	ID          uint      `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	PhoneNumber string    `json:"phone_number"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

// TableName specifies the table name for User
func (User) TableName() string {
	return "users"
}
