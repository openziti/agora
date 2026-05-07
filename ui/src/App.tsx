import { createBrowserRouter, RouterProvider, type RouteObject } from 'react-router';

import Catalog from './screens/Catalog';
import Dashboard from './screens/Dashboard';
import Login from './screens/Login';
import Sessions from './screens/Sessions';
import Workgroups from './screens/Workgroups';

export function App() {
  return <RouterProvider router={router} />;
}

const routes: RouteObject[] = [
  { path: '/', Component: Dashboard },
  { path: '/login', Component: Login },
  { path: '/sessions', Component: Sessions },
  { path: '/workgroups', Component: Workgroups },
  { path: '/catalog', Component: Catalog },
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
