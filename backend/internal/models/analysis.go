package models

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

type AnalysisResult struct {
	ID                    uint           `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID                uint           `json:"user_id" gorm:"not null"`
	ScanHistoryID         *uint          `json:"scan_history_id" gorm:"index"`
	TotalChats            int            `json:"totalChats"`
	TotalContacts         int            `json:"totalContacts"`
	AccountAgeDays        int            `json:"accountAgeDays"`
	TotalGroups           int            `json:"totalGroups"`
	TotalChatWithContact  int            `json:"totalChatWithContact"`
	SensitiveContentCount int            `json:"sensitiveContentCount"`
	TotalUnsavedChats     int            `json:"totalUnsavedChats"`
	UnknownNumberChats    int            `json:"unknownNumberChats"`
	Strength              string         `json:"strength"`
	Summary               string         `json:"summary"`
	ScanDate              time.Time      `json:"scan_date" gorm:"autoCreateTime"`
	CreatedAt             time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt             time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt             gorm.DeletedAt `json:"-" gorm:"index"`

	// Relationship
	User        User        `json:"user" gorm:"foreignKey:UserID"`
	ScanHistory ScanHistory `json:"scan_history" gorm:"foreignKey:ScanHistoryID"`
}

// TableName specifies the table name for AnalysisResult
func (AnalysisResult) TableName() string {
	return "analysis_results"
}

// ParameterEvaluation represents the evaluation result for each parameter
type ParameterEvaluation struct {
	Parameter string
	Value     int
	Status    string // "Baik", "Cukup", "Buruk"
	Score     int    // 3 for Baik, 2 for Cukup, 1 for Buruk
}

func CalculateStrength(totalChats, totalContacts, accountAgeDays, totalGroups, totalChatWithContact, sensitiveContentCount, totalUnsavedChats, unknownNumberChats int) (string, string) {
	fmt.Printf("DEBUG: Calculating strength with parameters:\n")
	fmt.Printf("  Total Chats: %d\n", totalChats)
	fmt.Printf("  Total Contacts: %d\n", totalContacts)
	fmt.Printf("  Account Age: %d days\n", accountAgeDays)
	fmt.Printf("  Total Groups: %d\n", totalGroups)
	fmt.Printf("  Chat with Contact: %d\n", totalChatWithContact)
	fmt.Printf("  Sensitive Content: %d\n", sensitiveContentCount)
	fmt.Printf("  Total Unsaved Chats: %d\n", totalUnsavedChats)
	fmt.Printf("  Unknown Number Chats: %d\n", unknownNumberChats)

	// Check if using default values
	if totalChats == 150 && totalContacts == 250 && accountAgeDays == 400 {
		fmt.Printf("DEBUG: Using default values for analysis\n")
	}

	evaluations := []ParameterEvaluation{
		evaluateTotalChats(totalChats),
		evaluateTotalContacts(totalContacts),
		evaluateAccountAge(accountAgeDays),
		evaluateTotalGroups(totalGroups),
		evaluateChatWithContacts(totalChatWithContact),
		evaluateSensitiveContent(sensitiveContentCount),
		evaluateUnsavedChats(totalUnsavedChats),
		evaluateUnknownChats(unknownNumberChats),
	}

	// Calculate total score
	totalScore := 0
	fmt.Printf("\nDEBUG: Parameter evaluations:\n")
	for _, eval := range evaluations {
		totalScore += eval.Score
		fmt.Printf("  %s: %d (%s) - Score: %d\n", eval.Parameter, eval.Value, eval.Status, eval.Score)
	}

	// Calculate average score (max possible: 24, min possible: 8)
	averageScore := float64(totalScore) / 8.0
	fmt.Printf("\nDEBUG: Total Score: %d, Average Score: %.2f\n", totalScore, averageScore)

	// Determine overall strength
	var strength string
	if averageScore >= 2.5 {
		strength = "Baik"
	} else if averageScore >= 1.5 {
		strength = "Cukup"
	} else {
		strength = "Buruk"
	}

	fmt.Printf("DEBUG: Final Strength: %s\n", strength)

	// Generate summary
	summary := generateSummary(evaluations, strength, averageScore)

	return strength, summary
}

func evaluateTotalChats(value int) ParameterEvaluation {
	var status string
	var score int
	if value >= 100 {
		status = "Baik"
		score = 3
	} else if value >= 40 {
		status = "Cukup"
		score = 2
	} else {
		status = "Buruk"
		score = 1
	}
	return ParameterEvaluation{"Total Chats", value, status, score}
}

func evaluateTotalContacts(value int) ParameterEvaluation {
	var status string
	var score int
	if value >= 200 {
		status = "Baik"
		score = 3
	} else if value >= 100 {
		status = "Cukup"
		score = 2
	} else {
		status = "Buruk"
		score = 1
	}
	return ParameterEvaluation{"Total Kontak", value, status, score}
}

func evaluateAccountAge(value int) ParameterEvaluation {
	var status string
	var score int
	if value >= 365 {
		status = "Baik"
		score = 3
	} else if value >= 90 {
		status = "Cukup"
		score = 2
	} else {
		status = "Buruk"
		score = 1
	}
	return ParameterEvaluation{"Umur Akun", value, status, score}
}

func evaluateTotalGroups(value int) ParameterEvaluation {
	var status string
	var score int
	if value >= 80 {
		status = "Baik"
		score = 3
	} else if value >= 30 {
		status = "Cukup"
		score = 2
	} else {
		status = "Buruk"
		score = 1
	}
	return ParameterEvaluation{"Total Grup", value, status, score}
}

func evaluateChatWithContacts(value int) ParameterEvaluation {
	var status string
	var score int
	if value >= 100 {
		status = "Baik"
		score = 3
	} else if value >= 30 {
		status = "Cukup"
		score = 2
	} else {
		status = "Buruk"
		score = 1
	}
	return ParameterEvaluation{"Chat dengan Kontak", value, status, score}
}

func evaluateSensitiveContent(value int) ParameterEvaluation {
	var status string
	var score int
	if value <= 5 {
		status = "Baik"
		score = 3
	} else if value <= 10 {
		status = "Cukup"
		score = 2
	} else {
		status = "Buruk"
		score = 1
	}
	return ParameterEvaluation{"Sensitivitas Chat", value, status, score}
}

func evaluateUnsavedChats(value int) ParameterEvaluation {
	var status string
	var score int
	if value <= 100 {
		status = "Baik"
		score = 3
	} else if value <= 500 {
		status = "Cukup"
		score = 2
	} else {
		status = "Buruk"
		score = 1
	}
	return ParameterEvaluation{"Uninterested Chat", value, status, score}
}

func evaluateUnknownChats(value int) ParameterEvaluation {
	var status string
	var score int
	if value <= 15 {
		status = "Baik"
		score = 3
	} else if value <= 30 {
		status = "Cukup"
		score = 2
	} else {
		status = "Buruk"
		score = 1
	}
	return ParameterEvaluation{"Chat tidak dikenal", value, status, score}
}

func generateSummary(evaluations []ParameterEvaluation, strength string, averageScore float64) string {
	baikCount := 0
	cukupCount := 0
	burukCount := 0

	for _, eval := range evaluations {
		switch eval.Status {
		case "Baik":
			baikCount++
		case "Cukup":
			cukupCount++
		case "Buruk":
			burukCount++
		}
	}

	summary := "Ringkasan Evaluasi Akun WhatsApp:\n\n"
	summary += "Kekuatan Akun: " + strength + "\n"
	summary += "Skor Rata-rata: " + fmt.Sprintf("%.1f", averageScore) + "/3.0\n\n"

	summary += "Distribusi Parameter:\n"
	summary += "- Baik: " + fmt.Sprintf("%d", baikCount) + " parameter\n"
	summary += "- Cukup: " + fmt.Sprintf("%d", cukupCount) + " parameter\n"
	summary += "- Buruk: " + fmt.Sprintf("%d", burukCount) + " parameter\n\n"

	summary += "Detail Parameter:\n"
	for _, eval := range evaluations {
		summary += fmt.Sprintf("• %s: %d (%s)\n", eval.Parameter, eval.Value, eval.Status)
	}

	return summary
}
