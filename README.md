# OpenMachineCore

[![CI](https://img.shields.io/github/actions/workflow/status/KevinKickass/OpenMachineCore/release.yml?style=for-the-badge)](https://github.com/KevinKickass/OpenMachineCore/actions/workflows/release.yml)
[![Latest Release](https://img.shields.io/github/v/release/KevinKickass/OpenMachineCore?sort=semver&style=for-the-badge)](https://github.com/KevinKickass/OpenMachineCore/releases/latest)
[![License](https://img.shields.io/github/license/KevinKickass/OpenMachineCore?style=for-the-badge)](https://github.com/KevinKickass/OpenMachineCore/blob/main/LICENSE)
[![Docker](https://img.shields.io/badge/docker-Dockerfile-2496ED?logo=docker&logoColor=white&style=for-the-badge)](https://github.com/KevinKickass/OpenMachineCore/blob/main/Dockerfile)
[![Buy Me a Coffee](https://img.shields.io/badge/buy_me_a_coffee-FFDD00?logo=buy-me-a-coffee&logoColor=black&style=for-the-badge)](https://www.buymeacoffee.com/KevinKickass)



OpenMachineCore (Backend) is a Go-based automation platform for industrial machine control. It combines a Modbus TCP - EtherCat and more planned - device layer, a workflow engine, and a machine state controller with REST, WebSocket and gRPC APIs. 

OpenMachineCore HMI is coming soon for displaying machine state and give the user a place for basic controlling of the machine.

OpenMachineCore Configurator is coming soon too to help set up workflows, devices and so on

You can also use your own HMI and use the APIs to control/display the machine states and devices

## Still work in progress - Not production ready

## Features

- **Two-tier authentication & authorization system:**
  - **Machine Tokens:** Permanent tokens for HMI/Configurators with operator-level access (never expire)
  - **User JWT:** Short-lived tokens for Technician/Admin with refresh token support (60min + 7 days)
  - **Permission-based access control:** Operator / Technician / Admin roles
  - **Argon2id password hashing** with automatic account locking after failed attempts
  - **WebSocket authentication** via first-message protocol
  - **Audit logging** for all authentication events
- **Workflow engine with:**
  - JSON-defined workflows
  - Step types: `device`, `workflow` (sub-workflow), `wait`
  - Optional loop configuration (continuous or fixed count)
- **Machine controller with high-level modes:**
  - Stop (controlled stop)
  - Home (move to reference position)
  - Start (automatic / production loop)
- **Modbus TCP device management** with logical I/O mapping
- **REST API** for devices, workflows, machine control, modules, and authentication
- **gRPC streaming** for workflow execution events
- **WebSocket streaming** for status, I/O and workflow updates
- **PostgreSQL-backed storage** for devices, workflows, executions, users, and tokens

## Architecture Overview

```

cmd/server/main.go
│
▼
LifecycleManager (internal/system)
├── DeviceManager      (internal/devices)
├── Workflow Engine    (internal/workflow/engine, executor, streaming, definition)
├── Machine Controller (internal/machine)
└── Auth Service       (internal/auth) ← NEW

APIs:

- REST (internal/api/rest) with Auth Middleware
- gRPC (internal/api/grpc, api/proto)
- WebSocket Hub (internal/api/websocket) with Auth

Storage:

- PostgreSQL (internal/storage)

Modbus:

- internal/modbus

```

## Getting Started

### Prerequisites

- Go 1.21 or higher
- PostgreSQL 13 or higher
- Optional: Modbus simulator for testing

### Clone and Build

```bash
git clone https://github.com/KevinKickass/OpenMachineCore.git
cd OpenMachineCore

go mod download
make build
```

The binary is built as:

```bash
./bin/openmachinecore
```


### Configuration

Create `configs/config.yaml` (adapt to your environment):

```yaml
server:
  http_port: 8080
  grpc_port: 50051
  shutdown_timeout: 30s

database:
  host: localhost
  port: 5432
  database: openmachinecore
  user: omc
  password: omc
  max_connections: 25

# Authentication configuration
auth:
  jwt_secret_env: "JWT_SECRET"              # Environment variable name
  access_token_ttl: 60m                     # 60 minutes
  refresh_token_ttl: 168h                   # 7 days
  max_failed_login_attempts: 5
  account_lock_duration: 15m

modbus:
  default_timeout: 1s
  default_poll_interval: 100ms

device_profiles:
  search_paths:
    - "device-descriptors/vendors"
    - "/etc/openmachinecore/profiles"
```


### Database

Create the database and apply migrations:

```bash
createdb openmachinecore
psql -d openmachinecore -f migrations/001_initial_schema.sql
psql -d openmachinecore -f migrations/002_workflow_engine.sql
psql -d openmachinecore -f migrations/003_auth_system.sql
```


### Initial Setup - Authentication

#### 1. Set JWT Secret (Production)

```bash
# IMPORTANT: Set a secure JWT secret in production
export JWT_SECRET="your-very-secure-secret-key-min-32-characters"
```

For development, a default secret is used (with warning).

#### 2. Create Admin User

```bash
./bin/openmachinecore --create-admin
```

Output:

```
Admin User Created Successfully!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Username: admin
Password: admin123
Role:     admin
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

CHANGE THE PASSWORD IMMEDIATELY IN PRODUCTION!
```


#### 3. Generate Machine Token for HMI

```bash
./bin/openmachinecore --generate-machine-token "HMI Production Line 1"
```

Output:

```
Machine Token Generated Successfully!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Name:        HMI Production Line 1
ID:          550e8400-e29b-41d4-a716-446655440000
Permissions: [operator]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Token: omc_550e8400-e29b-41d4-a716-446655440000_a8f5f167f44f4964e6c998dee827110c
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

IMPORTANT: Save this token securely!
   It will NOT be displayed again.
```


#### 4. Use Machine Token in HMI

```bash
# Set as environment variable
export OMC_API_KEY="omc_550e8400-e29b-41d4-a716-446655440000_a8f5f167f44f4964e6c998dee827110c"

# Use in API requests
curl http://localhost:8080/api/v1/machine/status \
  -H "Authorization: Bearer $OMC_API_KEY"
```


### Run

```bash
# Production with JWT secret
JWT_SECRET="your-secure-secret" ./bin/openmachinecore

# Development (uses default secret with warning)
./bin/openmachinecore
```

By default:

- REST API: `http://localhost:8080/api/v1`
- gRPC: `localhost:50051`
- WebSocket: `ws://localhost:8080/api/v1/ws/live`


## Authentication \& Authorization

### Permission Levels

| Permission | Machine Token | Technician JWT | Admin JWT |
| :-- | :-- | :-- | :-- |
| Machine Control (start/stop/home) | ✅ | ✅ | ✅ |
| Device Read | ✅ | ✅ | ✅ |
| Workflow Execute | ✅ | ✅ | ✅ |
| Device Write | ❌ | ✅ | ✅ |
| Diagnostics \& Logs | ❌ | ✅ | ✅ |
| Workflow CRUD | ❌ | ❌ | ✅ |
| Device Setup | ❌ | ❌ | ✅ |
| User Management | ❌ | ❌ | ✅ |
| Token Management | ❌ | ❌ | ✅ |

### User Authentication (Technician/Admin)

#### Login

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "admin123"
  }'
```

Response:

```json
{
  "access_token": "eyJhbGc...",
  "refresh_token": "a8f5f167...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```


#### Use JWT Token

```bash
TOKEN="eyJhbGc..."

curl http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $TOKEN"
```


#### Refresh Token

```bash
curl -X POST http://localhost:8080/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{
    "refresh_token": "a8f5f167..."
  }'
```


#### Logout

```bash
curl -X POST http://localhost:8080/api/v1/auth/logout \
  -H "Content-Type: application/json" \
  -d '{
    "refresh_token": "a8f5f167..."
  }'
```


### WebSocket Authentication

```javascript
const ws = new WebSocket('ws://localhost:8080/api/v1/ws/live');

ws.onopen = () => {
  // First message MUST be authentication
  ws.send(JSON.stringify({
    type: 'auth',
    token: 'omc_550e8400-...' // or JWT token
  }));
};

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  
  if (msg.type === 'auth_success') {
    console.log('Authenticated:', msg.permissions);
    // Now receive machine/workflow events
  } else if (msg.type === 'auth_failed') {
    console.error('Auth failed:', msg.reason);
  }
};
```


### Machine Token Management (Admin only)

```bash
# List all machine tokens
curl http://localhost:8080/api/v1/machine-tokens \
  -H "Authorization: Bearer $ADMIN_JWT"

# Delete a machine token
curl -X DELETE http://localhost:8080/api/v1/machine-tokens/<token-id> \
  -H "Authorization: Bearer $ADMIN_JWT"

# Update token metadata
curl -X PATCH http://localhost:8080/api/v1/machine-tokens/<token-id> \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "HMI Line 1 (Updated)",
    "metadata": {"location": "Building A"}
  }'
```


### User Management (Admin only)

```bash
# Create new user
curl -X POST http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "technician1",
    "password": "secure-password",
    "role": "technician"
  }'

# List all users
curl http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $ADMIN_JWT"

# Update user
curl -X PATCH http://localhost:8080/api/v1/users/<user-id> \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "password": "new-password",
    "role": "admin"
  }'

# Delete user
curl -X DELETE http://localhost:8080/api/v1/users/<user-id> \
  -H "Authorization: Bearer $ADMIN_JWT"
```


## Core Concepts

### Devices

Devices wrap Modbus connections and expose logical I/O (e.g. `TEST_OUTPUT`) mapped to registers / coils.

Example: create a Modbus test device (requires Admin JWT or authentication):

```bash
curl -X POST http://localhost:8080/api/v1/devices \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "instance_id": "test-modbus-sim",
    "composition": {
      "profile": "Generic/ModbusTCP",
      "connection": {
        "ip_address": "127.0.0.1",
        "port": 5502,
        "unit_id": 1
      }
    },
    "io_mapping": {
      "TEST_OUTPUT": "Coil_0",
      "TEST_INPUT": "Input_0"
    }
  }'
```

Read logical I/O (Machine Token or higher):

```bash
curl -X POST http://localhost:8080/api/v1/devices/<device-runtime-id>/read \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"register":"TEST_INPUT"}'
```

Write logical I/O (Technician JWT or Admin JWT only):

```bash
curl -X POST http://localhost:8080/api/v1/devices/<device-runtime-id>/write \
  -H "Authorization: Bearer $TECHNICIAN_JWT" \
  -H "Content-Type: application/json" \
  -d '{"register":"TEST_OUTPUT","value":true}'
```


### Workflows

Workflows are JSON objects stored in PostgreSQL and executed by the workflow engine.

Minimal example: toggle `TEST_OUTPUT` once on `test-modbus-sim`:

```json
{
  "workflow_name": "Toggle Output Once",
  "definition": {
    "id": "toggle-once",
    "name": "Toggle Output Once",
    "version": "1.0.0",
    "steps": [
      {
        "name": "Set HIGH",
        "type": "device",
        "device_id": "test-modbus-sim",
        "operation": "write_logical",
        "parameters": {
          "register": "TEST_OUTPUT",
          "value": true
        },
        "timeout": "1s"
      },
      {
        "name": "Wait",
        "type": "wait",
        "timeout": "1s"
      },
      {
        "name": "Set LOW",
        "type": "device",
        "device_id": "test-modbus-sim",
        "operation": "write_logical",
        "parameters": {
          "register": "TEST_OUTPUT",
          "value": false
        },
        "timeout": "1s"
      }
    ]
  },
  "active": true
}
```

Create (Admin JWT only):

```bash
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d @toggle-once.json
```

Execute (Machine Token or higher):

```bash
curl -X POST http://localhost:8080/api/v1/workflows/<workflow-id>/execute \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}'
```

Check execution (Machine Token or higher):

```bash
curl http://localhost:8080/api/v1/executions/<execution-id> \
  -H "Authorization: Bearer $TOKEN"

curl http://localhost:8080/api/v1/executions/<execution-id>/steps \
  -H "Authorization: Bearer $TOKEN"
```

Cancel (for looping workflows, Machine Token or higher):

```bash
curl -X POST http://localhost:8080/api/v1/executions/<execution-id>/cancel \
  -H "Authorization: Bearer $TOKEN"
```


#### Looping workflows

To run steps continuously (or N times), use `loop` in the workflow definition:

```json
"loop": {
  "enabled": true,
  "max_count": 0,
  "on_error": "stop"
}
```

- `max_count = 0` → infinite
- `on_error = "stop"` or `"continue"`


### Machine Controller

The machine controller uses three workflows:

- Stop workflow: controlled stop
- Home workflow: move to reference position
- Production workflow: main automatic loop

Configure workflows (Admin JWT only):

```bash
curl -X POST http://localhost:8080/api/v1/machine/configure \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "stop_workflow_id": "uuid-stop",
    "home_workflow_id": "uuid-home",
    "production_workflow_id": "uuid-production"
  }'
```

Commands (Machine Token or higher):

```bash
# Home sequence
curl -X POST http://localhost:8080/api/v1/machine/command \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":"home"}'

# Start production (looping workflow)
curl -X POST http://localhost:8080/api/v1/machine/command \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":"start"}'

# Controlled stop
curl -X POST http://localhost:8080/api/v1/machine/command \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":"stop"}'

# Reset from error/emergency
curl -X POST http://localhost:8080/api/v1/machine/command \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":"reset"}'
```

Machine status (Machine Token or higher):

```bash
curl http://localhost:8080/api/v1/machine/status \
  -H "Authorization: Bearer $TOKEN"
```


### Module / Device Descriptors

Device modules are described in:

- `device-descriptors/vendors/<vendor>/index.yaml`
- `device-descriptors/vendors/<vendor>/modules/<module>.json`

REST endpoints (Machine Token or higher):

```bash
# List vendors and modules
curl http://localhost:8080/api/v1/modules \
  -H "Authorization: Bearer $TOKEN"

# Get vendor index
curl http://localhost:8080/api/v1/modules/<vendor> \
  -H "Authorization: Bearer $TOKEN"

# Get specific module JSON
curl http://localhost:8080/api/v1/modules/<vendor>/<model> \
  -H "Authorization: Bearer $TOKEN"
```


## Project Structure

```
cmd/server/         Entry point
internal/api/rest   REST handlers with auth middleware
internal/api/grpc   gRPC services
internal/api/websocket  WebSocket hub with authentication
internal/auth       Authentication & authorization (NEW)
  ├── jwt.go        JWT token generation & validation
  ├── machine_token.go  Machine token management
  ├── middleware.go Permission-based access control
  ├── password.go   Argon2id password hashing
  └── service.go    Auth business logic
internal/config     Configuration
internal/devices    Device manager and compositions
internal/machine    Machine state controller
internal/modbus     Modbus TCP client & device wrapper
internal/storage    PostgreSQL client & repositories
  └── auth.go       User, token, and auth event storage (NEW)
internal/system     Lifecycle manager (startup, shutdown, servers)
internal/types      Shared type definitions
internal/workflow   Workflow engine, executor, streaming, definitions
device-descriptors/ Device profile and module definitions
migrations/         Database migrations
  ├── 001_initial_schema.sql
  ├── 002_workflow_engine.sql
  └── 003_auth_system.sql (NEW)
configs/            Configuration files
bin/                Built binaries (ignored in VCS)
```


## Security Best Practices

1. **Always set JWT_SECRET in production** - Never use the default secret
2. **Change default admin password immediately** after first login
3. **Use HTTPS in production** - All tokens must be transmitted securely
4. **Rotate machine tokens periodically** - Delete old tokens when decommissioning HMIs
5. **Monitor auth_events table** - Check for suspicious login attempts
6. **Use strong passwords** - Minimum 8 characters enforced by API
7. **Limit machine token permissions** - Use principle of least privilege

## Development

### Build \& Run

```bash
make build
JWT_SECRET="dev-secret" ./bin/openmachinecore
```


### Tests

```bash
go test ./...
```


### CLI Commands

```bash
# Generate machine token
./bin/openmachinecore --generate-machine-token "HMI Line 1"

# Create admin user
./bin/openmachinecore --create-admin

# Run with custom config
./bin/openmachinecore --config=/path/to/config.yaml
```


## License

Apache License 2.0 - See LICENSE for more information

---

This README focuses on practical information needed to build, configure, secure, and use devices, workflows, authentication, and the machine controller. See API_Documentation.md for further information.

---
