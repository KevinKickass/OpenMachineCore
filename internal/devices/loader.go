package devices

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/KevinKickass/OpenMachineCore/internal/types"
)

type ProfileLoader struct {
	cache       sync.Map
	validator   *Validator
	searchPaths []string
}

func NewProfileLoader(searchPaths []string) (*ProfileLoader, error) {
	validator, err := NewValidator()
	if err != nil {
		return nil, fmt.Errorf("failed to create validator: %w", err)
	}

	return &ProfileLoader{
		validator:   validator,
		searchPaths: searchPaths,
	}, nil
}

func (l *ProfileLoader) Load(profilePath string) (*types.DeviceProfileDefinition, error) {
	// Cache-Check
	if cached, ok := l.cache.Load(profilePath); ok {
		return cached.(*types.DeviceProfileDefinition), nil
	}

	var data []byte
	var err error
	var foundPath string

	for _, searchPath := range l.searchPaths {
		fullPath := filepath.Join(searchPath, profilePath+".json")
		data, err = os.ReadFile(fullPath)
		if err == nil {
			foundPath = fullPath
			break
		}
	}

	if data == nil {
		return nil, fmt.Errorf("profile not found: %s (searched in: %v)", profilePath, l.searchPaths)
	}

	if err := l.validator.ValidateProfile(data); err != nil {
		return nil, fmt.Errorf("validation failed for %s: %w", foundPath, err)
	}

	var profile types.DeviceProfileDefinition
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to unmarshal profile: %w", err)
	}

	l.cache.Store(profilePath, &profile)

	return &profile, nil
}

func (l *ProfileLoader) ClearCache() {
	l.cache.Range(func(key, value interface{}) bool {
		l.cache.Delete(key)
		return true
	})
}
