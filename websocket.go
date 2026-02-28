package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	database "shared/database/generated"
	"sync"

	"github.com/gorilla/websocket"
)

// We need an Upgrader to turn a normal HTTP request into a permanent WebSocket
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all frontend connections for now
	},
}

// Client represents a single connected user
type Client struct {
	conn *websocket.Conn
}

// Hub maintains the list of active clients and broadcasts messages
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.Mutex // Protects the clients map from concurrent writes
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run is the infinite loop that manages all WebSocket traffic
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("🟢 New user joined chat. Total online: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.conn.Close()
				log.Printf("🔴 User left chat. Total online: %d", len(h.clients))
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.Lock()
			// Blast the message to every connected client!
			for client := range h.clients {
				err := client.conn.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					client.conn.Close()
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()
		}
	}
}

// serveWs handles incoming WebSocket requests from the frontend
func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDKey).(string)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ WebSocket Upgrade Failed: %v", err)
		return
	}

	client := &Client{conn: conn}
	hub.register <- client

	// Keep the connection alive and listen for incoming text messages
	go func() {
		defer func() { hub.unregister <- client }()
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}

			// 🚨 INTERCEPT THE MESSAGE: Parse the incoming JSON
			var incoming struct {
				Type    string `json:"type"`
				Content string `json:"content"`
			}

			if err := json.Unmarshal(message, &incoming); err == nil && incoming.Type == "NEW_TEXT" {
				// 1. Save the text message to Postgres!
				savedMsg, err := Queries.CreateMessage(context.Background(), database.CreateMessageParams{
					UserID:  userID,
					Content: sql.NullString{String: incoming.Content, Valid: true},
					// FileUrl is left empty/null
				})

				if err == nil {
					username := r.Context().Value(usernameKey).(string)

					// 2. Broadcast the officially saved message to everyone
					broadcastMsg, _ := json.Marshal(map[string]interface{}{
						"type":       "NEW_TEXT",
						"message_id": savedMsg.ID,
						"user_id":    savedMsg.UserID,
						"username":   username,
						"content":    savedMsg.Content.String,
					})
					hub.broadcast <- broadcastMsg
					log.Printf("💾 Saved and broadcasted text message #%d", savedMsg.ID)
				} else {
					log.Printf("❌ Failed to save text message: %v", err)
				}
			}
		}
	}()
}
