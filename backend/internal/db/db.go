package db

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

const migrationLockKey = 740_321_001

func Connect() {
	// For MVP, we will try to read from standard env vars or fallback to a default
	// The frontend uses Postgres via Drizzle, so we connect to the same DB.
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		// Fallback for local development if not set
		dsn = "postgres://postgres:postgres@localhost:5432/visora?sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	fmt.Println("Connected to Database successfully")

	DB = db
	// The API and worker(s) boot together; an advisory lock on one pinned
	// connection keeps their migrations from racing on a fresh database.
	err = db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("SELECT pg_advisory_lock(?)", migrationLockKey).Error; err != nil {
			return err
		}
		defer conn.Exec("SELECT pg_advisory_unlock(?)", migrationLockKey)
		BackfillPublicIDs(conn)
		return AutoMigrate(conn)
	})
	if err != nil {
		log.Fatalf("Failed to auto migrate database: %v", err)
	}

	fmt.Println("Database AutoMigrate completed")
}
