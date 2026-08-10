# Provider Tool

The Provider tool allows the agent to manage LLM provider integrations within the Myaaw system. It supports listing configured providers, changing the default provider, fetching available models directly from a provider's API, and updating the active model for any provider.

## Function Name

`provider`

## Parameters

| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `action` | `string` | **Yes** | The action to perform: `list`, `set_default`, `set_model`, `fetch_models`. |
| `name` | `string` | Yes (for `set_default`, `set_model`, `fetch_models`) | The name of the provider. |
| `model` | `string` | Yes (for `set_model`) | The name of the LLM model to set as the active model for the provider. |

## Usage Examples

### List Providers

List all configured LLM providers and show which one is currently the default/active provider.

```json
{
  "action": "list"
}
```

### Set Default Provider

Change the active LLM provider that the system uses to respond to chats.

```json
{
  "action": "set_default",
  "name": "groq"
}
```

### Fetch Models

Query a specific provider's API to get a real-time list of all available models.

```json
{
  "action": "fetch_models",
  "name": "openai"
}
```

### Set Model

Update the default model used by a specific provider. This will also trigger a seamless memory hot-swap for the user's active session.

```json
{
  "action": "set_model",
  "name": "groq",
  "model": "llama-3.2-1b-preview"
}
```
