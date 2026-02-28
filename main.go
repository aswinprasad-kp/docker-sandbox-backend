package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	database "shared/database/generated"
	"shared/pb" // Our shared dictionary!

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func getMessagesHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Fetch the last 100 messages from Postgres
	msgs, err := Queries.GetMessages(context.Background())
	if err != nil {
		http.Error(w, "Failed to fetch messages", http.StatusInternalServerError)
		return
	}

	// 2. Format them for the frontend
	var response []map[string]interface{}
	for _, m := range msgs {
		msgType := "NEW_TEXT"
		if m.FileUrl.Valid {
			msgType = "NEW_IMAGE"
		}
		response = append(response, map[string]interface{}{
			"type":       msgType,
			"message_id": m.ID,
			"user_id":    m.UserID,
			"username":   m.Username,
			"content":    m.Content.String,
			"url":        m.FileUrl.String,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// uploadHandler handles the HTTP POST request from the React frontend
func uploadHandler(grpcClient pb.MediaServiceClient, hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Parse the incoming HTTP form file (Max 10MB)
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "Unable to parse form", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Missing 'file' field", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// 2. Open the gRPC Streaming Tunnel to the Media Engine
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		stream, err := grpcClient.UploadFile(ctx)
		if err != nil {
			http.Error(w, "Failed to connect to media engine", http.StatusInternalServerError)
			return
		}

		userID := r.Context().Value(userIDKey).(string)

		// 3. Send the Metadata as the very first message
		err = stream.Send(&pb.FileUploadRequest{
			Request: &pb.FileUploadRequest_Metadata{
				Metadata: &pb.FileMetadata{
					FileName:    header.Filename,
					ContentType: header.Header.Get("Content-Type"),
					UploaderId:  userID, // Hardcoded for this test
				},
			},
		})
		if err != nil {
			http.Error(w, "Failed to send metadata", http.StatusInternalServerError)
			return
		}

		// 4. THE STREAMING LOOP: Slice the file into 4KB chunks and send them
		buffer := make([]byte, 4096)
		for {
			n, err := file.Read(buffer)

			// 🚨 THE FIX: ALWAYS send the bytes if n > 0, even if EOF is reached!
			if n > 0 {
				sendErr := stream.Send(&pb.FileUploadRequest{
					Request: &pb.FileUploadRequest_ChunkData{
						ChunkData: buffer[:n],
					},
				})
				if sendErr != nil {
					http.Error(w, "Failed to send chunk", http.StatusInternalServerError)
					return
				}
			}

			if err == io.EOF {
				break // We reached the end of the file!
			}
			if err != nil {
				http.Error(w, "Error reading file", http.StatusInternalServerError)
				return
			}
		}

		// 5. Close the tunnel and wait for the Media Engine's final response
		res, err := stream.CloseAndRecv()
		if err != nil {
			log.Printf("❌ gRPC Error from Media Engine: %v", err) // 🚨 THE FIX: Log the crash!
			http.Error(w, "Error receiving response from media engine", http.StatusInternalServerError)
			return
		}

		// 🚨 NEW: 6. Save the message to PostgreSQL using sqlc!
		msg, err := Queries.CreateMessage(ctx, database.CreateMessageParams{
			UserID: userID,
			Content: sql.NullString{
				String: "", // It's just an image upload, no text yet
				Valid:  false,
			},
			FileUrl: sql.NullString{
				String: res.FileUrl,
				Valid:  true,
			},
		})
		if err != nil {
			log.Printf("❌ Database Insert Failed: %v", err)
			http.Error(w, "Failed to save message to database", http.StatusInternalServerError)
			return
		}

		log.Printf("💾 Saved Message #%d to Database via sqlc!", msg.ID)

		username := r.Context().Value(usernameKey).(string)

		// 🚨 NEW: Broadcast to all active WebSockets!
		// We package the message data into JSON and throw it into the Hub's broadcast channel.
		broadcastMsg, _ := json.Marshal(map[string]interface{}{
			"type":       "NEW_IMAGE",
			"message_id": msg.ID,
			"user_id":    msg.UserID,
			"username":   username,
			"url":        res.FileUrl,
		})
		hub.broadcast <- broadcastMsg
		log.Printf("📡 Broadcasted new image to chat room!")

		// 7. Return the final JSON to the Frontend
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":    "File uploaded and saved to database!",
			"message_id": msg.ID,
			"url":        res.FileUrl,
		})
	}
}

func main() {
	// 🚨 THE FIX: You MUST boot the database and initialize 'Queries' first!
	InitDB()

	// 🚨 Boot up the WebSocket Hub in a background goroutine!
	chatHub := NewHub()
	go chatHub.Run()

	// 1. Dial the Media Engine
	conn, err := grpc.NewClient("media-service:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	grpcClient := pb.NewMediaServiceClient(conn)

	http.HandleFunc("/api/messages", AuthMiddleware(getMessagesHandler))

	// 2. Set up the HTTP router
	// Inject the Hub into the upload handler
	http.HandleFunc("/api/upload", AuthMiddleware(uploadHandler(grpcClient, chatHub)))

	// 3. 🚨 Create the WebSocket endpoint
	http.HandleFunc("/ws", AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		serveWs(chatHub, w, r)
	}))

	log.Println("🌐 API Hub listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
