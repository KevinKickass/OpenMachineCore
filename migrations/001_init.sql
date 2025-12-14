-- migrations/001_init.sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE device_profiles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    profile_name VARCHAR(255) UNIQUE NOT NULL,
    vendor VARCHAR(255),
    model VARCHAR(255),
    definition JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE devices (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_name VARCHAR(255) UNIQUE NOT NULL,
    profile_id UUID REFERENCES device_profiles(id),
    ip_address INET NOT NULL,
    port INT DEFAULT 502,
    unit_id INT DEFAULT 1,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE io_mappings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id UUID REFERENCES devices(id) ON DELETE CASCADE,
    logical_name VARCHAR(255) NOT NULL,
    register_name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(device_id, logical_name)
);

CREATE TABLE workflows (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    workflow_name VARCHAR(255) UNIQUE NOT NULL,
    definition JSONB NOT NULL,
    active BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_devices_profile ON devices(profile_id);
CREATE INDEX idx_io_mappings_device ON io_mappings(device_id);
CREATE INDEX idx_workflows_active ON workflows(active);
