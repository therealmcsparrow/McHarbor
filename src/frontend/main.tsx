// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { MutationCache, QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { toast, Toaster } from 'sonner';
import type { MutationMeta } from '@resources/utils/api-mutation';
import { AuthProvider } from '@core/auth/AuthProvider';
import { ThemeProvider } from '@resources/hooks/useTheme';
import { TooltipProvider } from '@resources/components/ui/Tooltip';
import i18n, { initializeI18n } from '@core/i18n/i18n';
import App from './App';
import './app.css';

const mutationCache = new MutationCache({
  onSuccess: (data, variables, _context, mutation) => {
    const meta = mutation.options.meta as MutationMeta | undefined;
    if (meta?.success) {
      const msg = typeof meta.success === 'function'
        ? meta.success(data, variables)
        : meta.success;
      if (msg) toast.success(msg);
    }
  },
  onError: (error, _variables, _context, mutation) => {
    const meta = mutation.options.meta as MutationMeta | undefined;
    toast.error(meta?.error ?? error.message ?? i18n.t('errors.operationFailed'));
  },
});

const queryClient = new QueryClient({
  mutationCache,
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

await initializeI18n();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider delayDuration={200}>
        <AuthProvider>
          <App />
          <Toaster
            position="bottom-right"
            theme="system"
            richColors
            closeButton
          />
        </AuthProvider>
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>
);
