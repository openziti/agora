import { createBrowserRouter, RouterProvider, type RouteObject } from 'react-router';

import Dashboard from './screens/Dashboard';
import Sessions from './screens/Sessions';
import Workgroups from './screens/Workgroups';

export function App() {
  return <RouterProvider router={router} />;
}

const routes: RouteObject[] = [
  { path: '/', Component: Dashboard },
  { path: '/sessions', Component: Sessions },
  { path: '/workgroups', Component: Workgroups },
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
