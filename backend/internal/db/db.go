package db

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

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
	BackfillPublicIDs(db)
	err = AutoMigrate(db)
	if err != nil {
		log.Fatalf("Failed to auto migrate database: %v", err)
	}

	fmt.Println("Database AutoMigrate completed")
}
