package main

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func InitDB(connStr string) {
	var err error
	for i := 0; i < 5; i++ {
		DB, err = sql.Open("postgres", connStr)
		if err != nil && DB.Ping() == nil {
			log.Println("Connected to the database successfully!")
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to connect to the database: %v", err)
	}

	_, err = DB.Exec(`CREATE TABLE IF NOT EXISTS users (id SERIAL PRIMARY KEY, name TEXT)`)
	if err != nil {
		log.Fatalf("Failed to create users table: %v", err)
	}
}
