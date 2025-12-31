package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	// Database connection pool
	dbPool = make(map[string]*sql.DB)
	dbMux  sync.RWMutex
)

// getDatabase gets or creates a database connection from the pool
func getDatabase(dbName string) (*sql.DB, string, error) {
	// Try to get existing connection from pool (read lock)
	dbMux.RLock()
	if db, exists := dbPool[dbName]; exists {
		dbMux.RUnlock()
		return db, dbName + ".db", nil
	}
	dbMux.RUnlock()

	// Connection doesn't exist, create it (write lock)
	dbMux.Lock()
	defer dbMux.Unlock()

	// Double-check in case another goroutine created it
	if db, exists := dbPool[dbName]; exists {
		return db, dbName + ".db", nil
	}

	// Create new connection
	dbFile := filepath.Join(dbPath, dbName+".db")

	// Check if file exists
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return nil, "", fmt.Errorf("database not found: %s", dbName)
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro", dbFile))
	if err != nil {
		return nil, "", err
	}

	// Configure connection pool settings
	// SQLite benefits from limited concurrency due to file locking
	db.SetMaxOpenConns(20)           // Max 20 concurrent connections per database
	db.SetMaxIdleConns(10)           // Keep 10 idle connections ready
	db.SetConnMaxLifetime(time.Hour) // Recycle connections every hour

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, "", err
	}

	// Store in pool
	dbPool[dbName] = db
	log.Printf("Created connection pool for database: %s", dbName)

	return db, filepath.Base(dbFile), nil
}

// validateDatabases checks all databases are accessible at startup
func validateDatabases() error {
	// Check if database path exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database path does not exist: %s", dbPath)
	}

	// Find all database files
	dbFiles, err := filepath.Glob(filepath.Join(dbPath, "*.db"))
	if err != nil {
		return fmt.Errorf("failed to list database files: %v", err)
	}

	if len(dbFiles) == 0 {
		return fmt.Errorf("no database files (*.db) found in %s", dbPath)
	}

	var hasErrors bool

	for _, dbFile := range dbFiles {
		// Resolve symlinks if needed
		realPath := dbFile
		if info, err := os.Lstat(dbFile); err == nil && info.Mode()&os.ModeSymlink != 0 {
			if resolved, err := filepath.EvalSymlinks(dbFile); err == nil {
				realPath = resolved
			}
		}

		// Try to open and ping the database
		db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro", realPath))
		if err != nil {
			log.Printf("[!!] %s", realPath)
			log.Printf("%s", err)
			hasErrors = true
			continue
		}

		err = db.Ping()
		db.Close()
		if err != nil {
			log.Printf("[!!] %s", realPath)
			log.Printf("%s", err)
			hasErrors = true
			continue
		}

		log.Printf("[ok] %s", realPath)
	}

	if hasErrors {
		return fmt.Errorf("database validation failed")
	}

	return nil
}
