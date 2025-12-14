-- Drop unused tables from initial design
DROP TABLE IF EXISTS io_mappings CASCADE;
DROP TABLE IF EXISTS device_profiles CASCADE;

-- Remove profile_id from devices (no longer needed)
ALTER TABLE devices DROP COLUMN IF EXISTS profile_id;

-- Keep workflow tables for later, but add comment
COMMENT ON TABLE workflows IS 'Workflow definitions (Phase 1 - to be implemented)';
COMMENT ON TABLE workflow_compositions IS 'Device-to-workflow assignments (Phase 1 - to be implemented)';

-- Add updated_at trigger for device_compositions
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_device_compositions_updated_at 
    BEFORE UPDATE ON device_compositions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_devices_updated_at
    BEFORE UPDATE ON devices
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
