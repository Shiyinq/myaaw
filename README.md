# 🔗 Myaaw (My AI Agent Well)

<div align="center">
  <img src="docs/images/mascot/myaaw.png" alt="Myaaw Mascot" width="200" />

  [![Go Report Card](https://goreportcard.com/badge/github.com/Shiyinq/myaaw)](https://goreportcard.com/report/github.com/Shiyinq/myaaw)
  [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
  [![Go Version](https://img.shields.io/github/go-mod/go-version/Shiyinq/myaaw)](https://github.com/Shiyinq/myaaw)
  [![GitHub stars](https://img.shields.io/github/stars/Shiyinq/myaaw?style=social)](https://github.com/Shiyinq/myaaw/stargazers)

  **Myaaw is a cat who becomes your personal AI assistant.**
</div>

---



## ✨ Key Features
- [x] **Multimodal Input**: Supports Text, Voice (Transcribed), and Image input.
- [x] **Smart Responses**: Supports both Basic and Streamed responses for real-time interaction.
- [x] **Conversation Memory**: Remembers your chat history for natural flow.
- [x] **Agent Skills**: Dynamically extensible capabilities via `SKILL.md` definitions.
- [x] **Powerful Tools**: Native integration with Filesystem, Bash, and Python execution.
- [x] **Heartbeat (Autonomous Check)**: Scheduled background checks for health and tasks via `HEARTBEAT.md`.

## 🤖 Supported Providers

Myaaw currently focuses on delivering the best experience with **Google Gemini API**.


## 📡 Supported Channels

Myaaw can be integrated with various platforms:

- 📨 **Telegram**: Integration via Bot API (Polling or Webhook).
- 👾 **Discord**: Integration via Discord Bot.
- 💻 **Terminal**: Interactive TUI chat and CLI management.


# Table of Contents
- [✨ Key Features](#-key-features)
- [🤖 Supported Providers](#-supported-providers)
- [📡 Supported Channels](#-supported-channels)
- [🚀 Quick Start (Non-Technical)](#-quick-start-non-technical)
- [🛠 Development](#-development)
  - [Prerequisites](#prerequisites)
  - [Getting Started](#getting-started)
  - [Start Development](#start-development)
  - [Generate Swagger Documentation](#generate-swagger-documentation)
  - [Build from Source](#build-from-source-optional)
- [💻 Myaaw CLI Reference](#-myaaw-cli-reference)


## 🚀 Quick Start (Non-Technical)

If you just want to use Myaaw without worrying about code:

1.  **Download Myaaw**: Go to [Releases](https://github.com/Shiyinq/myaaw/releases) and download the binary for your OS (e.g., `myaaw-windows-amd64.exe` or `myaaw-macos-arm64`).
2.  **Onboard**: Open your terminal/command prompt in the folder where you downloaded it and run:
    ```bash
    ./myaaw onboard
    ```
    *This will guide you through setting up your API keys and configuration.*
3.  **Start Databases**: If you have Docker installed, run:
    ```bash
    myaaw docker setup
    ```
4.  **Launch**:
    ```bash
    myaaw gateway start
    ```
5.  **Check Status**:
    ```bash
    myaaw status
    ```

## 🛠 Development


### Prerequisites

Before you start, ensure you have:
- [Go](https://go.dev/) **1.24.2** or later installed.
- [Docker](https://www.docker.com/) installed.

#### Infrastructure (Docker Compose)

Myaaw requires Redis, MongoDB, and RabbitMQ. You can start all of them with a single command:

```bash
docker compose up -d
```

**Alternative (CLI)**:
If you prefer using the built-in CLI for setup and monitoring:
```bash
go run ./cmd/myaaw docker setup
go run ./cmd/myaaw status
```

| Service | Port | Description |
| :--- | :--- | :--- |
| **Redis** | `6379` | Message caching |
| **MongoDB** | `27018` | Data storage |
| **RabbitMQ** | `5672`, `15672` | Task queue & Management UI |

> [!NOTE]
> RabbitMQ Management UI is available at `http://localhost:15672` (guest/guest).

### Getting Started

1. **Clone the Repository**
   ```bash
   git clone https://github.com/Shiyinq/myaaw.git
   cd myaaw
   ```

2. **Install Dependencies**
   Like `npm install` for `package.json`, you should download the Go modules:
   ```bash
   go mod tidy
   ```

3. **Onboard (Dev Mode)**
   Since you have the source code, you can run the onboarding flow directly:
   ```bash
   go run ./cmd/myaaw onboard
   ```

### Start Development

Once everything is set up, you can start the gateway using **one** of the following methods:

#### Option A: Direct Execution (Standard)
```bash
go run ./cmd/myaaw gateway start
```

#### Option B: Live Reload (Recommended for Dev)
If you want the server to restart automatically when you change the code, use [Air](https://github.com/air-verse/air). 

1. Install it globally (first time only):
   ```bash
   go install github.com/air-verse/air@latest
   ```
2. Run it in the root directory:
   ```bash
   air
   ```

---

**Monitor Status & Logs**:
In a separate terminal, you can monitor the gateway:
```bash
go run ./cmd/myaaw status
go run ./cmd/myaaw logs
```

> [!NOTE]
> These commands only track services started via **Option A**. 
> If you are using **Option B (Air)**, the status and logs will appear directly in the same terminal where the `air` command is running.

---

### Generate Swagger Documentation
1. **Install Swagger for API Documentation**

   If you don't have `swag` installed on your machine, install it first:
   ```sh
   go install github.com/swaggo/swag/cmd/swag@latest
   ```

2. **Generate or Update Documentation**
    ```sh
    swag init -g ./cmd/server/main.go --parseDependency --parseInternal --output docs/swagger
    ```
    Or you can use the `swag.sh` script:

    For the first time, before running the script, execute:
    ```
    chmod +x swag.sh
    ```
    Then, run:
    ```
    ./swag.sh
    ```

3. **Swagger Documentation**

    http://localhost:8080/docs/index.html
### Build from Source (Optional)

If you want to build the Myaaw binaries yourself (e.g., for distribution or custom versions), you can use the provided `Makefile`:

1.  **Build for current platform**:
    ```bash
    make build
    ```
2.  **Cross-compile for all platforms**:
    ```bash
    make build-all
    ```
    *This will generate binaries for macOS (Intel/M1), Linux, and Windows in the `bin/` directory.*


## 💻 Myaaw CLI Reference

The `myaaw` CLI is your control center for managing the AI assistant, infrastructure, and services. You can run these commands via `go run ./cmd/myaaw` (Development) or simply `myaaw` (if installed globally).

### 🛠️ Getting Started
| Command | Description |
| :--- | :--- |
| `onboard` | Interactive setup for first-time users (Keys, Channels, Docker). |

### 🐱 Core Interaction
| Command | Description |
| :--- | :--- |
| `chat` | Start an interactive TUI chat session with Myaaw. Use `-m "text"` for one-shot. |
| `status` | Check the health of MongoDB, Redis, RabbitMQ, and channel configs. |
| `logs` | Interactive TUI to select and stream logs from `~/.myaaw/logs/`. |

### 🚀 Service Management
Myaaw runs as two components: a **Server** (Gateway/API) and a **Consumer** (Task processing).

| Command | Subcommands | Description |
| :--- | :--- | :--- |
| **`gateway`** | `start`, `stop`, `restart`, `status` | Manage **both** Server & Consumer as background daemons. |
| **`server`** | `run`, `start`, `stop`, `status` | Manage the Fiber Web Server specifically. |
| **`consumer`** | `run`, `start`, `stop`, `status` | Manage the RabbitMQ Message Consumer specifically. |

> [!TIP]
> Use `run` for foreground execution (useful for debugging) and `start` for background execution.

### 🐳 Infrastructure & Webhooks
| Command | Subcommands | Description |
| :--- | :--- | :--- |
| **`docker`** | `setup`, `stop`, `logs` | Helper to manage Redis, Mongo, and RabbitMQ via Docker Compose. |
| **`webhook`** | `set`, `info`, `delete` | Manage Telegram bot webhook configuration easily. |

### ⏰ Cron Scheduler & Tasks
Myaaw includes a built-in scheduler to automate tasks (like sending recurring messages or reminders).

| Command | Subcommands | Description |
| :--- | :--- | :--- |
| **`cron`** | `list` | View all scheduled jobs in a neat table. |
|  | `add` | Create a new job (supports cron expression, interval, or one-time `at`). |
|  | `remove` | Delete a scheduled job by ID. |
|  | `run` | Manually trigger a specific job immediately. |
|  | `history` | View execution logs (success/fail/skipped) for a job or globally. |

**Examples:**
```bash
# Every morning at 7 AM
myaaw cron add --name "Morning Greeting" --cron "0 7 * * *" --message "Selamat Pagi!" --channel telegram --to 123456789

# Every 2 hours
myaaw cron add --name "Hydration Check" --every "2h" --message "Drink water!" --channel telegram --to 123456789

# One-time reminder in 30 minutes
myaaw cron add --name "Meeting Remind" --at "30m" --message "Meeting in 30 mins" --channel telegram --to 123456789
```

### ⚙️ System & Config
| Command | Subcommands | Description |
| :--- | :--- | :--- |
| **`config`** | `check`, `dump` | Validate your `.env` or print current configuration (masked). |
| **`completion`** | `bash`, `zsh`, `fish`, `ps` | Generate autocompletion scripts for your shell. |
| **`help`** | `[command]` | Display help information for Myaaw or any specific command. |
| **`update`** | - | Automatically check for and install the latest version. |
| **`version`** | - | Show build version, commit hash, and system info. |

