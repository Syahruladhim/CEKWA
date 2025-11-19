package database

import (
	"fmt"
	"log"
	"os"

	"back_wa/internal/models"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDatabase initializes the database connection
func InitDatabase() {
	var err error

	// Check environment for database type
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "sqlite" // default to sqlite for development
	}

	switch dbType {
	case "mysql":
		DB, err = connectMySQL()
	case "postgres", "postgresql":
		DB, err = connectPostgreSQL()
	case "sqlite":
		DB, err = connectSQLite()
	default:
		log.Fatal("Unsupported database type:", dbType)
	}

	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Auto migrate tables
	err = migrateTables(DB)
	if err != nil {
		log.Fatal("Failed to migrate tables:", err)
	}

	log.Println("Database connected and migrated successfully!")
}

// connectMySQL connects to MySQL database
func connectMySQL() (*gorm.DB, error) {
	// Get database configuration from environment variables
	host := getEnv("DB_HOST", "127.0.0.1")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", "root")
	password := getEnv("DB_PASSWORD", "")
	dbName := getEnv("DB_NAME", "wa_analyzer")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&timeout=10s&readTimeout=30s&writeTimeout=30s",
		user, password, host, port, dbName)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// Set connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(3600) // 1 hour

	return db, nil
}

// connectPostgreSQL connects to PostgreSQL database
func connectPostgreSQL() (*gorm.DB, error) {
	// Get database configuration from environment variables
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	password := getEnv("DB_PASSWORD", "")
	dbName := getEnv("DB_NAME", "wa_analisis")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Jakarta",
		host, port, user, password, dbName)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %v", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// Set connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(3600) // 1 hour

	return db, nil
}

// connectSQLite connects to SQLite database (fallback)
func connectSQLite() (*gorm.DB, error) {
	return gorm.Open(sqlite.Open("whatsapp.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
}

// migrateTables creates/updates database tables
func migrateTables(db *gorm.DB) error {
    if err := db.AutoMigrate(
        &models.User{},
        &models.WhatsAppSession{},
        &models.AnalysisResult{},
        &models.ScanHistory{},
        &models.Transaction{},
        &models.PaymentMethod{},
        &models.PaymentCategory{},
    ); err != nil {
        return err
    }

    // Ensure transactions.phone_number exists (backward compatibility)
    // Works for SQLite, MySQL, and PostgreSQL
    type columnInfo struct{
        Name string
    }
    var hasPhone bool
    dbType := getEnv("DB_TYPE", "sqlite")
    switch dbType {
    case "mysql":
        rows, err := db.Raw("SHOW COLUMNS FROM transactions LIKE 'phone_number'").Rows()
        if err == nil {
            defer rows.Close()
            if rows.Next() { hasPhone = true }
        }
    case "postgres", "postgresql":
        rows, err := db.Raw("SELECT column_name FROM information_schema.columns WHERE table_name = 'transactions' AND column_name = 'phone_number'").Rows()
        if err == nil {
            defer rows.Close()
            if rows.Next() { hasPhone = true }
        }
    default: // sqlite
        rows, err := db.Raw("PRAGMA table_info(transactions)").Rows()
        if err == nil {
            defer rows.Close()
            var (
                cid int
                name string
                ctype string
                notnull int
                dflt interface{}
                pk int
            )
            for rows.Next() {
                if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err == nil {
                    if name == "phone_number" { hasPhone = true; break }
                }
            }
        }
    }

    if !hasPhone {
        // Add nullable column to avoid failures on existing rows
        var alterSQL string
        switch dbType {
        case "postgres", "postgresql":
            alterSQL = "ALTER TABLE transactions ADD COLUMN phone_number VARCHAR(50)"
        case "mysql":
            alterSQL = "ALTER TABLE transactions ADD COLUMN phone_number VARCHAR(50)"
        default: // sqlite
            alterSQL = "ALTER TABLE transactions ADD COLUMN phone_number VARCHAR(50)"
        }
        
        if err := db.Exec(alterSQL).Error; err != nil {
            log.Println("warning: failed to add phone_number column:", err)
        } else {
            log.Println("added phone_number column to transactions table")
        }
    }

    return nil
}

// getEnv gets environment variable with fallback
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// GetDB returns the database instance
func GetDB() *gorm.DB {
	return DB
}

// CheckAndReconnect checks if database connection is alive and reconnects if needed
func CheckAndReconnect() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	// Ping the database to check connection
	if err := sqlDB.Ping(); err != nil {
		log.Printf("Database connection lost, attempting to reconnect...")

		// Close the old connection
		sqlDB.Close()

		// Reinitialize the database
		InitDatabase()

		return nil
	}

	return nil
}
