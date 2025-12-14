package devices

import (
	"encoding/json"
	"fmt"
	"strings"

	_ "embed"
	"github.com/KevinKickass/OpenMachineCore/internal/types"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed schema/device-profile-v1.json
var deviceProfileSchemaJSON string

type Validator struct {
	schema *jsonschema.Schema
}

func NewValidator() (*Validator, error) {
	compiler := jsonschema.NewCompiler()

	if err := compiler.AddResource("device-profile-v1.json",
		strings.NewReader(deviceProfileSchemaJSON)); err != nil {
		return nil, fmt.Errorf("failed to add schema resource: %w", err)
	}

	schema, err := compiler.Compile("device-profile-v1.json")
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", err)
	}

	return &Validator{schema: schema}, nil
}

func (v *Validator) ValidateProfile(data []byte) error {
	var profile interface{}
	if err := json.Unmarshal(data, &profile); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if err := v.schema.Validate(profile); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}

	return nil
}

func (v *Validator) ValidateProfileDefinition(profile *types.DeviceProfileDefinition) error {
	data, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}

	return v.ValidateProfile(data)
}
