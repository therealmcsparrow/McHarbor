// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { MutationCache, QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppToaster } from '@resources/components/AppToaster';
import { isApprovalRequired, type MutationMeta } from '@resources/utils/api-mutation';
import { appToast, toastError } from '@resources/utils/toast';
import { AuthProvider } from '@core/auth/AuthProvider';
import { PermissionsBridge } from '@core/auth/PermissionsBridge';
import { shouldRetryQuery } from '@core/api/queryRetry';
import { publishEnvironmentBridge } from '@core/plugins/environmentBridge';
import { publishToastBridge } from '@core/plugins/toastBridge';
import { ThemeProvider } from '@resources/hooks/useTheme';
import { TooltipProvider } from '@resources/components/ui/Tooltip';
import { initializeI18n } from '@core/i18n/i18n';
import { useMotionStore } from '@resources/stores/motion';
import { applyInitialTheme } from './src/themes';
import { registerServiceWorker } from '@core/pwa/register';
import App from './App';
import './app.css';

// Apply the motion preference before the first paint, so a user who has
// turned motion down never sees the one frame of animation that would
// otherwise land between hydration and the store rehydrating.
useMotionStore.getState().sync();

// Same reasoning for the theme: link the persisted theme's stylesheet and
// set `.dark` before React mounts, so the first paint is already the
// user's theme rather than a frame of the app.css defaults.
applyInitialTheme();

const mutationCache = new MutationCache({
  onSuccess: (data, variables, _context, mutation) => {
    const meta = mutation.options.meta as MutationMeta | undefined;
    if (meta?.success) {
      const msg = typeof meta.success === 'function' ? meta.success(data, variables) : meta.success;
      if (msg) appToast.success(msg);
    }
  },
  onError: (error, _variables, _context, mutation) => {
    if (isApprovalRequired(error)) {
      appToast.info(error.message);
      return;
    }
    // meta.error names the operation; the server's own message and cause are
    // always shown under it, never replaced by it.
    const meta = mutation.options.meta as MutationMeta | undefined;
    const operation = typeof meta?.error === 'function' ? meta.error(error) : meta?.error;
    toastError(error, operation || undefined);
  },
});

const queryClient = new QueryClient({
  mutationCache,
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      retry: shouldRetryQuery,
      refetchOnWindowFocus: false,
    },
  },
});

await initializeI18n();

// Before the first render, so a plugin bundle mounted on the very first route
// already finds the selected environment rather than a private copy of the
// store. Unlike the permissions bridge this needs no query and no auth — the
// store is already hydrated from localStorage by the time this line runs.
publishEnvironmentBridge();
publishToastBridge();
registerServiceWorker();

const rootElement = document.getElementById('root');

if (!rootElement) {
  throw new Error('root element not found');
}

createRoot(rootElement).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider delayDuration={200}>
          <AuthProvider>
            <PermissionsBridge />
            <App />
            <AppToaster />
          </AuthProvider>
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
);
