# ChatGPT Subscription authentication

`ProviderType = "openai_chatgpt"`; `Label = "ChatGPT Subscription"`.

This package implements only official OpenAI public-client OAuth. It does not
read other applications' authentication caches or use private ChatGPT endpoints.

## Integration contract

- `DefaultManager() (*Manager, error)` uses the host-global
  `~/.myaaw/credentials/chatgpt` directory, independent of the selected agent.
- `NewManager(dir string) (*Manager, error)` uses an explicit private store.
- `Manager.Start(ctx, credentialID) (*Attempt, error)` starts a loopback listener
  before returning. An empty ID creates a separate registration; a saved ID
  reauthorizes that exact client/subject binding. The ID from a failed initial
  attempt's `Status().CredentialID` can also resume its retained, unverified
  client registration after restart, without creating another dynamic client.
  The context owns the attempt's
  lifetime: do not use a short-lived HTTP request context if the attempt must
  survive returning the authorization URL to a UI.
- `Attempt.ID`, `Attempt.AuthorizationURL`, and `Attempt.ExpiresAt` are immutable.
  **Do not log authorization URLs:** returning sign-in URLs contain ID-token hints.
- `Attempt.Wait(ctx) (Credential, error)` waits for completion. Cancelling this
  context stops only the waiter. `Attempt.Cancel()` stops the entire attempt.
- `Attempt.Status() AttemptStatus` is safe for API status responses and contains
  only `status` (`pending`, `connected`, `failed`, `expired`), `credential_id`,
  `email`, and sanitized `error`. Never serialize `Credential` as a status result.
- `Manager.Get(id) (Credential, error)` reads the saved record, including secrets.
- `Manager.AccessToken(ctx, id) (string, error)` returns a usable access token,
  refreshing near expiry when permitted. Rotations reload and update the entire
  record under a kernel lock shared by independent processes. It never returns
  expired access tokens, or refreshes before `earliest_refresh_at`. Once a
  rotation response is received, it finishes bounded identity validation and
  persists the replacement even if the requesting context is cancelled.
- `Manager.Delete(id) error` deletes only the local account or pending
  registration. It does **not** revoke the remote session and is not a complete
  sign-out operation. Users can disconnect the app in ChatGPT Settings. The
  official sign-out flow attempts refresh-token revocation before clearing
  tokens, while retaining the client/account mapping; this local-deletion
  contract deliberately does not implement or claim those semantics.

`Credential` exposes `ID`, `ClientID`, `Subject`, `Email`, `AccessToken`,
`RefreshToken`, `IDToken`, `ExpiresAt` (`time.Time`), and `Scopes` (`[]string`).
IDs are opaque local UUIDs, not emails or OAuth client IDs. Registrations with the
same email or subject remain separate. Missing records return `ErrNotFound`
(`os.ErrNotExist`). Confirmed unusable refresh tokens return
`ErrReauthorizationRequired`, clear the unusable token set, and retain the
client/subject/email mapping for returning sign-in. Both standard OAuth string
errors and structured `error.code` responses are recognized. Temporary network,
rate-limit, and infrastructure errors preserve records; `invalid_client` does
not erase tokens. OpenAI does not send disconnection notifications: cached
access tokens cannot reveal remote revocation until a request or refresh detects
it. `AccessToken` handles confirmed terminal refresh failures, not inference
response handling (which belongs in `internal/provider/chatgpt*.go`).

A verified ID token without the full required grant is retained in the account
store, but the attempt fails permission validation and must not be activated for
inference. `AccessToken` returns an error and no token for such accounts. A
refresh that reduces scopes still atomically saves its replacement refresh
token and actual grant, so the consumed previous token is not reused. Explicit
reauthorization requests the complete scope set again using the saved binding.

## Storage and validation

The store contains separate `host.json`, `accounts/<id>.json`,
`registrations/<id>.json`, and `.lock` files. A pending registration stores only
its local ID, callback-issued client ID, issuer, and host ID. It is **not** a
verified account: `Get` and `AccessToken` return `ErrNotFound` for pending-only
records. No unverified identity, tokens, code, or PKCE material is stored there.
It is retained before exchanging the code, including on `invalid_grant`, and
removed after committing verified credentials. A fresh `Start(ctx, "")` always
creates an independent registration instead of implicitly selecting a pending
one.

Directories are `0700`; files are `0600` and atomically replaced. Files and,
on Unix, their containing directories are synced. The UUID-based `urn:uuid:`
host ID survives account deletion and process restarts. Directory handles are
pinned with `os.Root`, so directory swaps cannot redirect reads, replacements,
or deletions outside the opened store. Symlinks, special files, hard-linked
credential/lock files, and (on Unix) foreign ownership are rejected before
permission repair or reading. Reauthorization commits reject deleted accounts
and stale snapshots changed by another sign-in or refresh.

Supported locking implementations cover macOS, Linux, BSD, and Windows; Unix
permission guarantees apply on Unix filesystems. Windows requires appropriate
owner-only host account ACLs; Go's `0600` mode does not configure Windows ACLs.

Every attempt uses fresh random state, nonce, and S256 PKCE values, a ten-minute
maximum lifetime, and `http://127.0.0.1:<ephemeral>/auth/callback`. ID-token
signatures and issuer/audience/expiry are checked with maintained `go-oidc` /
`go-jose` libraries against OpenAI's JWKS; subject, issued-at, nonce, and returning
identity bindings are also checked, as are not-before, authorized party for
multiple audiences, and `at_hash` when present. Requests with untrusted state do
not consume the real attempt; a state-validated callback is redeemed at most
once. Required granted scopes for inference are `openid`,
`offline_access`, `resource.invoke`, and `chatgpt.tokens.use.direct`.

Token expiry comes from `expires_in`, conservatively measured from request start.
The official token-reference page lists `earliest_refresh_at` without defining a
wire type; this implementation accepts Unix seconds (number or numeric string)
or RFC3339, rejects malformed values, and persists the resulting timestamp.

Token response bodies, authorization codes, tokens, state, and verifier material
are never included in errors or callback pages. Credential-bearing HTTP redirects
are not followed. Test-only endpoint overrides are package-private.

## Validation

```sh
go test ./internal/auth/chatgpt
go test -race ./internal/auth/chatgpt
go vet ./internal/auth/chatgpt
```

Tests use local `httptest` issuers and signed JWTs; no real OpenAI login or live
credentials are needed. Cross-process tests execute the test binary against a
shared temporary store and assert that only one rotating refresh occurs.
Regression tests cover RS256 and ES256 signatures, algorithm rejection, actual
JWKS key rotation, PKCE and callback bindings, failed-registration recovery,
revocation, scope withdrawal, stale reauthorization, cancellation after rotation,
and filesystem symlink/hard-link/FIFO/directory-swap attacks.

## Official flow audited

- [Registration and sign-in](https://developers.openai.com/siwc/token-sharing-open-source/sign-in.md)
- [Accounts and sessions](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions.md)
- [Token reference](https://developers.openai.com/siwc/token-sharing-open-source/token-reference.md)
- [Errors and recovery](https://developers.openai.com/siwc/token-sharing-open-source/errors-and-recovery.md)
- [Production discovery metadata](https://auth.openai.com/.well-known/openid-configuration)

OAuth and protected credential storage live only in this package. Provider
model discovery and streaming belong in `internal/provider/chatgpt*.go`; they
are intentionally not implemented here. The integration label is exactly
**ChatGPT Subscription**.
