package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load() // Load .env file if it

	connStr := os.Getenv("DATABASE_URL")
	log.Printf("Using database connection string: %s", connStr)

	if connStr == "" {
		connStr = "postgres://admin:secretpassword@localhost:5432/app_db?sslmode=disable"
	}

	InitDB(connStr)

	http.HandleFunc("/users", HandleUsers)

	fmt.Println("Server is running on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
