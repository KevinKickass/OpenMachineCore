package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/KevinKickass/OpenMachineCore/internal/storage"
	"github.com/KevinKickass/OpenMachineCore/internal/workflow/definition"
	"github.com/google/uuid"
)

type Severity string

const (
	SevError   Severity = "error"
	SevWarning Severity = "warning"
)

type Issue struct {
	Code       string         `json:"code"`
	Severity   Severity       `json:"severity"`
	Message    string         `json:"message"`
	WorkflowID string         `json:"workflow_id,omitempty"`
	StepName   string         `json:"step_name,omitempty"`
	Field      string         `json:"field,omitempty"`
	Path       string         `json:"path,omitempty"` // JSON Pointer-ish ("/steps/0/device_id")
	Hint       string         `json:"hint,omitempty"`
	Meta       map[string]any `json:"meta,omitempty"`
}

type Report struct {
	Valid    bool    `json:"valid"`
	Errors   []Issue `json:"errors"`
	Warnings []Issue `json:"warnings"`
}

type Validator struct {
	storage *storage.PostgresClient
}

func NewValidator(storage *storage.PostgresClient) *Validator {
	return &Validator{storage: storage}
}

// ValidateByID validates a stored workflow and all reachable sub-workflows.
// Load failures return (Report{}, err). Definition/semantic failures are returned in the Report (err == nil).
func (v *Validator) ValidateByID(ctx context.Context, workflowID uuid.UUID) (Report, error) {
	rep := Report{}

	wf, _, err := v.storage.LoadWorkflow(ctx, workflowID)
	if err != nil {
		return rep, err
	}

	def, err := definition.ParseWorkflow(wf.Definition)
	if err != nil {
		rep.addError(Issue{
			Code:       "WORKFLOW_900",
			Severity:   SevError,
			Message:    fmt.Sprintf("Workflow definition JSON invalid: %v", err),
			WorkflowID: workflowID.String(),
			Field:      "definition",
			Path:       "/definition",
		})
		rep.finalize()
		return rep, nil
	}

	st := &walkState{
		v:        v,
		cache:    map[uuid.UUID]*definition.Workflow{workflowID: def},
		visiting: map[uuid.UUID]bool{},
		done:     map[uuid.UUID]bool{},
		stack:    make([]uuid.UUID, 0, 8),
		report:   &rep,
	}

	st.walk(ctx, workflowID)

	rep.finalize()
	return rep, nil
}

type walkState struct {
	v        *Validator
	cache    map[uuid.UUID]*definition.Workflow
	visiting map[uuid.UUID]bool
	done     map[uuid.UUID]bool
	stack    []uuid.UUID
	report   *Report
}

func (st *walkState) walk(ctx context.Context, wid uuid.UUID) {
	if st.done[wid] {
		return
	}
	if st.visiting[wid] {
		st.report.addError(Issue{
			Code:       "WORKFLOW_050",
			Severity:   SevError,
			Message:    "Circular workflow reference detected",
			WorkflowID: wid.String(),
		})
		return
	}

	def, err := st.getWorkflow(ctx, wid)
	if err != nil {
		st.report.addError(Issue{
			Code:       "WORKFLOW_901",
			Severity:   SevError,
			Message:    fmt.Sprintf("Failed to load workflow: %v", err),
			WorkflowID: wid.String(),
		})
		st.done[wid] = true
		return
	}
	if def == nil {
		st.report.addError(Issue{
			Code:       "WORKFLOW_003",
			Severity:   SevError,
			Message:    "Referenced workflow not found",
			WorkflowID: wid.String(),
		})
		st.done[wid] = true
		return
	}

	st.visiting[wid] = true
	st.stack = append(st.stack, wid)

	st.validateWorkflow(ctx, wid, def)

	st.stack = st.stack[:len(st.stack)-1]
	st.visiting[wid] = false
	st.done[wid] = true
}

