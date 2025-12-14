package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
)

const machineTokenPrefix = "omc_"

type MachineTokenGenerator struct{}

func NewMachineTokenGenerator() *MachineTokenGenerator {
	return &MachineTokenGenerator{}
}

// GenerateMachineToken creates a new machine token
// Format: omc_<uuid>_<random_secret>
func (m *MachineTokenGenerator) GenerateMachineToken() (string, string, error) {
	id := uuid.New()

	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate secret: %w", err)
	}
	secret := hex.EncodeToString(secretBytes)

	token := fmt.Sprintf("%s%s_%s", machineTokenPrefix, id.String(), secret)
	hash := m.HashToken(token)

	return token, hash, nil
}

// HashToken hashes a machine token for storage
func (m *MachineTokenGenerator) HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// ValidateTokenFormat checks if token has correct format
func (m *MachineTokenGenerator) ValidateTokenFormat(token string) bool {
	if len(token) < len(machineTokenPrefix)+36+1+64 {
		return false
	}
	if token[:len(machineTokenPrefix)] != machineTokenPrefix {
		return false
	}
	return true
}
