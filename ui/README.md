# Agora UI

React dashboard shell for Agora. This slice is a Vite + React 19 + TypeScript app with Tailwind v4 configured from `src/globals.css`.

## Development

```sh
npm install
npm run dev
```

The dev server proxies `/v1` to `http://localhost:18080`, matching the local controller default used by the dashboard work order.

Point the proxy at a different controller by setting `AGORA_CONTROLLER_URL`:

```sh
AGORA_CONTROLLER_URL=http://localhost:28080 npm run dev
```

The proxy preserves the `/v1` prefix on both sides. A browser request to
`http://localhost:5173/v1/sessions` is forwarded to
`http://localhost:18080/v1/sessions` by default.

## Runtime Config

`index.html` defines `window.__AGORA_CONFIG__` before the React entrypoint runs.
Read it through `src/lib/config.ts` instead of reading the global directly.

The config is for non-sensitive deployment metadata only:

- `apiBasePath`: same-origin API prefix, default `/v1`
- `version`: display/build version string, default `dev`
- `demoFlags`: optional demo display flags

Future fields should follow the same rule: values that vary by deployment and
are safe for every browser user to inspect can live here. Credentials, secrets,
session material, and account identity belong in the cookie-based auth flow, not
runtime config.

## Verification

```sh
npm run build
npm run lint
npm run test
```

`npm run test` uses `vitest run --passWithNoTests` until later slices add UI tests.

The Go controller embeds `dist/` in normal builds. Build the SPA before
running untagged Go builds or tests that import `ui`:

```sh
npm run build
go test ./ui
```

Use the `no_agora_ui` build tag when a Go-only workflow should not require
`dist/` to exist:

```sh
go build -tags no_agora_ui ./...
```

`ui.Middleware(apiHandler)` forwards `/v1` requests to the supplied API handler
and serves the React app, including the `index.html` fallback for SPA routes, for
all other GET and HEAD requests.

## Assets

Inter and JetBrains Mono are loaded from `@fontsource-variable` packages in `src/main.tsx`; the app does not depend on remote font hosts at runtime.
