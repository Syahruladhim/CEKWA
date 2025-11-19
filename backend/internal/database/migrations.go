package database

import (
	"database/sql"
	"fmt"
)

// CreateTransactionsTable creates the transactions table
func CreateTransactionsTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		external_id VARCHAR(255) UNIQUE NOT NULL,
		invoice_id VARCHAR(255) NOT NULL,
		amount DECIMAL(10,2) NOT NULL,
		currency VARCHAR(3) DEFAULT 'IDR',
		status VARCHAR(20) DEFAULT 'pending',
		payment_method VARCHAR(50) NOT NULL,
		payment_channel VARCHAR(50),
		description TEXT,
        phone_number VARCHAR(50),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		paid_at DATETIME,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	`

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create transactions table: %v", err)
	}

	// Create index on external_id for faster lookups
	indexQuery := `CREATE INDEX IF NOT EXISTS idx_transactions_external_id ON transactions(external_id);`
	_, err = db.Exec(indexQuery)
	if err != nil {
		return fmt.Errorf("failed to create index on external_id: %v", err)
	}

	// Create index on user_id for faster user transaction lookups
	userIndexQuery := `CREATE INDEX IF NOT EXISTS idx_transactions_user_id ON transactions(user_id);`
	_, err = db.Exec(userIndexQuery)
	if err != nil {
		return fmt.Errorf("failed to create index on user_id: %v", err)
	}

	// Create index on status for faster status filtering
	statusIndexQuery := `CREATE INDEX IF NOT EXISTS idx_transactions_status ON transactions(status);`
	_, err = db.Exec(statusIndexQuery)
	if err != nil {
		return fmt.Errorf("failed to create index on status: %v", err)
	}

    // Ensure phone_number column exists for backward compatibility (older deployments)
    // Check pragma table_info for transactions
    var colName string
    checkQuery := `PRAGMA table_info(transactions);`
    rows, err := db.Query(checkQuery)
    if err == nil {
        defer rows.Close()
        found := false
        var (
            cid int
            name string
            ctype string
            notnull int
            dfltValue interface{}
            pk int
        )
        for rows.Next() {
            if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
                if name == "phone_number" {
                    found = true
                    break
                }
            }
        }
        if !found {
            // Add column if missing
            alterQuery := `ALTER TABLE transactions ADD COLUMN phone_number VARCHAR(50);`
            if _, err := db.Exec(alterQuery); err != nil {
                // Log but don't fail startup
                fmt.Printf("warning: failed to add phone_number column: %v\n", err)
            } else {
                fmt.Println("Added phone_number column to transactions table")
            }
        }
    } else {
        // Could not run PRAGMA, continue without fatal error
        _ = colName
    }

	fmt.Println("Transactions table created successfully")
	return nil
}

// CreatePaymentMethodsTable creates the payment_methods table
func CreatePaymentMethodsTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS payment_methods (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(50) NOT NULL,
		type VARCHAR(20) NOT NULL,
		is_active BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create payment_methods table: %v", err)
	}

	// Insert default payment methods
	insertQuery := `
	INSERT OR IGNORE INTO payment_methods (name, type, is_active) VALUES
	('BCA', 'bank_transfer', 1),
	('BNI', 'bank_transfer', 1),
	('BRI', 'bank_transfer', 1),
	('Mandiri', 'bank_transfer', 1),
	('DANA', 'ewallet', 1),
	('OVO', 'ewallet', 1),
	('LinkAja', 'ewallet', 1),
	('ShopeePay', 'ewallet', 1),
	('GoPay', 'ewallet', 1),
	('Visa', 'credit_card', 1),
	('Mastercard', 'credit_card', 1),
	('JCB', 'credit_card', 1),
	('QRIS', 'qris', 1);
	`

	_, err = db.Exec(insertQuery)
	if err != nil {
		return fmt.Errorf("failed to insert default payment methods: %v", err)
	}

	fmt.Println("Payment methods table created successfully")
	return nil
}

// CreatePaymentCategoriesTable creates the payment_categories table
func CreatePaymentCategoriesTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS payment_categories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(100) NOT NULL,
		price DECIMAL(10,2) NOT NULL,
		is_active BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create payment_categories table: %v", err)
	}

	// Insert default payment categories
	insertQuery := `
	INSERT OR IGNORE INTO payment_categories (name, price, is_active) VALUES
	('Analisis WhatsApp', 50000.00, 1),
	('Analisis WhatsApp Premium', 100000.00, 1),
	('Analisis WhatsApp Enterprise', 200000.00, 1);
	`

	_, err = db.Exec(insertQuery)
	if err != nil {
		return fmt.Errorf("failed to insert default payment categories: %v", err)
	}

	fmt.Println("Payment categories table created successfully")
	return nil
}
