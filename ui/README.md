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

## Assets

Inter and JetBrains Mono are loaded from `@fontsource-variable` packages in `src/main.tsx`; the app does not depend on remote font hosts at runtime.
