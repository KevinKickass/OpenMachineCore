package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// User models
type User struct {
	ID                  uuid.UUID  `json:"id"`
	Username            string     `json:"username"`
	PasswordHash        string     `json:"-"` // Never expose in JSON
	Role                string     `json:"role"`
	CreatedAt           time.Time  `json:"created_at"`
	LastLoginAt         *time.Time `json:"last_login_at"`
	FailedLoginAttempts int        `json:"-"`
	LockedUntil         *time.Time `json:"locked_until,omitempty"`
}

type MachineToken struct {
	ID              uuid.UUID              `json:"id"`
	TokenHash       string                 `json:"-"` // Never expose
	Name            string                 `json:"name"`
	Permissions     []string               `json:"permissions"`
	CreatedAt       time.Time              `json:"created_at"`
	LastUsedAt      *time.Time             `json:"last_used_at"`
	CreatedByUserID *uuid.UUID             `json:"created_by_user_id"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// GetUserByUsername retrieves a user by username
func (p *PostgresClient) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := p.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, role, created_at, last_login_at, 
		       failed_login_attempts, locked_until
		FROM users
		WHERE username = $1
	`, username).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Role,
		&user.CreatedAt, &user.LastLoginAt, &user.FailedLoginAttempts, &user.LockedUntil,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

// CreateUser creates a new user
func (p *PostgresClient) CreateUser(ctx context.Context, username, passwordHash, role string) (*User, error) {
	var user User
	err := p.pool.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role)
		VALUES ($1, $2, $3)
		RETURNING id, username, role, created_at, last_login_at, failed_login_attempts, locked_until
	`, username, passwordHash, role).Scan(
		&user.ID, &user.Username, &user.Role, &user.CreatedAt,
		&user.LastLoginAt, &user.FailedLoginAttempts, &user.LockedUntil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return &user, nil
}

// UpdateLastLogin updates the last login timestamp
func (p *PostgresClient) UpdateLastLogin(ctx context.Context, userID uuid.UUID) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE users SET last_login_at = NOW() WHERE id = $1
	`, userID)
	return err
}

// IncrementFailedLoginAttempts increments failed login counter
func (p *PostgresClient) IncrementFailedLoginAttempts(ctx context.Context, userID uuid.UUID) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE users 
		SET failed_login_attempts = failed_login_attempts + 1,
		    locked_until = CASE 
		        WHEN failed_login_attempts + 1 >= 5 THEN NOW() + INTERVAL '15 minutes'
		        ELSE locked_until
		    END
		WHERE id = $1
	`, userID)
	return err
}

// ResetFailedLoginAttempts resets failed login counter
func (p *PostgresClient) ResetFailedLoginAttempts(ctx context.Context, userID uuid.UUID) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE users 
		SET failed_login_attempts = 0, locked_until = NULL 
		WHERE id = $1
	`, userID)
	return err
}

// Machine Token Methods
func (p *PostgresClient) CreateMachineToken(ctx context.Context, tokenHash, name string, permissions []string, createdByUserID *uuid.UUID, metadata map[string]interface{}) (*MachineToken, error) {
	var token MachineToken
	err := p.pool.QueryRow(ctx, `
		INSERT INTO machine_tokens (token_hash, name, permissions, created_by_user_id, metadata)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, token_hash, name, permissions, created_at, last_used_at, created_by_user_id, metadata
	`, tokenHash, name, permissions, createdByUserID, metadata).Scan(
		&token.ID, &token.TokenHash, &token.Name, &token.Permissions,
		&token.CreatedAt, &token.LastUsedAt, &token.CreatedByUserID, &token.Metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create machine token: %w", err)
	}
	return &token, nil
}

func (p *PostgresClient) GetMachineTokenByHash(ctx context.Context, tokenHash string) (*MachineToken, error) {
	var token MachineToken
	err := p.pool.QueryRow(ctx, `
		SELECT id, token_hash, name, permissions, created_at, last_used_at, created_by_user_id, metadata
		FROM machine_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(
		&token.ID, &token.TokenHash, &token.Name, &token.Permissions,
		&token.CreatedAt, &token.LastUsedAt, &token.CreatedByUserID, &token.Metadata,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("token not found")
		}
		return nil, fmt.Errorf("failed to get machine token: %w", err)
	}
	return &token, nil
}

func (p *PostgresClient) UpdateMachineTokenLastUsed(ctx context.Context, tokenID uuid.UUID) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE machine_tokens SET last_used_at = NOW() WHERE id = $1
	`, tokenID)
	return err
}

