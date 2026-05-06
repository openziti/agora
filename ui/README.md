# Agora UI

React dashboard shell for Agora. This slice is a Vite + React 19 + TypeScript app with Tailwind v4 configured from `src/globals.css`.

## Development

```sh
npm install
npm run dev
```

The dev server proxies `/v1` to `http://localhost:18080`, matching the local controller default used by the dashboard work order.

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
