# Cookie-Based Authentication — Reference

This document is preserved verbatim from the zrok project (its `COOKIE_AUTH.md`), where it captures the design for migrating zrok's web UI from `localStorage`-based auth to `httpOnly` cookies. Agora adopts the same architecture from day one rather than retrofitting it. This file is the implementer's reference; the canonical agora design lives in `design.md` ("Authentication Posture" section), and the agora-specific work units live in `work-order.md` (Track G).

## How to read this document

Read the rest of this file as the architectural background and design rationale. The shape of the solution — cookie-to-header middleware, double-submit CSRF, login-cookie-emit on success, logout-cookie-clear — is shared between zrok and agora. The deltas below identify everywhere agora differs.

## Agora vs. zrok deltas

The deltas are listed in the order the original document presents the corresponding material.

| Topic | Zrok (this document) | Agora |
| --- | --- | --- |
| Cookie name source | `controller/config/config.go` `WebUIConfig` struct, configurable via YAML | Hard-coded `agora-session` and `agora-csrf` in `internal/controller/auth_middleware.go`. No config struct, no exposure via `/configuration`. Configurability can be added later if a deployment requires it. |
| Cookie name discovery (UI) | UI fetches `csrfCookieName` from `/configuration` on app load and stores in Zustand | UI reads the literal `agora-csrf` cookie name from `ui/src/lib/cookies.ts`. Hard-coded both sides. |
| Login response shape | Modify `/login` to return `{email}` instead of raw token; ripples into `Login.tsx` `d.email` vs `d.toString()` | Login response shape **stays unchanged** at `AccountTokenResponse{accountToken}`. The CLI consumes this same endpoint and we do not change its contract. The dashboard SPA reads-and-discards the body's `accountToken` after login; the cookie carries auth from there. |
| Cookie-emission mechanism | go-swagger `middleware.Responder` wrapper (`loginCookieResponder`) attached at the handler return | HTTP middleware around the ogen handler (response-side `loginCookieEmitMiddleware`) that buffers the response, parses `accountToken` from the body, sets the cookies, and forwards the unchanged body. Different mechanism, same outcome. |
| OpenAPI codegen | go-swagger; spec changes regenerate via `bin/generate_rest.sh` into `rest_client_zrok/`, `rest_server_zrok/`, `rest_model_zrok/` | ogen; spec changes regenerate via `bin/generate_rest.sh` into `internal/api/oas_*`. Spec modules live in `internal/api/specs/<module>/`. |
| Spec composition | `specs/src/account.yml`, `specs/src/metadata.yml`, `specs/src/definitions.yml` | `internal/api/specs/account/paths.yml`, `internal/api/specs/account/schemas.yml`, etc. Same per-module convention. |
| Account password storage | Already present on the `accounts` table | Already present on the `accounts` table (columns `password_salt` and `password_hash`). No schema work. |
| Password hashing | Existing zrok `rehashPassword(password, salt)` helper | Existing agora `internal/controller/passwords.go` with argon2id-based `hashPassword`, `verifyPassword`, `rehashPassword`. **Already implemented and tested.** |
| Login handler | Existing `controller/login.go` returning the raw token; needs to be modified to return `{email}` and to set cookies | Existing `internal/controller/login.go` returning `AccountTokenResponse{accountToken}`. **Response shape stays unchanged** so the CLI continues to consume the same endpoint. Cookies are set by the response-side middleware (G.4); the handler does not touch cookies. The handler body *is* modified by Track A's A.3 work unit to emit `account.login` (success path) and `account.login_failed` (bad-password path) audit events, but that change is invisible to the CLI. |
| `regenerateAccountToken` handler | Modify to update the session cookie after token change | Same response-side middleware (`loginCookieEmitMiddleware`) handles `/v1/account/regenerate-token` automatically. Handler is not modified. |
| Logout endpoint | New `POST /logout` | New `POST /v1/account/logout`, scoped under the existing `account` module to keep the OpenAPI grouping consistent. |
| UI `User` type changes | Make `token` optional, since it's only present transiently during registration display | Same — `token` field in `ui/src/model/user.ts` is optional and only used transiently for registration display. |
| UI `getXApi(user)` refactor | Touches 12 files; mechanical removal of the `user` parameter | **Not applicable.** Agora's UI is greenfield with this work; the API client is built cookie-first from the start. There are no `getXApi(user)` call sites to refactor. |
| `RegenerateAccountTokenModal` | Stop persisting token to localStorage | **Not applicable in MVP.** Agora has no regenerate-account-token UI in dashboard scope. The cookie-emit middleware will refresh the session cookie correctly when this UI is added later. |
| `Register.tsx` | Stop persisting token | **Not applicable.** Agora has no registration UI in dashboard scope; `demo-bootstrap` provisions accounts directly. |
| Login form library | MUI v6 components (`TextField`, `Button`, etc.) | Tailwind v4 + plain HTML inputs styled with the dashboard's design tokens. No MUI in agora's UI. |
| Configuration endpoint | `/configuration` exposes `csrfCookieName` to the UI | No `/configuration` endpoint added in this work. (One may be added later for version display, but it is not required by the auth track.) |
| Package boundary issue | zrok's `configure_zrok.go` is in `rest_server_zrok` package; can't access controller config directly; needs setter or threading | Not applicable. Agora's middleware lives in `internal/controller/auth_middleware.go` alongside the controller; direct access to `cfg`. |
| TLS-aware `Secure` flag | Read `cfg.Tls != nil` from controller config and thread to cookie creation | Same — agora's controller config has a `Tls` field; pass `cfg.Tls != nil` into the middleware constructor. |
| CLI compatibility | Critical concern; zrok has deployed users on `X-TOKEN` headers | Less load-bearing — agora's CLI is not yet deployed at scale. The same dual-path posture is maintained anyway (CLI uses `X-TOKEN`, browser uses cookies, cookie-to-header middleware bridges) so CLI keeps working through this transition. |
| Migration steps | Significant: remove `user` param from `getXApi` calls (12 files), modify Login response handling, modify token regeneration flow, update Register/Reset flows | **Most steps not applicable.** Agora ships cookie-first from day one. The agora work order (Track G) captures the actually-needed steps. |

