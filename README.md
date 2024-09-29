# TEO
Telegram Ollama Integration

# Table of Contents
- [TEO](#teo)
- [Table of Contents](#table-of-contents)
  - [Install Ollama](#install-ollama)
  - [Development](#development)
    - [Running Local Server](#running-local-server)
    - [Generate Swagger Documentation](#generate-swagger-documentation)
  - [Deployment](#deployment)
  - [Telegram Bot Setup](#telegram-bot-setup)
    - [Setting the Webhook](#setting-the-webhook)
      - [Public IP or Domain](#public-ip-or-domain)
      - [Localhost Setup](#localhost-setup)
      - [Setting the Webhook](#setting-the-webhook-1)
    - [Optional](#optional)
      - [Get Webhook Info](#get-webhook-info)
      - [Delete Webhook](#delete-webhook)


## Install Ollama
Ensure you have Ollama installed by following the instructions in the official repository [Ollama GitHub](https://github.com/ollama/ollama?tab=readme-ov-file#ollama).

Additionally, you need to have at least one model installed. 

The default model specified in the `.env` file is `qwen2.5:1.5b-instruct`. 

To install it, run the following command in your terminal:
```
ollama pull qwen2.5:1.5b-instruct
```

## Development
### Running Local Server
1. **Clone the Repository**
   ```sh
   git clone https://github.com/Shiyinq/teo.git
   cd teo
   ```

2. **Install Go Modules**
   ```sh
   go mod tidy
   ```

3. **Create .env File**
   ```sh
   cp .env.example .env
   ```

4. **Install Air for Live Reloading**

   If you don't have `air` installed on your machine, install it first:
   ```sh
   go install github.com/air-verse/air@latest
   ```

5. **Run the Development Server**
   ```sh
   air
   ```

6. **Server**

    http://localhost:8080

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

## Deployment

Before you begin, ensure you have [Docker](https://docs.docker.com/engine/install/) installed.

**1. Clone the Repository**
```sh
git clone https://github.com/Shiyinq/noto.git
cd noto
```

**2. Create Environment Files**
```sh
cp .env.example .env
```

Open each `.env` file you have created and update the values as needed.

**3. Build and Run the Docker Containers**
```sh
docker compose up --build -d
```
Wait a few minutes for the setup to complete. You can then access:
- Backend at http://localhost:8080/docs

## Telegram Bot Setup

### Setting the Webhook
After running the backend, either using Docker or manually, you need to set up the webhook with the Telegram API.

#### Public IP or Domain

If your server has a public IP or domain, you can directly set the webhook to Telegram using:

```
https://yourdomain.com/webhook
```

#### Localhost Setup
If you are running the backend locally, you need to use a tool like [ngrok](https://ngrok.com) to expose your local server to the internet. You can run the following command:

```
ngrok http 8080
```

This will generate a public URL, and your webhook will look something like this:

```
https://9e64-114-124-182-000.ngrok-free.app/webhook
```

#### Setting the Webhook
To set the webhook with Telegram, use the following API endpoint:

```
https://api.telegram.org/bot{my_bot_token}/setWebhook?url={url_to_send_updates_to}
```

Example:

```
https://api.telegram.org/bot123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11/setWebhook?url=https://9e64-114-124-182-000.ngrok-free.app/webhook
```

### Optional
#### Get Webhook Info
You can retrieve the current webhook info using:

```
https://api.telegram.org/bot{my_bot_token}/getWebhookInfo
```

#### Delete Webhook
To remove the webhook, make a call to the `setWebhook` method with an empty `url` parameter:

```
https://api.telegram.org/bot{my_bot_token}/setWebhook?url=
```