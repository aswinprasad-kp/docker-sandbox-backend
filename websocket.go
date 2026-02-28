package main

import (
	"log"
	"net/http"
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
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ WebSocket Upgrade Failed: %v", err)
		return
	}

	client := &Client{conn: conn}
	hub.register <- client

	// Keep the connection alive and listen for incoming text messages (if any)
	go func() {
		defer func() { hub.unregister <- client }()
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				break // If they close the browser, this loop breaks and they unregister
			}
			// If they send a text message, broadcast it to everyone
			hub.broadcast <- message
		}
	}()
}
