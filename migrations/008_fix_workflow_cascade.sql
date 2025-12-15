-- Fix workflow_executions and execution_steps foreign keys to use CASCADE

-- 1. Fix workflow_executions -> workflows
ALTER TABLE workflow_executions
DROP CONSTRAINT IF EXISTS workflow_executions_workflow_id_fkey;

ALTER TABLE workflow_executions
DROP CONSTRAINT IF EXISTS fk_workflow;

ALTER TABLE workflow_executions
ADD CONSTRAINT workflow_executions_workflow_id_fkey
FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE;

-- 2. Fix execution_steps -> workflow_executions
ALTER TABLE execution_steps
DROP CONSTRAINT IF EXISTS execution_steps_execution_id_fkey;

ALTER TABLE execution_steps
DROP CONSTRAINT IF EXISTS fk_execution;

ALTER TABLE execution_steps
ADD CONSTRAINT execution_steps_execution_id_fkey
FOREIGN KEY (execution_id) REFERENCES workflow_executions(id) ON DELETE CASCADE;

-- 3. Fix execution_events -> workflow_executions
ALTER TABLE execution_events
DROP CONSTRAINT IF EXISTS execution_events_execution_id_fkey;

ALTER TABLE execution_events
DROP CONSTRAINT IF EXISTS fk_execution_event;

ALTER TABLE execution_events
ADD CONSTRAINT execution_events_execution_id_fkey
FOREIGN KEY (execution_id) REFERENCES workflow_executions(id) ON DELETE CASCADE;
