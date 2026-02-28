package main

import (
	"database/sql"
	"log"
	"os"
	"time" // ⏳ We need this for the retry sleep!

	database "shared/database/generated" // Note: the package inside is named 'database'

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

// The global variables so handlers.go and main.go can see them!
var DB *sql.DB
var Queries *database.Queries

func InitDB() {
	user := os.Getenv("POSTGRES_USER")
	if user == "" {
		user = "postgres"
	}
	password := os.Getenv("POSTGRES_PASSWORD")
	if password == "" {
		password = "postgres"
	}
	dbName := os.Getenv("POSTGRES_DB")
	if dbName == "" {
		dbName = "nexus_chat"
	}

	connStr := "postgres://" + user + ":" + password + "@postgres-db:5432/" + dbName + "?sslmode=disable"

	var err error
	DB, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("❌ Failed to open database connection: %v", err)
	}

	// 🚨 The Postgres Wait Loop!
	log.Println("⏳ Attempting to connect to PostgreSQL...")
	for i := 0; i < 10; i++ {
		err = DB.Ping()
		if err == nil {
			break // Success! It's awake!
		}
		log.Printf("⏳ Database not ready yet... retrying in 2s")
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatalf("❌ Database ping failed after retries: %v", err)
	}
	log.Println("🐘 Connected to PostgreSQL!")

	// --- 1. Run Migrations Automatically ---
	driver, err := postgres.WithInstance(DB, &postgres.Config{})
	if err != nil {
		log.Fatalf("❌ Could not create migration driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://migrations", "postgres", driver)
	if err != nil {
		log.Fatalf("❌ Migration initialization failed: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("❌ Migration failed: %v", err)
	}
	log.Println("✅ Database schema is locked and loaded!")

	// --- 2. Initialize sqlc ---
	Queries = database.New(DB)
}