func (p *PostgresClient) ListMachineTokens(ctx context.Context) ([]*MachineToken, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, name, permissions, created_at, last_used_at, created_by_user_id, metadata
		FROM machine_tokens
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list machine tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*MachineToken
	for rows.Next() {
		var token MachineToken
		err := rows.Scan(
			&token.ID, &token.Name, &token.Permissions, &token.CreatedAt,
			&token.LastUsedAt, &token.CreatedByUserID, &token.Metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan machine token: %w", err)
		}
		tokens = append(tokens, &token)
	}
	return tokens, nil
}

func (p *PostgresClient) DeleteMachineToken(ctx context.Context, tokenID uuid.UUID) error {
	result, err := p.pool.Exec(ctx, `DELETE FROM machine_tokens WHERE id = $1`, tokenID)
	if err != nil {
		return fmt.Errorf("failed to delete machine token: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Refresh Token Methods
func (p *PostgresClient) StoreRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt)
	return err
}

func (p *PostgresClient) GetRefreshToken(ctx context.Context, tokenHash string) (*uuid.UUID, error) {
	var userID uuid.UUID
	var expiresAt time.Time
	var revokedAt *time.Time

	err := p.pool.QueryRow(ctx, `
		SELECT user_id, expires_at, revoked_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(&userID, &expiresAt, &revokedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("refresh token not found")
		}
		return nil, fmt.Errorf("failed to get refresh token: %w", err)
	}

	if revokedAt != nil {
		return nil, fmt.Errorf("refresh token revoked")
	}

	if time.Now().After(expiresAt) {
		return nil, fmt.Errorf("refresh token expired")
	}

	return &userID, nil
}

func (p *PostgresClient) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = NOW() WHERE token_hash = $1
	`, tokenHash)
	return err
}

func (p *PostgresClient) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = NOW() 
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	return err
}

// Auth Event Logging
func (p *PostgresClient) LogAuthEvent(ctx context.Context, eventType string, userID, machineTokenID *uuid.UUID, ipAddress, userAgent string, success bool, reason string) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO auth_events (event_type, user_id, machine_token_id, ip_address, user_agent, success, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, eventType, userID, machineTokenID, ipAddress, userAgent, success, reason)
	return err
}

func (p *PostgresClient) GetUserByID(ctx context.Context, userID uuid.UUID) (*User, error) {
	var user User
	err := p.pool.QueryRow(ctx, `
		SELECT id, username, role, created_at, last_login_at, failed_login_attempts, locked_until
		FROM users WHERE id = $1
	`, userID).Scan(
		&user.ID, &user.Username, &user.Role, &user.CreatedAt,
		&user.LastLoginAt, &user.FailedLoginAttempts, &user.LockedUntil,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

func (p *PostgresClient) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, username, role, created_at, last_login_at, failed_login_attempts, locked_until
		FROM users ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var user User
		err := rows.Scan(
			&user.ID, &user.Username, &user.Role, &user.CreatedAt,
			&user.LastLoginAt, &user.FailedLoginAttempts, &user.LockedUntil,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, &user)
	}
	return users, nil
}

func (p *PostgresClient) UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE users SET password_hash = $1 WHERE id = $2
	`, passwordHash, userID)
	return err
}

func (p *PostgresClient) UpdateUserRole(ctx context.Context, userID uuid.UUID, role string) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE users SET role = $1 WHERE id = $2
	`, role, userID)
	return err
}

func (p *PostgresClient) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	result, err := p.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (p *PostgresClient) UpdateMachineToken(ctx context.Context, tokenID uuid.UUID, name *string, metadata map[string]interface{}) error {
	if name != nil {
		_, err := p.pool.Exec(ctx, `
			UPDATE machine_tokens SET name = $1 WHERE id = $2
		`, *name, tokenID)
		if err != nil {
			return err
		}
	}

	if metadata != nil {
		_, err := p.pool.Exec(ctx, `
			UPDATE machine_tokens SET metadata = $1 WHERE id = $2
		`, metadata, tokenID)
		if err != nil {
			return err
		}
	}

	return nil
}
