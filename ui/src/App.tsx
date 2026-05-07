import { type ReactNode, useEffect } from 'react';
import { createBrowserRouter, Navigate, RouterProvider, useLocation, type RouteObject } from 'react-router';

import { BrandMark } from './components';
import { getWhoami, setUnauthorizedHandler } from './lib/api';
import { clearAuthState, getAuthState, setAuthenticatedAccount, useAuthState } from './lib/auth-state';
import Catalog from './screens/Catalog';
import Contracts from './screens/Contracts';
import Dashboard from './screens/Dashboard';
import Login from './screens/Login';
import Sessions from './screens/Sessions';
import Workgroups from './screens/Workgroups';

export function App() {
  useEffect(() => {
    const abort = new AbortController();
    const unregisterUnauthorizedHandler = setUnauthorizedHandler(() => {
      clearAuthState();
    });

    void getWhoami({ signal: abort.signal, suppressUnauthorizedHandler: true })
      .then((account) => {
        if (!abort.signal.aborted) {
          setAuthenticatedAccount(account);
        }
      })
      .catch(() => {
        if (!abort.signal.aborted && getAuthState().status !== 'authenticated') {
          clearAuthState();
        }
      });

    return () => {
      abort.abort();
      unregisterUnauthorizedHandler();
    };
  }, []);

  return <RouterProvider router={router} />;
}

const routes: RouteObject[] = [
  {
    path: '/',
    element: (
      <RequireAuth>
        <Dashboard />
      </RequireAuth>
    ),
  },
  { path: '/login', element: <LoginRoute /> },
  {
    path: '/sessions',
    element: (
      <RequireAuth>
        <Sessions />
      </RequireAuth>
    ),
  },
  {
    path: '/workgroups',
    element: (
      <RequireAuth>
        <Workgroups />
      </RequireAuth>
    ),
  },
  {
    path: '/catalog',
    element: (
      <RequireAuth>
        <Catalog />
      </RequireAuth>
    ),
  },
  {
    path: '/contracts',
    element: (
      <RequireAuth>
        <Contracts />
      </RequireAuth>
    ),
  },
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

function RequireAuth({ children }: { children: ReactNode }) {
  const auth = useAuthState();
  const location = useLocation();

  if (auth.status === 'unknown') {
    return <AuthLoadingScreen />;
  }

  if (auth.status === 'unauthenticated') {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return children;
}

function LoginRoute() {
  const auth = useAuthState();

  if (auth.status === 'unknown') {
    return <AuthLoadingScreen />;
  }

  if (auth.status === 'authenticated') {
    return <Navigate to="/" replace />;
  }

  return <Login />;
}

function AuthLoadingScreen() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-page px-6 py-10 text-text">
      <section className="w-full max-w-[26rem] rounded-card border border-border bg-panel p-6 shadow-sm">
        <BrandMark product="agora" className="justify-center" />
        <div className="mt-8 h-2 overflow-hidden rounded-status bg-panel-subtle">
          <div className="h-full w-1/2 rounded-status bg-brand-agora" />
        </div>
      </section>
    </main>
  );
}
