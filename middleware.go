package main

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// 🚨 This MUST exactly match the secret key in auth-service!
var jwtSecret = []byte(os.Getenv("JWT_SECRET"))

// We use a custom type for context keys to prevent collisions
type contextKey string

const userIDKey contextKey = "user_id"
const usernameKey contextKey = "username"

// AuthMiddleware is the bouncer for our API Hub
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenString := ""

		// 1. Try to get the token from the Authorization header (for standard HTTP API calls)
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			// 2. Fallback: Try to get the token from the URL query (Crucial for WebSockets!)
			tokenString = r.URL.Query().Get("token")
		}

		if tokenString == "" {
			http.Error(w, "Unauthorized: Missing Token", http.StatusUnauthorized)
			return
		}

		// 3. Parse and cryptographically verify the token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Unauthorized: Invalid Token", http.StatusUnauthorized)
			return
		}

		// 4. Extract the user_id from the payload
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			userID := claims["user_id"].(string)
			username := claims["username"].(string)

			// 5. Inject the real user_id into the request context and pass it to the next handler
			ctx := context.WithValue(r.Context(), userIDKey, userID)
			ctx = context.WithValue(ctx, usernameKey, username)
			next.ServeHTTP(w, r.WithContext(ctx))
		} else {
			http.Error(w, "Unauthorized: Invalid Claims", http.StatusUnauthorized)
		}
	}
}
