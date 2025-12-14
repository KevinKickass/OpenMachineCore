package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/KevinKickass/OpenMachineCore/internal/config"
	"github.com/KevinKickass/OpenMachineCore/internal/storage"
	"github.com/google/uuid"
)

type Permission string

const (
	PermOperator   Permission = "operator"
	PermTechnician Permission = "technician"
	PermAdmin      Permission = "admin"
)

type AuthService struct {
	storage         *storage.PostgresClient
	jwtHandler      *JWTHandler
	passwordHasher  *PasswordHasher
	machineTokenGen *MachineTokenGenerator
}

func NewAuthService(store *storage.PostgresClient, cfg config.AuthConfig) *AuthService {
	jwtSecret := cfg.GetJWTSecret()

	return &AuthService{
		storage:         store,
		jwtHandler:      NewJWTHandler(jwtSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL),
		passwordHasher:  NewPasswordHasher(),
		machineTokenGen: NewMachineTokenGenerator(),
	}
}

// LoginUser authenticates a user and returns tokens
func (a *AuthService) LoginUser(ctx context.Context, username, password, ipAddress, userAgent string) (accessToken, refreshToken string, err error) {
	user, err := a.storage.GetUserByUsername(ctx, username)
	if err != nil {
		a.logAuthEvent(ctx, "user_login_failed", nil, nil, ipAddress, userAgent, false, "user not found")
		return "", "", fmt.Errorf("invalid credentials")
	}

	// Check if account is locked
	if user.LockedUntil != nil && time.Now().Before(*user.LockedUntil) {
		return "", "", fmt.Errorf("account locked until %v", user.LockedUntil)
	}

	// Verify password
	valid, err := a.passwordHasher.VerifyPassword(password, user.PasswordHash)
	if err != nil || !valid {
		a.storage.IncrementFailedLoginAttempts(ctx, user.ID)
		a.logAuthEvent(ctx, "user_login_failed", &user.ID, nil, ipAddress, userAgent, false, "invalid password")
		return "", "", fmt.Errorf("invalid credentials")
	}

	// Reset failed attempts
	a.storage.ResetFailedLoginAttempts(ctx, user.ID)

	// Generate tokens
	accessToken, err = a.jwtHandler.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err = a.jwtHandler.GenerateRefreshToken()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store refresh token
	tokenHash := a.hashRefreshToken(refreshToken)
	expiresAt := time.Now().Add(a.jwtHandler.refreshTokenTTL)
	if err := a.storage.StoreRefreshToken(ctx, user.ID, tokenHash, expiresAt); err != nil {
		return "", "", fmt.Errorf("failed to store refresh token: %w", err)
	}

	// Update last login
	a.storage.UpdateLastLogin(ctx, user.ID)
	a.logAuthEvent(ctx, "user_login_success", &user.ID, nil, ipAddress, userAgent, true, "")

	return accessToken, refreshToken, nil
}

// ValidateMachineToken validates a machine token and returns permissions
func (a *AuthService) ValidateMachineToken(ctx context.Context, token, ipAddress, userAgent string) ([]Permission, error) {
	if !a.machineTokenGen.ValidateTokenFormat(token) {
		return nil, fmt.Errorf("invalid token format")
	}

	tokenHash := a.machineTokenGen.HashToken(token)
	machineToken, err := a.storage.GetMachineTokenByHash(ctx, tokenHash)
	if err != nil {
		a.logAuthEvent(ctx, "machine_token_failed", nil, nil, ipAddress, userAgent, false, "token not found")
		return nil, fmt.Errorf("invalid token")
	}

	// Update last used
	a.storage.UpdateMachineTokenLastUsed(ctx, machineToken.ID)
	a.logAuthEvent(ctx, "machine_token_success", nil, &machineToken.ID, ipAddress, userAgent, true, "")

	permissions := make([]Permission, len(machineToken.Permissions))
	for i, p := range machineToken.Permissions {
		permissions[i] = Permission(p)
	}

	return permissions, nil
}

// ValidateToken validates any token (JWT or Machine Token)
func (a *AuthService) ValidateToken(ctx context.Context, token, ipAddress, userAgent string) ([]Permission, error) {
	// Try JWT first
	if claims, err := a.jwtHandler.ValidateAccessToken(token); err == nil {
		return a.roleToPermissions(claims.Role), nil
	}

	// Try Machine Token
	return a.ValidateMachineToken(ctx, token, ipAddress, userAgent)
}

func (a *AuthService) roleToPermissions(role string) []Permission {
	switch role {
	case "admin":
		return []Permission{PermOperator, PermTechnician, PermAdmin}
	case "technician":
		return []Permission{PermOperator, PermTechnician}
	default:
		return []Permission{PermOperator}
	}
}

