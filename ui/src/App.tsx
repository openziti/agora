import { createBrowserRouter, RouterProvider, type RouteObject } from 'react-router';

import Dashboard from './screens/Dashboard';

export function App() {
  return <RouterProvider router={router} />;
}

const routes: RouteObject[] = [
  { path: '/', Component: Dashboard },
  ...(import.meta.env.DEV
    ? [
        {
          path: '/_kit',
          lazy: async () => ({ Component: (await import('./_kit/Kit')).default }),
        },
      ]
    : []),
];

const router = createBrowserRouter(routes);