func (st *walkState) getWorkflow(ctx context.Context, wid uuid.UUID) (*definition.Workflow, error) {
	if d, ok := st.cache[wid]; ok {
		return d, nil
	}

	exists, err := st.v.storage.WorkflowExists(ctx, wid)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	wf, _, err := st.v.storage.LoadWorkflow(ctx, wid)
	if err != nil {
		return nil, err
	}

	def, err := definition.ParseWorkflow(wf.Definition)
	if err != nil {
		st.report.addError(Issue{
			Code:       "WORKFLOW_900",
			Severity:   SevError,
			Message:    fmt.Sprintf("Workflow definition JSON invalid: %v", err),
			WorkflowID: wid.String(),
			Field:      "definition",
			Path:       "/definition",
		})
		return nil, nil
	}

	st.cache[wid] = def
	return def, nil
}

func (st *walkState) validateWorkflow(ctx context.Context, wid uuid.UUID, wf *definition.Workflow) {
	if strings.TrimSpace(wf.Name) == "" {
		st.report.addError(Issue{
			Code:       "WORKFLOW_001",
			Severity:   SevError,
			Message:    "Workflow name is required",
			WorkflowID: wid.String(),
			Field:      "name",
			Path:       "/name",
		})
	}
	if strings.TrimSpace(wf.Version) == "" {
		st.report.addWarning(Issue{
			Code:       "WORKFLOW_002",
			Severity:   SevWarning,
			Message:    "Workflow version is empty",
			WorkflowID: wid.String(),
			Field:      "version",
			Path:       "/version",
		})
	}
	if len(wf.Steps) == 0 {
		st.report.addError(Issue{
			Code:       "WORKFLOW_004",
			Severity:   SevError,
			Message:    "Workflow has no steps",
			WorkflowID: wid.String(),
			Field:      "steps",
			Path:       "/steps",
		})
		return
	}
	if wf.Loop != nil && wf.Loop.Enabled && wf.Loop.MaxCount < 0 {
		st.report.addError(Issue{
			Code:       "WORKFLOW_005",
			Severity:   SevError,
			Message:    "loop.max_count must be >= 0",
			WorkflowID: wid.String(),
			Field:      "loop.max_count",
			Path:       "/loop/max_count",
		})
	}

	for i := range wf.Steps {
		step := wf.Steps[i]
		base := fmt.Sprintf("/steps/%d", i)

		if strings.TrimSpace(step.Name) == "" {
			st.report.addError(Issue{
				Code:       "STEP_001",
				Severity:   SevError,
				Message:    "Step name is required",
				WorkflowID: wid.String(),
				Field:      "name",
				Path:       base + "/name",
				Meta:       map[string]any{"step_index": i},
			})
		}

		switch step.Type {
		case definition.StepTypeDevice:
			st.validateDeviceStep(ctx, wid, &step, i, base)
		case definition.StepTypeWorkflow:
			st.validateSubWorkflowStep(ctx, wid, &step, i, base)
		case definition.StepTypeWait:
			// ok
		default:
			st.report.addError(Issue{
				Code:       "STEP_002",
				Severity:   SevError,
				Message:    fmt.Sprintf("Unsupported step type: %s", step.Type),
				WorkflowID: wid.String(),
				Field:      "type",
				Path:       base + "/type",
				Meta:       map[string]any{"step_index": i},
			})
		}
	}
}

