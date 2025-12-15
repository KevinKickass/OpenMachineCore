package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type contextKey string

const (
	userIDKey      contextKey = "user_id"
	permissionsKey contextKey = "permissions"
)

// AuthMiddleware validates tokens and enforces authentication
func (a *AuthService) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "missing authorization header",
			})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid authorization header format",
			})
			c.Abort()
			return
		}

		token := parts[1]
		ipAddress := c.ClientIP()
		userAgent := c.GetHeader("User-Agent")

		// Try JWT first to get user info
		if claims, err := a.jwtHandler.ValidateAccessToken(token); err == nil {
			c.Set("permissions", a.roleToPermissions(claims.Role))
			c.Set("user_id", claims.UserID)
			c.Set("username", claims.Username)
			c.Set("role", claims.Role)
			c.Next()
			return
		}

		// Fall back to machine token (no user_id for machine tokens)
		permissions, err := a.ValidateMachineToken(c.Request.Context(), token, ipAddress, userAgent)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid or expired token",
			})
			c.Abort()
			return
		}

		// Store permissions in context (machine tokens don't have user_id)
		c.Set("permissions", permissions)
		c.Next()
	}
}

// RequirePermission checks if user has required permission
func RequirePermission(required Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		perms, exists := c.Get("permissions")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "no permissions found",
			})
			c.Abort()
			return
		}

		permissions := perms.([]Permission)
		hasPermission := false
		for _, p := range permissions {
			if p == required {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			c.JSON(http.StatusForbidden, gin.H{
				"error":    "insufficient permissions",
				"required": string(required),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// GetUserPermissions extracts permissions from context
func GetUserPermissions(ctx context.Context) []Permission {
	if perms, ok := ctx.Value(permissionsKey).([]Permission); ok {
		return perms
	}
	return nil
}
