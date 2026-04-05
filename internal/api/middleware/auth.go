package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const ContextUserIDKey = "user_id"

func JWTAuth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing Authorization header"})
			return
		}

		tokenString := strings.TrimSpace(authHeader)
		if trimmed, found := strings.CutPrefix(tokenString, "Bearer "); found {
			tokenString = strings.TrimSpace(trimmed)
		}
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid bearer token"})
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}

		userID := extractUserID(claims)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user id not found in token"})
			return
		}

		c.Set(ContextUserIDKey, userID)
		c.Next()
	}
}

func extractUserID(claims jwt.MapClaims) string {
	if sub, ok := claims["sub"].(string); ok && strings.TrimSpace(sub) != "" {
		return strings.TrimSpace(sub)
	}

	if uid, ok := claims["user_id"].(string); ok && strings.TrimSpace(uid) != "" {
		return strings.TrimSpace(uid)
	}

	return ""
}
