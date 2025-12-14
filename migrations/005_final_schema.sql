-- Final clean schema for Phase 1

-- Devices table (core device info)
-- Already exists, just document
COMMENT ON TABLE devices IS 'Physical devices (couplers) with connection info';
COMMENT ON COLUMN devices.device_name IS 'Unique device instance name';
COMMENT ON COLUMN devices.ip_address IS 'Device IP address or hostname';
COMMENT ON COLUMN devices.enabled IS 'Auto-load on boot if true';

-- Device compositions (modular assembly)
-- Already exists, just document
COMMENT ON TABLE device_compositions IS 'Runtime device composition from modules (coupler + terminals)';
COMMENT ON COLUMN device_compositions.composition IS 'JSONB: {coupler: {...}, terminals: [{...}]}';
COMMENT ON COLUMN device_compositions.io_mapping IS 'JSONB: {logical_name: register_name}';

-- Example queries for documentation
COMMENT ON DATABASE openmachinecore IS 'OpenMachineCore - Open Source Machine Control System

Device Query Examples:
- List all devices: SELECT * FROM devices WHERE enabled = true;
- Get composition: SELECT dc.* FROM device_compositions dc JOIN devices d ON dc.device_id = d.id WHERE d.device_name = ''xyz'';
- Find by IP: SELECT * FROM devices WHERE ip_address = ''192.168.1.100'';
';

-- Create view for easier device querying
CREATE OR REPLACE VIEW v_devices_full AS
SELECT 
    d.id,
    d.device_name,
    d.ip_address,
    d.port,
    d.unit_id,
    d.enabled,
    d.created_at,
    d.updated_at,
    dc.composition,
    dc.io_mapping,
    dc.composition->'coupler'->>'module' AS coupler_module,
    jsonb_array_length(dc.composition->'terminals') AS terminal_count
FROM devices d
LEFT JOIN device_compositions dc ON d.id = dc.device_id;

COMMENT ON VIEW v_devices_full IS 'Complete device view with composition and metadata';
