// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Suspense, type ComponentType, type ReactNode } from 'react';
import { useRouteError, type RouteObject } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Spinner } from '@resources/components/ui/Spinner';
import { Button } from '@resources/components/ui/Button';

export function ErrorPage() {
  const { t } = useTranslation('common');
  const error = useRouteError() as Error;
  return (
    <div className="flex h-screen items-center justify-center bg-background">
      <div className="max-w-md rounded-lg border border-border bg-card p-8 text-center">
        <h1 className="mb-2 text-xl font-bold text-foreground">{t('errors.somethingWentWrong')}</h1>
        <p className="mb-4 text-sm text-muted-foreground">{error?.message || t('errors.unexpectedError')}</p>
        <Button onClick={() => window.location.reload()}>{t('errors.reloadPage')}</Button>
      </div>
    </div>
  );
}

export function SuspenseWrapper({ children }: { children: ReactNode }) {
  return (
    <Suspense
      fallback={
        <div className="flex h-64 items-center justify-center">
          <Spinner size="lg" />
        </div>
      }
    >
      {children}
    </Suspense>
  );
}

// `handle.scroll` declares each route's scroll behaviour, read by AppLayout
// through `useMatches()`; see `resources/layout/PageShell.tsx` for the contract.
export type RouteHandle = { scroll: 'data' | 'page'; padded?: boolean; sidebar?: boolean; onPhone?: boolean };

export function pageRoute(path: string, Page: ComponentType, handle?: RouteHandle): RouteObject {
  const route: RouteObject = {
    path,
    element: (
      <SuspenseWrapper>
        <Page />
      </SuspenseWrapper>
    ),
  };
  if (handle) route.handle = handle;
  return route;
}
