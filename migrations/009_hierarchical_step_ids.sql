-- Migration 009: Add hierarchical step ID system
-- Enables tracking of call-stack and nested subroutines

-- Add columns to workflow_executions table
ALTER TABLE workflow_executions
ADD COLUMN current_step_id VARCHAR(500),
ADD COLUMN call_stack JSONB;

-- Create index for fast step ID lookup
CREATE INDEX idx_workflow_executions_current_step_id ON workflow_executions(current_step_id);

-- Add columns to execution_steps table for hierarchical tracking
ALTER TABLE execution_steps
ADD COLUMN hierarchical_step_id VARCHAR(500),
ADD COLUMN depth INT DEFAULT 0;

-- Create indexes for hierarchical step lookups
CREATE INDEX idx_execution_steps_hier_id ON execution_steps(hierarchical_step_id);
CREATE INDEX idx_execution_steps_depth ON execution_steps(depth);

-- Migration note:
-- current_step INT column remains for backward compatibility
-- call_stack stores JSON array of CallFrame objects:
-- [
--   {"workflow_id": "uuid", "program_name": "main", "step_number": "10"},
--   {"workflow_id": "uuid", "program_name": "sub_pick", "step_number": "20"}
-- ]
--
-- hierarchical_step_id format: "main:S10:sub_pick:S20:sub_gripper:S5"
-- depth indicates nesting level (0=main, 1=first sub, 2=nested sub, etc.)
