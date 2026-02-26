package main

import (
	"encoding/json"
	"net/http"
)

func HandleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rows, _ := DB.Query(`SELECT name FROM users`)
		defer rows.Close()
		var users []string
		for rows.Next() {
			var name string
			rows.Scan(&name)
			users = append(users, name)
		}
		json.NewEncoder(w).Encode(users)
	} else if r.Method == http.MethodPost {
		var u struct {
			Name string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&u)
		DB.Exec(`INSERT INTO users (name) VALUES ($1)`, u.Name)
		w.WriteHeader(http.StatusCreated)
	}
}