func (st *walkState) validateDeviceStep(ctx context.Context, wid uuid.UUID, step *definition.Step, idx int, base string) {
	stepName := step.Name

	if strings.TrimSpace(step.DeviceID) == "" {
		st.report.addError(Issue{
			Code:       "DEVICE_010",
			Severity:   SevError,
			Message:    "device_id is required for device step",
			WorkflowID: wid.String(),
			StepName:   stepName,
			Field:      "device_id",
			Path:       base + "/device_id",
			Meta:       map[string]any{"step_index": idx},
		})
	} else {
		exists, enabled, err := st.v.storage.DeviceExistsEnabledByName(ctx, step.DeviceID)
		if err != nil {
			st.report.addError(Issue{
				Code:       "DEVICE_999",
				Severity:   SevError,
				Message:    fmt.Sprintf("Device lookup failed: %v", err),
				WorkflowID: wid.String(),
				StepName:   stepName,
				Field:      "device_id",
				Path:       base + "/device_id",
				Meta:       map[string]any{"step_index": idx},
			})
		} else if !exists {
			st.report.addError(Issue{
				Code:       "DEVICE_001",
				Severity:   SevError,
				Message:    fmt.Sprintf("Device not found: %s", step.DeviceID),
				WorkflowID: wid.String(),
				StepName:   stepName,
				Field:      "device_id",
				Path:       base + "/device_id",
				Meta:       map[string]any{"step_index": idx},
			})
		} else if !enabled {
			st.report.addError(Issue{
				Code:       "DEVICE_002",
				Severity:   SevError,
				Message:    fmt.Sprintf("Device is disabled: %s", step.DeviceID),
				WorkflowID: wid.String(),
				StepName:   stepName,
				Field:      "device_id",
				Path:       base + "/device_id",
				Meta:       map[string]any{"step_index": idx},
			})
		}
	}

	op := strings.TrimSpace(step.Operation)
	if op == "" {
		st.report.addError(Issue{
			Code:       "DEVICE_011",
			Severity:   SevError,
			Message:    "operation is required for device step",
			WorkflowID: wid.String(),
			StepName:   stepName,
			Field:      "operation",
			Path:       base + "/operation",
			Meta:       map[string]any{"step_index": idx},
		})
		return
	}

	supported := map[string]struct{}{
		"read": {}, "write": {}, "read_logical": {}, "write_logical": {}, "read_register": {}, "write_register": {},
	}
	if _, ok := supported[op]; !ok {
		st.report.addError(Issue{
			Code:       "DEVICE_012",
			Severity:   SevError,
			Message:    fmt.Sprintf("Unsupported operation: %s", op),
			WorkflowID: wid.String(),
			StepName:   stepName,
			Field:      "operation",
			Path:       base + "/operation",
			Meta:       map[string]any{"step_index": idx},
		})
		return
	}

	// Parameter presence checks as WARNING because values may come from execution input.
	required := requiredParamsForOp(op)
	for _, k := range required {
		if step.Parameters == nil {
			st.report.addWarning(Issue{
				Code:       "DEVICE_020",
				Severity:   SevWarning,
				Message:    fmt.Sprintf("Missing parameter '%s' (step.parameters is empty)", k),
				WorkflowID: wid.String(),
				StepName:   stepName,
				Field:      "parameters." + k,
				Path:       base + "/parameters",
				Hint:       "Define it in step.parameters or provide it in the execution input",
				Meta:       map[string]any{"step_index": idx, "param": k},
			})
			continue
		}
		if _, ok := step.Parameters[k]; !ok {
			st.report.addWarning(Issue{
				Code:       "DEVICE_020",
				Severity:   SevWarning,
				Message:    fmt.Sprintf("Missing parameter '%s'", k),
				WorkflowID: wid.String(),
				StepName:   stepName,
				Field:      "parameters." + k,
				Path:       base + "/parameters",
				Hint:       "Define it in step.parameters or provide it in the execution input",
				Meta:       map[string]any{"step_index": idx, "param": k},
			})
		}
	}

	// Light static checks if register_type is present.
	if step.Parameters != nil && (op == "read" || op == "write") {
		if v, ok := step.Parameters["register_type"]; ok {
			if s, ok := v.(string); ok && s != "holding" && s != "input" {
				st.report.addError(Issue{
					Code:       "DEVICE_021",
					Severity:   SevError,
					Message:    fmt.Sprintf("Invalid register_type: %s", s),
					WorkflowID: wid.String(),
					StepName:   stepName,
					Field:      "parameters.register_type",
					Path:       base + "/parameters/register_type",
					Meta:       map[string]any{"step_index": idx},
				})
			}
		}
	}
}

