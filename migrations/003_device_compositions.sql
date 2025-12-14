-- Separate table for standalone device compositions
CREATE TABLE device_compositions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id UUID REFERENCES devices(id) ON DELETE CASCADE,
    instance_id VARCHAR(255) UNIQUE NOT NULL,
    composition JSONB NOT NULL,
    io_mapping JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_device_compositions_device ON device_compositions(device_id);
CREATE INDEX idx_device_compositions_instance ON device_compositions(instance_id);
CREATE INDEX idx_device_compositions_composition ON device_compositions USING GIN (composition);

COMMENT ON TABLE device_compositions IS 'Device module compositions (standalone devices)';
