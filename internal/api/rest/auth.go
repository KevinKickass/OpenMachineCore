package rest

import (
	"net/http"

	"github.com/KevinKickass/OpenMachineCore/internal/auth"
	"github.com/KevinKickass/OpenMachineCore/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Login request/response types
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"` // seconds
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Machine Token Management
type CreateMachineTokenRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Permissions []string               `json:"permissions"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type CreateMachineTokenResponse struct {
	Token       string                 `json:"token"` // Only returned once!
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	Permissions []string               `json:"permissions"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// User Management
type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role" binding:"required,oneof=technician admin"`
}

type UpdateUserRequest struct {
	Password *string `json:"password,omitempty" binding:"omitempty,min=8"`
	Role     *string `json:"role,omitempty" binding:"omitempty,oneof=technician admin"`
}

// Auth handlers
func (s *Server) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.NewErrorResponse("AUTH_400", "Invalid request body", err.Error()))
		return
	}

	authService := c.MustGet("authService").(*auth.AuthService)
	accessToken, refreshToken, err := authService.LoginUser(
		c.Request.Context(),
		req.Username,
		req.Password,
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)

	if err != nil {
		c.JSON(http.StatusUnauthorized, types.NewErrorResponse("AUTH_401", "Invalid credentials", nil))
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600, // 60 minutes
	})
}

func (s *Server) refreshToken(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.NewErrorResponse("AUTH_400", "Invalid request body", err.Error()))
		return
	}

	authService := c.MustGet("authService").(*auth.AuthService)
	accessToken, newRefreshToken, err := authService.RefreshAccessToken(
		c.Request.Context(),
		req.RefreshToken,
	)

	if err != nil {
		c.JSON(http.StatusUnauthorized, types.NewErrorResponse("AUTH_401", "Invalid or expired refresh token", nil))
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	})
}

func (s *Server) logout(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.NewErrorResponse("AUTH_400", "Invalid request body", err.Error()))
		return
	}

	authService := c.MustGet("authService").(*auth.AuthService)
	if err := authService.RevokeRefreshToken(c.Request.Context(), req.RefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, types.NewErrorResponse("AUTH_500", "Failed to logout", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
}

func (s *Server) getCurrentUser(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, types.NewErrorResponse("AUTH_401", "Not authenticated", nil))
		return
	}

	authService := c.MustGet("authService").(*auth.AuthService)
	user, err := authService.GetUserByID(c.Request.Context(), userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusNotFound, types.NewErrorResponse("USER_404", "User not found", nil))
		return
	}

	permissions, _ := c.Get("permissions")
	c.JSON(http.StatusOK, gin.H{
		"user":        user,
		"permissions": permissions,
	})
}

// Machine Token Management (Admin only)
func (s *Server) createMachineToken(c *gin.Context) {
	var req CreateMachineTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.NewErrorResponse("TOKEN_400", "Invalid request body", err.Error()))
		return
	}

	// Default to operator permission if none specified
	if len(req.Permissions) == 0 {
		req.Permissions = []string{"operator"}
	}

	userID, _ := c.Get("user_id")
	authService := c.MustGet("authService").(*auth.AuthService)

	token, machineToken, err := authService.CreateMachineToken(
		c.Request.Context(),
		req.Name,
		req.Permissions,
		userID.(*uuid.UUID),
		req.Metadata,
	)

	if err != nil {
		s.logger.Error("Failed to create machine token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, types.NewErrorResponse("TOKEN_500", "Failed to create token", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, CreateMachineTokenResponse{
		Token:       token, // Only time this is returned!
		ID:          machineToken.ID,
		Name:        machineToken.Name,
		Permissions: machineToken.Permissions,
		Metadata:    machineToken.Metadata,
	})
}

func (s *Server) listMachineTokens(c *gin.Context) {
	authService := c.MustGet("authService").(*auth.AuthService)
	tokens, err := authService.ListMachineTokens(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.NewErrorResponse("TOKEN_500", "Failed to list tokens", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"tokens": tokens})
}

func (s *Server) deleteMachineToken(c *gin.Context) {
	tokenID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, types.NewErrorResponse("TOKEN_400", "Invalid token ID", err.Error()))
		return
	}

	authService := c.MustGet("authService").(*auth.AuthService)
	if err := authService.DeleteMachineToken(c.Request.Context(), tokenID); err != nil {
		c.JSON(http.StatusInternalServerError, types.NewErrorResponse("TOKEN_500", "Failed to delete token", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "token deleted"})
}

func (s *Server) updateMachineToken(c *gin.Context) {
	tokenID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, types.NewErrorResponse("TOKEN_400", "Invalid token ID", err.Error()))
		return
	}

	var req struct {
		Name     *string                `json:"name"`
		Metadata map[string]interface{} `json:"metadata"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.NewErrorResponse("TOKEN_400", "Invalid request body", err.Error()))
		return
	}

	authService := c.MustGet("authService").(*auth.AuthService)
	if err := authService.UpdateMachineToken(c.Request.Context(), tokenID, req.Name, req.Metadata); err != nil {
		c.JSON(http.StatusInternalServerError, types.NewErrorResponse("TOKEN_500", "Failed to update token", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "token updated"})
}

// User Management (Admin only)
func (s *Server) createUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.NewErrorResponse("USER_400", "Invalid request body", err.Error()))
		return
	}

	authService := c.MustGet("authService").(*auth.AuthService)
	user, err := authService.CreateUser(c.Request.Context(), req.Username, req.Password, req.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.NewErrorResponse("USER_500", "Failed to create user", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, user)
}

func (s *Server) listUsers(c *gin.Context) {
	authService := c.MustGet("authService").(*auth.AuthService)
	users, err := authService.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.NewErrorResponse("USER_500", "Failed to list users", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

func (s *Server) updateUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, types.NewErrorResponse("USER_400", "Invalid user ID", err.Error()))
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.NewErrorResponse("USER_400", "Invalid request body", err.Error()))
		return
	}

	authService := c.MustGet("authService").(*auth.AuthService)
	if err := authService.UpdateUser(c.Request.Context(), userID, req.Password, req.Role); err != nil {
		c.JSON(http.StatusInternalServerError, types.NewErrorResponse("USER_500", "Failed to update user", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user updated"})
}

func (s *Server) deleteUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, types.NewErrorResponse("USER_400", "Invalid user ID", err.Error()))
		return
	}

	authService := c.MustGet("authService").(*auth.AuthService)
	if err := authService.DeleteUser(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, types.NewErrorResponse("USER_500", "Failed to delete user", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}
