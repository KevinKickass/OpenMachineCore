<img src="https://r2cdn.perplexity.ai/pplx-full-logo-primary-dark%402x.png" style="height:64px;margin-right:32px"/>

# OpenMachineCore API Documentation

## Table of Contents

1. [Quick Start](#quick-start)
2. [Device Configuration](#device-configuration)
3. [Workflow Management](#workflow-management)
4. [Machine Control](#machine-control)
5. [Workflow Examples](#workflow-examples)

***

## Quick Start

**Base URL:** `http://localhost:8080/api/v1`

**Prerequisites:**

- OpenMachineCore running
- PostgreSQL database initialized
- At least one device configured

***

## 1. Device Configuration

### 1.1 Create a Device

Devices are created from **Device Compositions** that reference a device profile and map logical I/O names.[^1]

**Endpoint:** `POST /devices`

**Request Body:**

```json
{
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
    "TEST_INPUT": "Input_0",
    "TEMPERATURE": "HoldingRegister_0"
  }
}
```

**Response:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "runtime_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "name": "test-modbus-sim",
  "message": "Device created and persisted successfully"
}
```

**Curl Example:**

```bash
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


### 1.2 List All Devices

**Endpoint:** `GET /devices`

**Response:**

```json
{
  "devices": [
    {
      "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "name": "test-modbus-sim",
      "profile": "ModbusTCP",
      "connected": true
    }
  ],
  "count": 1
}
```


### 1.3 Read Device I/O

**Endpoint:** `POST /devices/:id/read`

**Request Body:**

```json
{
  "register": "TEST_INPUT"
}
```

**Response:**

```json
{
  "register": "TEST_INPUT",
  "value": true,
  "timestamp": 1734180000
}
```


### 1.4 Write Device I/O

**Endpoint:** `POST /devices/:id/write`

**Request Body:**

```json
{
  "register": "TEST_OUTPUT",
  "value": true
}
```

**Response:**

```json
{
  "message": "Register written successfully",
  "register": "TEST_OUTPUT",
  "value": true
}
```


***

## 2. Workflow Management

### 2.1 Create a Workflow

Workflows define sequences of steps that can interact with devices, wait, or call other workflows.[^2][^3]

**Endpoint:** `POST /workflows`

**Basic Workflow Structure:**

```json
{
  "workflow_name": "My Workflow",
  "definition": {
    "id": "my-workflow",
    "name": "My Workflow",
    "version": "1.0.0",
    "loop": {
      "enabled": false,
      "max_count": 0,
      "on_error": "stop"
    },
    "steps": [
      {
        "name": "Step Name",
        "type": "device|workflow|wait",
        "timeout": "2s"
      }
    ]
  },
  "active": true
}
```

**Step Types:**

#### Device Step

```json
{
  "name": "Turn Output On",
  "type": "device",
  "device_id": "test-modbus-sim",
  "operation": "write_logical",
  "parameters": {
    "register": "TEST_OUTPUT",
    "value": true
  },
  "timeout": "1s",
  "on_error": "fail"
}
```

**Operations:**

- `write_logical` - Write to logical I/O name
- `read_logical` - Read from logical I/O name
- `write_register` - Write to register by name
- `read_register` - Read from register by name


#### Wait Step

```json
{
  "name": "Wait 2 seconds",
  "type": "wait",
  "timeout": "2s"
}
```


#### Sub-Workflow Step

```json
{
  "name": "Safety Check",
  "type": "workflow",
  "workflow_id": "safety-check-workflow-uuid",
  "timeout": "5s"
}
```


### 2.2 Execute a Workflow

**Endpoint:** `POST /workflows/:id/execute`

**Request Body (optional):**

```json
{
  "input_data": {
    "parameter1": "value1"
  }
}
```

**Response:**

```json
{
  "execution_id": "abc-123-def-456",
  "message": "Workflow execution started"
}
```


### 2.3 Check Execution Status

**Endpoint:** `GET /executions/:id`

**Response:**

```json
{
  "id": "abc-123-def-456",
  "workflow_id": "my-workflow-uuid",
  "status": "running",
  "current_step": 2,
  "started_at": "2025-12-14T12:00:00Z",
  "completed_at": null
}
```

**Status Values:** `pending`, `running`, `success`, `failed`, `cancelled`

### 2.4 Cancel Execution

**Endpoint:** `POST /executions/:id/cancel`

**Response:**

```json
{
  "message": "Execution cancelled"
}
```


### 2.5 List All Workflows

**Endpoint:** `GET /workflows`

**Response:**

```json
{
  "workflows": [
    {
      "id": "workflow-uuid",
      "workflow_name": "My Workflow",
      "active": true,
      "created_at": "2025-12-14T10:00:00Z"
    }
  ],
  "count": 1
}
```


***

## 3. Machine Control

The Machine Controller manages high-level machine states using workflows.[^3]

### 3.1 Configure Machine Workflows

**Endpoint:** `POST /machine/configure`

**Request Body:**

```json
{
  "stop_workflow_id": "uuid-of-stop-workflow",
  "home_workflow_id": "uuid-of-home-workflow",
  "production_workflow_id": "uuid-of-production-workflow"
}
```

**Response:**

```json
{
  "message": "Machine workflows configured"
}
```


### 3.2 Get Machine Status

**Endpoint:** `GET /machine/status`

**Response:**

```json
{
  "state": "stopped",
  "current_workflow": "",
  "execution_id": "",
  "error_message": "",
  "production_cycles": 0,
  "last_state_change": "2025-12-14T12:00:00Z"
}
```

**Machine States:**

- `stopped` - Machine is stopped
- `homing` - Moving to home position
- `ready` - Ready to start production
- `running` - Production running
- `stopping` - Controlled stop in progress
- `error` - Error state, requires reset
- `emergency` - Emergency stop active


### 3.3 Send Machine Commands

**Endpoint:** `POST /machine/command`

**Request Body:**

```json
{
  "command": "home|start|stop|reset"
}
```

**Commands:**

#### Home Command

Executes the home workflow to move machine to reference position.

```bash
curl -X POST http://localhost:8080/api/v1/machine/command \
  -H "Content-Type: application/json" \
  -d '{"command": "home"}'
```

**State Transition:** `stopped` → `homing` → `ready`

#### Start Command

Starts production by executing the production workflow in loop mode.

```bash
curl -X POST http://localhost:8080/api/v1/machine/command \
  -H "Content-Type: application/json" \
  -d '{"command": "start"}'
```

**State Transition:** `ready` → `running`

#### Stop Command

Executes controlled stop workflow and halts production.

```bash
curl -X POST http://localhost:8080/api/v1/machine/command \
  -H "Content-Type: application/json" \
  -d '{"command": "stop"}'
```

**State Transition:** `running` → `stopping` → `stopped`

#### Reset Command

Resets error state after fixing the issue.

```bash
curl -X POST http://localhost:8080/api/v1/machine/command \
  -H "Content-Type: application/json" \
  -d '{"command": "reset"}'
```

**State Transition:** `error` → `stopped`

**Response:**

```json
{
  "message": "Command accepted",
  "command": "start"
}
```


***

## 4. Workflow Examples

### 4.1 Simple Toggle Workflow

Toggles an output once:

```json
{
  "workflow_name": "Toggle Output Once",
  "definition": {
    "id": "toggle-once",
    "name": "Toggle Output",
    "version": "1.0.0",
    "steps": [
      {
        "name": "Turn On",
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
        "timeout": "2s"
      },
      {
        "name": "Turn Off",
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


### 4.2 Continuous Loop Workflow

Toggles output continuously until stopped:

```json
{
  "workflow_name": "Toggle Continuous",
  "definition": {
    "id": "toggle-continuous",
    "name": "Continuous Toggle",
    "version": "1.0.0",
    "loop": {
      "enabled": true,
      "max_count": 0,
      "on_error": "stop"
    },
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
        "timeout": "500ms"
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
        "timeout": "500ms"
      },
      {
        "name": "Wait",
        "type": "wait",
        "timeout": "1s"
      }
    ]
  },
  "active": true
}
```


### 4.3 Home Workflow (for Machine)

```json
{
  "workflow_name": "Machine Home",
  "definition": {
    "id": "machine-home",
    "name": "Move to Home Position",
    "version": "1.0.0",
    "steps": [
      {
        "name": "Enable Drive",
        "type": "device",
        "device_id": "drive-controller",
        "operation": "write_logical",
        "parameters": {
          "register": "DRIVE_ENABLE",
          "value": true
        },
        "timeout": "1s"
      },
      {
        "name": "Move to Home",
        "type": "device",
        "device_id": "drive-controller",
        "operation": "write_logical",
        "parameters": {
          "register": "HOME_COMMAND",
          "value": true
        },
        "timeout": "10s"
      },
      {
        "name": "Wait for Home Position",
        "type": "wait",
        "timeout": "5s"
      }
    ]
  },
  "active": false
}
```


### 4.4 Production Workflow with Sub-Workflow

```json
{
  "workflow_name": "Production Main",
  "definition": {
    "id": "production-main",
    "name": "Main Production Cycle",
    "version": "1.0.0",
    "loop": {
      "enabled": true,
      "max_count": 0,
      "on_error": "stop"
    },
    "steps": [
      {
        "name": "Safety Check",
        "type": "workflow",
        "workflow_id": "safety-check-workflow-uuid",
        "timeout": "2s"
      },
      {
        "name": "Pick Item",
        "type": "device",
        "device_id": "gripper",
        "operation": "write_logical",
        "parameters": {
          "register": "GRIPPER_CLOSE",
          "value": true
        },
        "timeout": "2s"
      },
      {
        "name": "Wait",
        "type": "wait",
        "timeout": "500ms"
      },
      {
        "name": "Place Item",
        "type": "device",
        "device_id": "gripper",
        "operation": "write_logical",
        "parameters": {
          "register": "GRIPPER_CLOSE",
          "value": false
        },
        "timeout": "2s"
      }
    ]
  },
  "active": false
}
```


***

## 5. Complete Setup Example

Here's a complete workflow from device setup to machine operation:

```bash
# Step 1: Create Device
DEVICE_RESP=$(curl -s -X POST http://localhost:8080/api/v1/devices \
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
      "DRIVE_ENABLE": "Coil_1",
      "HOME_COMMAND": "Coil_2"
    }
  }')

echo "Device created: $DEVICE_RESP"

# Step 2: Create Stop Workflow
STOP_WF=$(curl -s -X POST http://localhost:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "workflow_name": "Machine Stop",
    "definition": {
      "id": "machine-stop",
      "name": "Stop Machine",
      "version": "1.0.0",
      "steps": [
        {
          "name": "Disable Output",
          "type": "device",
          "device_id": "test-modbus-sim",
          "operation": "write_logical",
          "parameters": {"register": "TEST_OUTPUT", "value": false},
          "timeout": "1s"
        }
      ]
    }
  }' | jq -r '.workflow_id')

echo "Stop Workflow: $STOP_WF"

# Step 3: Create Home Workflow
HOME_WF=$(curl -s -X POST http://localhost:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "workflow_name": "Machine Home",
    "definition": {
      "id": "machine-home",
      "name": "Home Position",
      "version": "1.0.0",
      "steps": [
        {
          "name": "Enable Drive",
          "type": "device",
          "device_id": "test-modbus-sim",
          "operation": "write_logical",
          "parameters": {"register": "DRIVE_ENABLE", "value": true},
          "timeout": "1s"
        },
        {
          "name": "Home",
          "type": "device",
          "device_id": "test-modbus-sim",
          "operation": "write_logical",
          "parameters": {"register": "HOME_COMMAND", "value": true},
          "timeout": "2s"
        }
      ]
    }
  }' | jq -r '.workflow_id')

echo "Home Workflow: $HOME_WF"

# Step 4: Create Production Workflow
PROD_WF=$(curl -s -X POST http://localhost:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "workflow_name": "Production",
    "definition": {
      "id": "production",
      "name": "Production Cycle",
      "version": "1.0.0",
      "loop": {
        "enabled": true,
        "max_count": 0,
        "on_error": "stop"
      },
      "steps": [
        {
          "name": "Output HIGH",
          "type": "device",
          "device_id": "test-modbus-sim",
          "operation": "write_logical",
          "parameters": {"register": "TEST_OUTPUT", "value": true},
          "timeout": "500ms"
        },
        {
          "name": "Wait",
          "type": "wait",
          "timeout": "1s"
        },
        {
          "name": "Output LOW",
          "type": "device",
          "device_id": "test-modbus-sim",
          "operation": "write_logical",
          "parameters": {"register": "TEST_OUTPUT", "value": false},
          "timeout": "500ms"
        },
        {
          "name": "Wait",
          "type": "wait",
          "timeout": "1s"
        }
      ]
    }
  }' | jq -r '.workflow_id')

echo "Production Workflow: $PROD_WF"

# Step 5: Configure Machine
curl -X POST http://localhost:8080/api/v1/machine/configure \
  -H "Content-Type: application/json" \
  -d "{
    \"stop_workflow_id\": \"$STOP_WF\",
    \"home_workflow_id\": \"$HOME_WF\",
    \"production_workflow_id\": \"$PROD_WF\"
  }"

