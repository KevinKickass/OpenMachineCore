-- Add workflow composition support
CREATE TABLE workflow_compositions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    workflow_id UUID REFERENCES workflows(id) ON DELETE CASCADE,
    instance_id VARCHAR(255) NOT NULL,
    composition JSONB NOT NULL,
    io_mapping JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(workflow_id, instance_id)
);

-- Index for fast JSONB queries
CREATE INDEX idx_workflow_compositions_workflow ON workflow_compositions(workflow_id);
CREATE INDEX idx_workflow_compositions_composition ON workflow_compositions USING GIN (composition);
CREATE INDEX idx_workflow_compositions_io_mapping ON workflow_compositions USING GIN (io_mapping);

-- Function to validate composition
CREATE OR REPLACE FUNCTION validate_composition()
RETURNS TRIGGER AS $$
BEGIN
    -- Ensure composition has required fields
    IF NOT (NEW.composition ? 'coupler' AND NEW.composition ? 'terminals') THEN
        RAISE EXCEPTION 'Invalid composition: missing coupler or terminals';
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_validate_composition
    BEFORE INSERT OR UPDATE ON workflow_compositions
    FOR EACH ROW
    EXECUTE FUNCTION validate_composition();

COMMENT ON TABLE workflow_compositions IS 'Device compositions for workflows';
COMMENT ON COLUMN workflow_compositions.composition IS 'Module composition (coupler + terminals)';
COMMENT ON COLUMN workflow_compositions.io_mapping IS 'Logical name to register mapping';
