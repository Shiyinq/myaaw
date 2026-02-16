# 🔗 Myaaw (My AI Agent Well)

<div align="center">
  <img src="docs/images/mascot/myaaw.png" alt="Myaaw Mascot" width="200" />
</div>

Myaaw is a cat who becomes your personal AI assistant.


## Providers
- [x] Ollama
- [x] OpenAI
- [x] Gemini
- [x] Groq
- [x] Mistral
- [ ] Anthropic

## Features
- [x] Text Input
- [x] Voice Input
- [x] Image Input
- [x] Basic Response
- [x] Stream Response
- [x] Predefine Prompts
- [x] Tools
- [ ] Memory


# Table of Contents
- [🔗 Myaaw](#-myaaw)
  - [Providers](#providers)
  - [Features](#features)
- [Table of Contents](#table-of-contents)
  - [🚀 Quick Start (Non-Technical)](#-quick-start-non-technical)
  - [🛠 Development](#-development)
    - [Prerequisites](#prerequisites)
    - [Getting Started](#getting-started)
  - [🐳 Docker Usage](#-docker-usage)
  - [📡 Telegram/Discord Setup](#-telegramdiscord-setup)


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

Before development process, ensure you have the following installed:


#### Redis

It is recommended to use Docker to install Redis. If you haven’t installed Docker yet, you can follow the official Docker installation guide.

To install Redis using Docker, run the following command:

```
docker pull redis
```
Then, start Redis with:

```
docker run --name redis-server -d -p 6379:6379 redis
```
Ensure Redis is running by checking with:

```
docker ps
```

#### MongoDB

Similarly, use Docker to install MongoDB. Run the following command to pull the MongoDB image:
```
docker pull mongo
```
Start MongoDB with:

```
docker run --name mongodb-server -p 27017:27017 -v mongodb-data:/data/db -d mongo
```

Ensure MongoDB is running by checking with:
```
docker ps
```

#### RabbitMQ

Run the following command to pull the RabbitMQ image:

```bash
docker pull rabbitmq:4.0.2-management
```

Once the image is downloaded, start RabbitMQ with the following command:

```
docker run -d --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:4.0.2-management
```

- **Port 5672**: Used for RabbitMQ communication (AMQP).
- **Port 15672**: Used for accessing the RabbitMQ Management UI.

Ensure RabbitMQ is running by checking with:

```
docker ps
```

You can access the RabbitMQ Management UI in your browser at:

```
http://localhost:15672
```

**Username:** `guest`  
**Password:** `guest`

### Getting Started

1. **Clone the Repository**
   ```bash
   git clone https://github.com/Shiyinq/myaaw.git
   cd myaaw
   ```

2. **Onboard (Dev Mode)**
   Since you have the source code, you can run the onboarding flow directly:
   ```bash
   go run ./cmd/myaaw onboard
   ```

3. **Run with Live Reload**
   We recommend using `air` for development:
   ```bash
   air
   ```

4. **Manage via CLI**
   ```bash
   go run ./cmd/myaaw status
   go run ./cmd/myaaw docker setup
   ```


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

## 🐳 Docker Usage

Myaaw uses Docker primarily for infrastructure (MongoDB, Redis, RabbitMQ).

- **Start Infrastructure**: `myaaw docker setup`
- **Stop Infrastructure**: `myaaw docker stop`
- **View Logs**: `myaaw docker logs`

For full containerized deployment (Production):
```bash
docker compose up --build -d
```

## 📡 Telegram/Discord Setup

### Setting the Webhook
Use the built-in CLI to manage your webhooks easily:

```bash
myaaw webhook set
myaaw webhook info
```


#### Bot Token
You can obtain a bot token from [BotFather](https://t.me/BotFather) and add bot token to `.env` file.

#### Development
If you are running the backend locally, you need to use a tool like [ngrok](https://ngrok.com) to expose your local server to the internet. 

##### Install ngrok

Visit the ngrok [Getting Started Documentation](https://ngrok.com/docs/getting-started/) for installation instructions.

##### Obtain Your ngrok Auth Token

Go to the ngrok [Dashboard](https://dashboard.ngrok.com/get-started/your-authtoken) to find your auth token.

Open the `.env` file and edit it as follows:

```
# NGROK
NGROK_ACTIVE=true
NGROK_AUTHTOKEN=your-token-here
```

Restart the backend using the `air` command, and the Telegram bot will activate automatically.

If you do not want to use ngrok, set `NGROK_ACTIVE` to `false`.

#### Production

If your server has a public IP or domain, you can directly set the webhook to Telegram:

```
https://yourdomain.com
```

#### Use CLI
You can use the CLI app to manage the Telegram webhook.
```sh
go run cmd/telegram/telegram.go
```
This command will display a CLI menu like this.
````
Welcome to Telegram Webhook CLI
===============================
Choose an option:
1. Set Webhook
2. Get Webhook Info
3. Delete Webhook
4. Exit CLI

Enter choice: 
````

Or you can manually set it up by making a request to the Telegram API.

#### Manual Setup
##### Set Webhook
To set the webhook with Telegram, use the following API endpoint:

```
https://api.telegram.org/bot{my_bot_token}/setWebhook?url={your_domain_or_your_ip_public_or_ngrok_url}/webhook/telegram
```

Example:

```
https://api.telegram.org/bot123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11/setWebhook?url=https://yourdomain.com/webhook/telegram
```
##### Get Webhook Info
You can retrieve the current webhook info using:

```
https://api.telegram.org/bot{my_bot_token}/getWebhookInfo
```

##### Delete Webhook
To remove the webhook, make a call to the `setWebhook` method with an empty `url` parameter:

```
https://api.telegram.org/bot{my_bot_token}/setWebhook?url=
```