## What follows

The remainder of this file is the original zrok document. Treat it as the architectural reference. Where it conflicts with the deltas above, the deltas win. Where the agora work order in `work-order.md` Track G describes specific work units, those win over both.

---

# Issue #4: Move auth token from localStorage to httpOnly cookie

## Context

The web UI stores the permanent account token in plaintext `localStorage`, accessible to any JavaScript on the page. While XSS (#2) and console.log leaks (#5) are now fixed, a future XSS regression or malicious browser extension could steal the token. The token is permanent (no expiry), shared with CLI, and equivalent to a password.

**Goal**: Move the web UI to cookie-based auth so JavaScript never sees the token. CLI auth is unaffected.

**Architectural constraints preserved**:
- Controller remains stateless (no session tables) -- the cookie carries the token
- Same DB lookup per request (unchanged)
- CLI continues using `X-TOKEN` header directly
- Same-origin deployment (UI embedded in controller binary)

## Configuration

Add a `WebUI` section to the controller config (`controller/config/config.go`):

```go
type WebUIConfig struct {
    SessionCookieName string // defaults to "zrok-webui-session"
    CsrfCookieName    string // defaults to "zrok-webui-csrf"
}
```


The `Config` struct gains a `WebUI *WebUIConfig` field. `DefaultConfig()` sets the defaults. The middleware and login handler read cookie names from this config.

Example YAML:
```yaml
webUi:
  sessionCookieName: zrok-webui-session
  csrfCookieName:    zrok-webui-csrf
```

The UI also needs to know the CSRF cookie name to read it from `document.cookie`. The `/configuration` metadata endpoint (already used by the UI on load) can expose it:

```yaml
# in the configuration response
csrfCookieName: zrok-webui-csrf
```

## Implementation Plan

### Part 1: Server-side -- cookie middleware + login changes

**1a. Cookie-to-header middleware** (`rest_server_zrok/configure_zrok.go:52`)

In `setupMiddlewares`, add middleware that runs after routing but before authentication:
- Read the `zrok-webui-session` cookie from the request (name configurable via `WebUI.SessionCookieName` in controller config, defaults to `zrok-webui-session`)
- If present AND `x-token` header is NOT already set, copy the cookie value into the `x-token` header
- This lets the existing `zrokAuthenticator.authenticate` work unchanged

```go
func setupMiddlewares(handler http.Handler) http.Handler {
    sessionCookie := cfg.WebUI.SessionCookieName // default: "zrok-webui-session"
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // if no X-TOKEN header (CLI), check for session cookie (web UI)
        if r.Header.Get("x-token") == "" {
            if c, err := r.Cookie(sessionCookie); err == nil {
                r.Header.Set("x-token", c.Value)
            }
        }
        handler.ServeHTTP(w, r)
    })
}
```

**1b. CSRF middleware** (`rest_server_zrok/configure_zrok.go:58`)

In `setupGlobalMiddleware`, add CSRF validation (double-submit cookie pattern):
- On non-safe methods (POST, PUT, DELETE, PATCH) when `zrok-webui-session` cookie is present:
  - Compare `zrok-webui-csrf` cookie value to `X-CSRF-Token` header value
  - If they don't match, return 403
- Skip CSRF check if request uses `x-token` header directly (CLI path)
- Skip CSRF check for `/api/v2/login` (no cookie exists yet)

**1c. Modify login handler** (`controller/login.go`)

After successful credential validation:
- Generate a CSRF token (reuse `CreateToken()` from `controller/util.go`)
- Set `zrok-webui-session` cookie: httpOnly, Secure (when TLS), SameSite=Strict, Path=/api/v2
- Set `zrok-webui-csrf` cookie: NOT httpOnly (JS must read it), Secure, SameSite=Strict, Path=/ (name configurable via `WebUI.CsrfCookieName`, defaults to `zrok-webui-csrf`)
- Return JSON response `{"email": a.Email}` instead of the raw token string

This requires changing the login handler from `account.LoginHandlerFunc(loginHandler)` to a custom handler that can access `http.ResponseWriter` to set cookies. Use the `middleware.ResponderFunc` pattern.

**1d. Add logout endpoint**

- New `POST /logout` endpoint (no auth required, just clears cookies)
- Spec: add to `specs/src/account.yml`
- Handler: set `zrok-webui-session` and `zrok-webui-csrf` cookies with `MaxAge: -1` to delete them
- Regenerate API: `bin/generate_rest.sh`

**1e. Update regenerateAccountToken handler** (`controller/regenerateAccountToken.go`)

After generating new token, also update the `zrok-webui-session` cookie with the new token value so the web session doesn't break.

### Part 2: UI-side -- switch from header auth to cookie auth

**2a. Change `User` type** (`ui/src/model/user.ts`)

```ts
export interface User {
    email: string;
    token?: string;  // only present transiently (registration display)
}
```

**2b. Change API config** (`ui/src/model/api.ts`)

Stop sending `X-TOKEN` header. Use `credentials: 'same-origin'` so browser sends cookies. Add CSRF header on every request by reading `zrok-webui-csrf` cookie.

```ts
// csrfCookieName fetched from /configuration endpoint on app load, stored in Zustand
function getCsrfToken(cookieName: string): string | undefined {
    const match = document.cookie.match(new RegExp(`(?:^|;\\s*)${cookieName}=([^;]*)`));
    return match ? match[1] : undefined;
}

export const getApiConfig = (csrfCookieName: string): Configuration => {
    return new Configuration({
        credentials: 'same-origin',
        headers: {
            'X-CSRF-Token': getCsrfToken(csrfCookieName) ?? '',
        },
    });
}
```

Note: `getApiConfig` no longer needs a `user` parameter for authenticated requests. It takes the CSRF cookie name (from the `/configuration` response, stored in the Zustand store) instead.

**2c. Update function signatures** -- all `getAccountApi(user)`, `getMetadataApi(user)`, `getShareApi(user)`, `getEnvironmentApi(user)` calls lose their `user` parameter. This touches many files but is mechanical.

**2d. Update `App.tsx`** (`ui/src/App.tsx`)

- `login()`: call the login API, on success store `{email}` in localStorage (for display only, no token). Cookies are set by the server response.
- `logout()`: call `POST /logout` (clears cookies server-side), then `localStorage.removeItem("user")`, then update store.
- `checkUser()`: still reads localStorage for email. Auth validity comes from the cookie (if the cookie is expired/missing, API calls will 401).

**2e. Update `Login.tsx`** (`ui/src/Login.tsx`)

- Login response is now `{email}` instead of a raw token string
- `onLogin({email: d.email})` instead of `onLogin({email: email, token: d.toString()})`

**2f. Update `RegenerateAccountTokenModal.tsx`**

- Server response still contains `accountToken` for display
- Stop storing token in localStorage: remove `localStorage.setItem("user", JSON.stringify(newUser))`
- The session cookie is updated server-side; the UI just needs to refresh the email in store

**2g. Update `Register.tsx`**

- Registration response still contains `accountToken` for one-time display
- No token stored in localStorage

### Part 3: API spec changes

**File**: `specs/src/account.yml`

1. Change `/login` response from `schema: type: string` to:
   ```yaml
   schema:
     properties:
       email:
         type: string
   ```

2. Add `/logout` endpoint:
   ```yaml
   /logout:
     post:
       tags:
         - account
       operationId: logout
       responses:
         200:
           description: logout successful
   ```

3. Regenerate: `bin/generate_rest.sh`

## Files Modified

| File | Change |
|------|--------|
| `controller/config/config.go` | Add `WebUIConfig` struct with cookie name fields + defaults |
| `specs/src/account.yml` | Modify login response schema, add logout endpoint |
| `specs/src/metadata.yml` | Add `csrfCookieName` to configuration response |
| `rest_server_zrok/configure_zrok.go` | Cookie-to-header middleware, CSRF validation middleware (uses config for cookie names) |
| `controller/login.go` | Set cookies (names from config), return `{email}` not raw token |
| `controller/logout.go` | New file: clear cookies (names from config) |
| `controller/controller.go` | Wire up logout handler |
| `controller/configuration.go` | Expose `csrfCookieName` in `/configuration` response |
| `controller/regenerateAccountToken.go` | Update session cookie after token change |
| `ui/src/model/user.ts` | Make `token` optional |
| `ui/src/model/store.ts` | Store `csrfCookieName` from configuration response |
| `ui/src/model/api.ts` | Remove X-TOKEN header, add credentials + CSRF header (dynamic cookie name) |
| `ui/src/App.tsx` | Login/logout flow changes, fetch csrfCookieName from config |
| `ui/src/Login.tsx` | Handle new login response shape |
| `ui/src/RegenerateAccountTokenModal.tsx` | Stop persisting token to localStorage |
| `ui/src/Register.tsx` | Stop persisting token |
| Multiple UI files | Remove `user` param from `getAccountApi`/`getMetadataApi`/`getShareApi`/`getEnvironmentApi` calls |

## Implementation Notes for Fresh Agent

### Package boundary: config access in `configure_zrok.go`

`configure_zrok.go` is in the `rest_server_zrok` package, but the controller config (`cfg`) is a package-level var in the `controller` package. The middleware functions (`setupMiddlewares`, `setupGlobalMiddleware`) cannot access `cfg` directly.

**Solution**: The `configureAPI` function in `configure_zrok.go` already receives `api *operations.ZrokAPI`. Thread the config through by either:
- Adding a package-level setter in `rest_server_zrok` (e.g., `var WebUICfg *WebUIConfig`) set from `controller.Run()` before `server.ConfigureAPI()` is called, or
- Changing `setupMiddlewares`/`setupGlobalMiddleware` to be closures created by `configureAPI` that capture config values passed through the API object

The first approach is simpler and matches the existing `HealthCheck` pattern (line 17 of `configure_zrok.go`: `var HealthCheck func(...)`).

### Setting cookies from go-swagger handlers

Go-swagger handlers return a `middleware.Responder` interface with `WriteResponse(rw http.ResponseWriter, producer runtime.Producer)`. To set cookies, create a custom Responder that wraps the standard response:

```go
type loginCookieResponder struct {
    cookies []*http.Cookie
    wrapped middleware.Responder
}

func (r *loginCookieResponder) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {
    for _, c := range r.cookies {
        http.SetCookie(rw, c)
    }
    r.wrapped.WriteResponse(rw, producer)
}
```

The login handler returns `&loginCookieResponder{cookies: [...], wrapped: account.NewLoginOK().WithPayload(email)}`. Same pattern for `regenerateAccountToken` and `logout`.

### TLS-aware `Secure` flag

The login handler needs to know whether to set `Secure: true` on cookies. The controller config has `cfg.Tls != nil` for this. Pass this as a boolean alongside the cookie names.

### Spec composition

API specs are composed from modular files in `specs/src/`. Never edit `specs/zrok.yml` directly.
- `specs/src/account.yml` -- login/logout endpoints
- `specs/src/metadata.yml` -- `/configuration` endpoint
- `specs/src/definitions.yml` -- the `configuration` model definition (add `csrfCookieName` property here)
- Run `bin/generate_rest.sh` to regenerate `rest_client_zrok/`, `rest_server_zrok/`, and `rest_model_zrok/`

### Complete list of UI files using `get*Api(user)`

These 12 files pass `user` to API config helpers and will need signature updates:
- `src/model/api.ts` (definitions)
- `src/ApiConsole.tsx`
- `src/AccountMetricsModal.tsx`
- `src/AccountPasswordChangeModal.tsx`
- `src/EnvironmentMetricsModal.tsx`
- `src/EnvironmentPanel.tsx`
- `src/ShareMetricsModal.tsx`
- `src/SharePanel.tsx`
- `src/AccessPanel.tsx`
- `src/ReleaseAccessModal.tsx`
- `src/ReleaseEnvironmentModal.tsx`
- `src/ReleaseShareModal.tsx`
- `src/RegenerateAccountTokenModal.tsx`

### Login response type change

The current `LoginOK.Payload` is `string` (the raw token). After the spec change it becomes an object with an `email` field. This changes the generated `login_responses.go` -- the `Payload` type changes from `string` to a struct. The UI's `Login.tsx` must handle `d.email` instead of `d.toString()`.

### Implementation order

1. Config changes (`controller/config/config.go`) -- no dependencies
2. API spec changes (`specs/src/`) + regenerate -- produces new types
3. Server-side handlers (login, logout, regenerateAccountToken) -- uses new types
4. Middleware (`configure_zrok.go`) -- uses config
5. UI changes -- uses new login response shape + cookie-based auth

## Key Design Decisions

1. **Cookie-to-header middleware** instead of modifying go-swagger auth: avoids touching generated code, existing `zrokAuthenticator.authenticate` works unchanged
2. **Double-submit CSRF**: stateless, no DB state, server compares `zrok-webui-csrf` cookie to `X-CSRF-Token` header
3. **Modify existing `/login`** (not new endpoint): CLI doesn't use `/login`, so no compatibility concern
4. **`SameSite=Strict`**: prevents cross-origin cookie sending, strongest CSRF defense at the cookie level
5. **CSRF token on all requests via header**: generated API client already supports custom headers in Configuration

## Verification

1. `bin/generate_rest.sh` -- regenerate API bindings
2. `go install ./...` -- builds without errors
3. `go test ./...` -- existing tests pass
4. `cd ui && npm run build` -- UI builds
5. Manual testing:
   - Login via web UI -- verify `zrok-webui-session` (httpOnly) and `zrok-webui-csrf` (readable) cookies set
   - Verify `localStorage` contains only `{email}`, no token
   - Verify authenticated API calls work (cookie sent automatically)
   - Verify logout clears both cookies
   - Verify CLI `zrok enable` + API calls still work with X-TOKEN header
   - Verify CSRF: manually craft a cross-origin POST -- should get 403
   - Verify token regeneration updates the session cookie
   - `document.cookie` should show `zrok-webui-csrf=...` but NOT `zrok-webui-session`