# Step 6: Operate Machine
curl -X POST http://localhost:8080/api/v1/machine/command -d '{"command":"home"}'
sleep 3

curl http://localhost:8080/api/v1/machine/status

curl -X POST http://localhost:8080/api/v1/machine/command -d '{"command":"start"}'
sleep 5

curl http://localhost:8080/api/v1/machine/status

curl -X POST http://localhost:8080/api/v1/machine/command -d '{"command":"stop"}'
```


***

## Error Handling

All endpoints return consistent error responses:

```json
{
  "error": "Error description"
}
```

**HTTP Status Codes:**

- `200` - Success
- `201` - Created
- `202` - Accepted (async operations)
- `400` - Bad Request
- `404` - Not Found
- `500` - Internal Server Error

***

**For more details, see the source code or contact the development team.**
<span style="display:none">[^10][^4][^5][^6][^7][^8][^9]</span>

<div align="center">⁂</div>

[^1]: https://gist.github.com/azagniotov/a4b16faf0febd12efbc6c3d7370383a6

[^2]: https://n8n.io/workflows/5171-learn-api-fundamentals-with-an-interactive-hands-on-tutorial-workflow/

[^3]: https://www.designgurus.io/answers/detail/how-to-design-api-workflow

[^4]: https://zuplo.com/learning-center/document-apis-with-markdown

[^5]: https://www.uni-hildesheim.de/gitlab/help/development/documentation/restful_api_styleguide.md

[^6]: https://docs.gitlab.com/development/documentation/restful_api_styleguide/

[^7]: https://docs.github.com/en/rest/markdown/markdown

[^8]: https://dev.to/lasserafn/11-api-documentation-best-practices-for-cicd-2024-3l13

[^9]: https://docs.typo3.org/m/typo3/docs-how-to-document/main/en-us/Advanced/Format.html

[^10]: https://qodex.ai/blog/11-best-practices-for-api-documentation