func (a *AuthService) hashRefreshToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (a *AuthService) logAuthEvent(ctx context.Context, eventType string, userID, machineTokenID *uuid.UUID, ip, userAgent string, success bool, reason string) {
	_ = a.storage.LogAuthEvent(ctx, eventType, userID, machineTokenID, ip, userAgent, success, reason)
}

// RefreshAccessToken generates new access token from refresh token
func (a *AuthService) RefreshAccessToken(ctx context.Context, refreshToken string) (string, string, error) {
	tokenHash := a.hashRefreshToken(refreshToken)

	userID, err := a.storage.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		return "", "", fmt.Errorf("invalid refresh token: %w", err)
	}

	// Get user details
	user, err := a.storage.GetUserByID(ctx, *userID)
	if err != nil {
		return "", "", fmt.Errorf("user not found: %w", err)
	}

	// Revoke old refresh token
	a.storage.RevokeRefreshToken(ctx, tokenHash)

	// Generate new tokens
	accessToken, err := a.jwtHandler.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	newRefreshToken, err := a.jwtHandler.GenerateRefreshToken()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store new refresh token
	newTokenHash := a.hashRefreshToken(newRefreshToken)
	expiresAt := time.Now().Add(a.jwtHandler.refreshTokenTTL)
	if err := a.storage.StoreRefreshToken(ctx, user.ID, newTokenHash, expiresAt); err != nil {
		return "", "", fmt.Errorf("failed to store refresh token: %w", err)
	}

	return accessToken, newRefreshToken, nil
}

// RevokeRefreshToken revokes a refresh token
func (a *AuthService) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	tokenHash := a.hashRefreshToken(refreshToken)
	return a.storage.RevokeRefreshToken(ctx, tokenHash)
}

// CreateMachineToken creates a new machine token
func (a *AuthService) CreateMachineToken(ctx context.Context, name string, permissions []string, createdByUserID *uuid.UUID, metadata map[string]interface{}) (string, *storage.MachineToken, error) {
	token, tokenHash, err := a.machineTokenGen.GenerateMachineToken()
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate token: %w", err)
	}

	machineToken, err := a.storage.CreateMachineToken(ctx, tokenHash, name, permissions, createdByUserID, metadata)
	if err != nil {
		return "", nil, fmt.Errorf("failed to store token: %w", err)
	}

	a.logAuthEvent(ctx, "machine_token_created", createdByUserID, &machineToken.ID, "", "", true, "")
	return token, machineToken, nil
}

// ListMachineTokens returns all machine tokens (without token values)
func (a *AuthService) ListMachineTokens(ctx context.Context) ([]*storage.MachineToken, error) {
	return a.storage.ListMachineTokens(ctx)
}

// DeleteMachineToken deletes a machine token
func (a *AuthService) DeleteMachineToken(ctx context.Context, tokenID uuid.UUID) error {
	return a.storage.DeleteMachineToken(ctx, tokenID)
}

// UpdateMachineToken updates token metadata
func (a *AuthService) UpdateMachineToken(ctx context.Context, tokenID uuid.UUID, name *string, metadata map[string]interface{}) error {
	return a.storage.UpdateMachineToken(ctx, tokenID, name, metadata)
}

// CreateUser creates a new user
func (a *AuthService) CreateUser(ctx context.Context, username, password, role string) (*storage.User, error) {
	passwordHash, err := a.passwordHasher.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	return a.storage.CreateUser(ctx, username, passwordHash, role)
}

// GetUserByID retrieves a user by ID
func (a *AuthService) GetUserByID(ctx context.Context, userID uuid.UUID) (*storage.User, error) {
	return a.storage.GetUserByID(ctx, userID)
}

// ListUsers returns all users
func (a *AuthService) ListUsers(ctx context.Context) ([]*storage.User, error) {
	return a.storage.ListUsers(ctx)
}

// UpdateUser updates user details
func (a *AuthService) UpdateUser(ctx context.Context, userID uuid.UUID, password *string, role *string) error {
	if password != nil {
		passwordHash, err := a.passwordHasher.HashPassword(*password)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}
		if err := a.storage.UpdateUserPassword(ctx, userID, passwordHash); err != nil {
			return err
		}
	}

	if role != nil {
		if err := a.storage.UpdateUserRole(ctx, userID, *role); err != nil {
			return err
		}
	}

	return nil
}

// DeleteUser deletes a user
func (a *AuthService) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	return a.storage.DeleteUser(ctx, userID)
}