func requiredParamsForOp(op string) []string {
	switch op {
	case "read":
		return []string{"register_type", "address"}
	case "write":
		return []string{"register_type", "address", "value"}
	case "read_logical":
		return []string{"register"}
	case "write_logical":
		return []string{"register", "value"}
	case "read_register":
		return []string{"register"}
	case "write_register":
		return []string{"register", "value"}
	default:
		return nil
	}
}

func (st *walkState) validateSubWorkflowStep(ctx context.Context, wid uuid.UUID, step *definition.Step, idx int, base string) {
	stepName := step.Name

	if strings.TrimSpace(step.WorkflowID) == "" {
		st.report.addError(Issue{
			Code:       "WORKFLOW_010",
			Severity:   SevError,
			Message:    "workflow_id is required for workflow step",
			WorkflowID: wid.String(),
			StepName:   stepName,
			Field:      "workflow_id",
			Path:       base + "/workflow_id",
			Meta:       map[string]any{"step_index": idx},
		})
		return
	}

	subID, err := uuid.Parse(step.WorkflowID)
	if err != nil {
		st.report.addError(Issue{
			Code:       "WORKFLOW_011",
			Severity:   SevError,
			Message:    fmt.Sprintf("Invalid workflow_id: %v", err),
			WorkflowID: wid.String(),
			StepName:   stepName,
			Field:      "workflow_id",
			Path:       base + "/workflow_id",
			Meta:       map[string]any{"step_index": idx},
		})
		return
	}

	exists, err := st.v.storage.WorkflowExists(ctx, subID)
	if err != nil {
		st.report.addError(Issue{
			Code:       "WORKFLOW_999",
			Severity:   SevError,
			Message:    fmt.Sprintf("Workflow lookup failed: %v", err),
			WorkflowID: wid.String(),
			StepName:   stepName,
			Field:      "workflow_id",
			Path:       base + "/workflow_id",
			Meta:       map[string]any{"step_index": idx},
		})
		return
	}
	if !exists {
		st.report.addError(Issue{
			Code:       "WORKFLOW_003",
			Severity:   SevError,
			Message:    fmt.Sprintf("Referenced workflow not found: %s", subID.String()),
			WorkflowID: wid.String(),
			StepName:   stepName,
			Field:      "workflow_id",
			Path:       base + "/workflow_id",
			Meta:       map[string]any{"step_index": idx},
		})
		return
	}

	// Cycle detection: sub-workflow already on stack.
	if st.visiting[subID] {
		st.report.addError(Issue{
			Code:       "WORKFLOW_050",
			Severity:   SevError,
			Message:    "Circular workflow reference detected",
			WorkflowID: wid.String(),
			StepName:   stepName,
			Field:      "workflow_id",
			Path:       base + "/workflow_id",
			Meta: map[string]any{
				"step_index": idx,
				"cycle":      st.cyclePath(subID),
			},
		})
		return
	}

	st.walk(ctx, subID)
}

func (st *walkState) cyclePath(target uuid.UUID) []string {
	start := -1
	for i := range st.stack {
		if st.stack[i] == target {
			start = i
			break
		}
	}
	if start == -1 {
		return []string{target.String()}
	}

	out := make([]string, 0, len(st.stack)-start+1)
	for _, id := range st.stack[start:] {
		out = append(out, id.String())
	}
	out = append(out, target.String())
	return out
}

func (r *Report) addError(i Issue) {
	if i.Severity == "" {
		i.Severity = SevError
	}
	r.Errors = append(r.Errors, i)
}

func (r *Report) addWarning(i Issue) {
	if i.Severity == "" {
		i.Severity = SevWarning
	}
	r.Warnings = append(r.Warnings, i)
}

func (r *Report) finalize() {
	sortIssues(r.Errors)
	sortIssues(r.Warnings)
	r.Valid = len(r.Errors) == 0
}

func sortIssues(list []Issue) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.WorkflowID != b.WorkflowID {
			return a.WorkflowID < b.WorkflowID
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
}
