# OpenMachineCore

[![Release Build](https://github.com/KevinKickass/OpenMachineCore/actions/workflows/release.yml/badge.svg)](https://github.com/KevinKickass/OpenMachineCore/actions)
[![Latest Release](https://img.shields.io/github/v/release/KevinKickass/OpenMachineCore?sort=semver)](https://github.com/KevinKickass/OpenMachineCore/releases)

OpenMachineCore (Backend) is a Go-based automation platform for industrial machine control. It combines a Modbus TCP - EtherCat and more planned - device layer, a workflow engine, and a machine state controller with REST, Websocket and gRPC APIs. 

OpenMachineCore HMI is coming soon for displaying machine state and give the user a place for basic controling of the machine.

OpenMachineCore Configurator is coming soon too to help set up workflows, devices and so on

You can also use your own HMI and use the APIs to control/display the machine states and devices

## Still work in progress - Not production ready

## Features

- Workflow engine with:
  - JSON-defined workflows
  - Step types: `device`, `workflow` (sub-workflow), `wait`
  - Optional loop configuration (continuous or fixed count)
- Machine controller with high-level modes:
  - Stop (controlled stop)
  - Home (move to reference position)
  - Start (automatic / production loop)
- Modbus TCP device management with logical I/O mapping
- REST API for devices, workflows, machine control, and modules
- gRPC streaming for workflow execution events
- Websocket streaming for status, I/O and workflow updates
- PostgreSQL-backed storage for devices, workflows, and executions

## Architecture Overview

```

cmd/server/main.go
│
▼
LifecycleManager (internal/system)
├── DeviceManager      (internal/devices)
├── Workflow Engine    (internal/workflow/engine, executor, streaming, definition)
└── Machine Controller (internal/machine)

APIs:
- REST (internal/api/rest)
- gRPC (internal/api/grpc, api/proto)
- Websocket Hub (internal/api/websocket)

Storage:
- PostgreSQL (internal/storage)

Modbus:
- internal/modbus

```

## Getting Started

### Prerequisites

- Go 1.25 or higher
- PostgreSQL 13 or higher
- Optional: Modbus simulator for testing

### Clone and Build

```

git clone https://github.com/KevinKickass/OpenMachineCore.git
cd OpenMachineCore

go mod download
make build

```

The binary is built as:

```

./bin/openmachinecore

```

### Configuration

Create `configs/config.yaml` (adapt to your environment):

```

database:
host: localhost
port: 5432
user: openmachine
password: your_password
dbname: openmachinecore
sslmode: disable

server:
http_port: 8080
grpc_port: 50051

modbus:
default_timeout: 2000000000         \# 2s (nanoseconds)
default_poll_interval: 1000000000   \# 1s (nanoseconds)

device_profiles:
search_paths:
- "device-descriptors/vendors"
- "/etc/openmachinecore/profiles"

```

### Database

Create the database and apply migrations:

```

createdb openmachinecore
psql -d openmachinecore -f migrations/001_initial_schema.sql
psql -d openmachinecore -f migrations/002_workflow_engine.sql

```

(Adjust migration filenames to match your repo.)

### Run

```

./bin/openmachinecore

```

By default:

- REST API: `http://localhost:8080/api/v1`
- gRPC: `localhost:50051`

## Core Concepts

### Devices

Devices wrap Modbus connections and expose logical I/O (e.g. `TEST_OUTPUT`) mapped to registers / coils.

Example: create a Modbus test device:

```

curl -X POST http://localhost:8080/api/v1/devices \
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

Read logical I/O:

```

curl -X POST http://localhost:8080/api/v1/devices/<device-runtime-id>/read \
-H "Content-Type: application/json" \
-d '{"register":"TEST_INPUT"}'

```

Write logical I/O:

```

curl -X POST http://localhost:8080/api/v1/devices/<device-runtime-id>/write \
-H "Content-Type: application/json" \
-d '{"register":"TEST_OUTPUT","value":true}'

```

### Workflows

Workflows are JSON objects stored in PostgreSQL and executed by the workflow engine.

Minimal example: toggle `TEST_OUTPUT` once on `test-modbus-sim`:

```

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

Create:

```

curl -X POST http://localhost:8080/api/v1/workflows \
-H "Content-Type: application/json" \
-d @toggle-once.json

```

Execute:

```

curl -X POST http://localhost:8080/api/v1/workflows/<workflow-id>/execute \
-H "Content-Type: application/json" \
-d '{}'

```

Check execution:

```

curl http://localhost:8080/api/v1/executions/<execution-id>
curl http://localhost:8080/api/v1/executions/<execution-id>/steps

```

Cancel (for looping workflows):

```

curl -X POST http://localhost:8080/api/v1/executions/<execution-id>/cancel

```

#### Looping workflows

To run steps continuously (or N times), use `loop` in the workflow definition:

```

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

Configure workflows:

```

curl -X POST http://localhost:8080/api/v1/machine/configure \
-H "Content-Type: application/json" \
-d '{
"stop_workflow_id": "uuid-stop",
"home_workflow_id": "uuid-home",
"production_workflow_id": "uuid-production"
}'

```

Commands:

```


# Home sequence

curl -X POST http://localhost:8080/api/v1/machine/command \
-H "Content-Type: application/json" \
-d '{"command":"home"}'

# Start production (looping workflow)

curl -X POST http://localhost:8080/api/v1/machine/command \
-H "Content-Type: application/json" \
-d '{"command":"start"}'

# Controlled stop

curl -X POST http://localhost:8080/api/v1/machine/command \
-H "Content-Type: application/json" \
-d '{"command":"stop"}'

# Reset from error/emergency

curl -X POST http://localhost:8080/api/v1/machine/command \
-H "Content-Type: application/json" \
-d '{"command":"reset"}'

```

Machine status:

```

curl http://localhost:8080/api/v1/machine/status

```

### Module / Device Descriptors

Device modules are described in:

- `device-descriptors/vendors/<vendor>/index.yaml`
- `device-descriptors/vendors/<vendor>/modules/<module>.json`

REST endpoints:

```


# List vendors and modules

curl http://localhost:8080/api/v1/modules

# Get vendor index

curl http://localhost:8080/api/v1/modules/<vendor>

# Get specific module JSON

curl http://localhost:8080/api/v1/modules/<vendor>/<model>

```

## Project Structure

```

cmd/server/         Entry point
internal/api/rest   REST handlers
internal/api/grpc   gRPC services
internal/config     Configuration
internal/devices    Device manager and compositions
internal/machine    Machine state controller
internal/modbus     Modbus TCP client \& device wrapper
internal/storage    PostgreSQL client \& repositories
internal/system     Lifecycle manager (startup, shutdown, servers)
internal/types      Shared type definitions
internal/workflow   Workflow engine, executor, streaming, definitions
device-descriptors/ Device profile and module definitions
migrations/         Database migrations
configs/            Configuration files
bin/                Built binaries (ignored in VCS)

```

## Development

### Build & Run

```

make build
./bin/openmachinecore

```

### Tests

```

go test ./...

```

## License

Apache License 2.0 - See LICENSE for more information

---

This README intentionally focuses on the minimal but practical information you need right now: how to build, configure, and use devices, workflows, and the machine controller. See API_Documentation.md for further information.

---
