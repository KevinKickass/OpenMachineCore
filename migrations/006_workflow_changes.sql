-- migrations/004_workflow_engine.sql

-- Workflow executions
CREATE TABLE IF NOT EXISTS workflow_executions (
    id UUID PRIMARY KEY,
    workflow_id UUID NOT NULL REFERENCES workflows(id),
    status VARCHAR(20) NOT NULL,
    current_step INT NOT NULL DEFAULT 0,
    input JSONB,
    output JSONB,
    error TEXT,
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    CONSTRAINT fk_workflow FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE INDEX idx_workflow_executions_workflow_id ON workflow_executions(workflow_id);
CREATE INDEX idx_workflow_executions_status ON workflow_executions(status);

-- Execution steps
CREATE TABLE IF NOT EXISTS execution_steps (
    id UUID PRIMARY KEY,
    execution_id UUID NOT NULL REFERENCES workflow_executions(id),
    step_index INT NOT NULL,
    step_name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL,
    input JSONB,
    output JSONB,
    error TEXT,
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    CONSTRAINT fk_execution FOREIGN KEY (execution_id) REFERENCES workflow_executions(id) ON DELETE CASCADE
);

CREATE INDEX idx_execution_steps_execution_id ON execution_steps(execution_id);

-- Execution events for streaming
CREATE TABLE IF NOT EXISTS execution_events (
    id UUID PRIMARY KEY,
    execution_id UUID NOT NULL REFERENCES workflow_executions(id),
    event_type VARCHAR(50) NOT NULL,
    payload JSONB,
    timestamp TIMESTAMP NOT NULL,
    CONSTRAINT fk_execution_event FOREIGN KEY (execution_id) REFERENCES workflow_executions(id) ON DELETE CASCADE
);

CREATE INDEX idx_execution_events_execution_id ON execution_events(execution_id);
CREATE INDEX idx_execution_events_timestamp ON execution_events(timestamp);
