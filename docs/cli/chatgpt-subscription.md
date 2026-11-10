# ChatGPT Subscription

ChatGPT Subscription (`openai_chatgpt`, `auth_mode: "oauth"`) uses official OpenAI browser OAuth and your eligible ChatGPT plan permissions. It is separate from an OpenAI API-key provider. It does not read authentication caches from other applications or use private ChatGPT endpoints.

This is the command-oriented quick guide. See the detailed [ChatGPT Subscription integration reference](../provider/chatgpt-subscription.md) for OAuth internals, API payloads, Responses/tool continuation, capability limits, and troubleshooting, or the [Provider Integration Guide](../provider/README.md) to compare integrations.

## Local browser setup

Run on a desktop where the browser and Myaaw run as the same local user on the **same machine**:

```sh
myaaw --agent myfriend provider connect-chatgpt personal --default
# Select a model returned by live discovery.

# Or choose a known model explicitly (no model picker):
myaaw --agent myfriend provider connect-chatgpt personal --model YOUR_MODEL_ID --default
```

The command launches the system browser directly, without a shell, and waits up to ten minutes for the loopback callback. Authorization URLs are not printed or logged: reconnect URLs can include identity-token hints. `Ctrl+C` cancels the attempt. The saved provider contains only a local credential reference, never OAuth tokens. `provider create` and the onboarding wizard also offer **ChatGPT Subscription** and skip API-key/base-URL prompts.

The dashboard must also be opened through `localhost` or a loopback IP on the API host. Login APIs reject non-loopback socket peers, hosts, browser origins, and referrers; forwarded headers are not trusted to establish locality. The frontend proxy preserves Origin/Referer even though it rewrites Host, so a remote dashboard is still rejected. Browser requests with neither Origin nor Referer are rejected; use a normal local dashboard session rather than stripping both headers. A remote browser cannot complete the server's loopback callback. An ordinary SSH port-forward is not sufficient: its browser callback would target the client machine, not the API host. Use a desktop session on the host for the initial sign-in, then attach its existing credential in headless automation.

## Reconnect, status, disconnect

```sh
# Reauthorize exactly the registration already attached to this provider:
myaaw --agent myfriend provider connect-chatgpt personal --model YOUR_MODEL_ID

# Metadata only: connection state, local credential ID, account label, expiry:
myaaw --agent myfriend provider status-chatgpt personal

# Detach only this provider reference:
myaaw --agent myfriend provider disconnect-chatgpt personal
```

**Disconnect is not sign-out or revocation.** It clears only that provider's `credential_id`, keeping its model/type and the host credential. Other providers and agents sharing the credential continue to work. Deleting a provider also leaves shared credentials intact. To revoke the remote app grant, manage connected apps in ChatGPT Settings. Explicit local credential deletion is a separate auth-manager operation, not this command/API.

Connection status is a local snapshot, not an online revocation check:

- `disconnected`: no credential reference.
- `unavailable`: reference exists in config, but the local credential cannot be read (for example after restoring a backup on another host).
- `connected`: locally usable token/grant metadata.
- `refresh_required`: cached access-token expiry passed; inference/model discovery must refresh.
- `reauthorization_required`: missing usable tokens or required plan scopes; sign in again.

Status never refreshes tokens. Remote revocation can remain undetected until inference or a refresh reaches OpenAI.

## Failed-registration recovery

If an initial callback issued a dynamic client but token exchange failed, the auth store retains a pending registration without unverified identity or tokens. Keep the recovery credential ID printed by the CLI (or returned by the session status API) and resume it:

```sh
myaaw --agent myfriend provider connect-chatgpt personal \
  --retry-credential-id RECOVERY_ID --model YOUR_MODEL_ID
```

This calls OAuth `Start` with the retained binding, even after restart. Starting without that ID creates an independent registration. A pending-only ID is **not** a connected account and cannot be used to create a provider through `--credential-id` or the provider write APIs.

## Headless attachment and onboarding

After a successful local browser sign-in, reuse the printed credential ID **on that same host**:

```sh
myaaw --agent myfriend provider connect-chatgpt personal \
  --credential-id LOCAL_CREDENTIAL_ID --model YOUR_MODEL_ID --default

myaaw --agent worker onboard --yes \
  --provider openai_chatgpt --credential-id LOCAL_CREDENTIAL_ID \
  --model YOUR_MODEL_ID --channel skip
```

`--credential-id` attaches an already verified local credential; it does not open a browser. `--retry-credential-id` instead retries browser authorization for a saved or pending registration. Headless onboarding requires an explicit model (or the existing model for that same subscription provider). OAuth setup ignores unrelated `MYAAW_API_KEY` values; explicitly supplying an API key or custom base URL is rejected.

## Credentials, privacy, and backups

OAuth credentials live in the host-global `~/.myaaw/credentials/chatgpt` store, outside all agent roots and agent backups. They include access/refresh/identity tokens and verified account metadata. On Unix the store uses owner-only directories/files, atomic writes, and process-shared rotation locking. Protect the OS account and filesystem; Windows additionally needs owner-only ACLs.

The agent config stores `type`, `auth_mode`, `credential_id`, and the chosen model only. A backup may include that reference but not the host credential. Restoring it on another machine requires a new local sign-in and updating the reference. Onboarding saves host credentials as soon as browser authorization succeeds, even if you later cancel the agent review; no agent files are created until review is saved.

Signing in shares authorization and account metadata with OpenAI; inference sends the selected conversation/tool content to OpenAI under that grant. Status APIs expose only connection/account/expiry metadata, not tokens, codes, PKCE data, or authorization URLs. Treat returned sign-in URLs as sensitive and do not put them in telemetry, screenshots, logs, or shared support reports.

## Quotas and failures

Availability and quotas depend on your ChatGPT plan, permissions, and OpenAI's current model eligibility. Model selection uses live discovery, not fabricated subscription model defaults. The subscription route does **not** silently fall back to an API key, another account, or paid API usage when authentication, permission, discovery, or quota checks fail. API-key OpenAI/Gemini flows remain separate and unchanged.

See [Providers](provider.md), [Onboarding](onboard.md), and [Control Plane API](../api/control-plane.md#chatgpt-subscription).
