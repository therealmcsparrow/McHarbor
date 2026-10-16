// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createBrowserRouter, Navigate, type RouteObject } from 'react-router';
import { AppLayout } from '@resources/layout/AppLayout';
import { AuthLayout } from '@resources/layout/AuthLayout';
import { MFAEnrollmentGuard } from '@core/auth/MFAEnrollmentGuard';
import { OnCallLayout } from '@modules/oncall/layout/OnCallLayout';
import './router/shellWiring';
import { appRoutes } from './router/appRoutes';
import { ErrorPage, pageRoute } from './router/routeHelpers';
import * as P from './router/lazyPages';

const authHandle = { scroll: 'data' } as const;

const routes: RouteObject[] = [
  { ...pageRoute('/', P.RootPage), errorElement: <ErrorPage /> },
  {
    path: '/oncall',
    element: (
      <MFAEnrollmentGuard>
        <OnCallLayout />
      </MFAEnrollmentGuard>
    ),
    errorElement: <ErrorPage />,
    children: [
      { index: true, element: <Navigate to="/oncall/alerts" replace /> },
      pageRoute('alerts', P.OnCallAlertsPage),
      pageRoute('fleet', P.OnCallFleetPage),
      pageRoute('containers/:envId/:containerId', P.OnCallContainerPage),
      pageRoute('containers/:envId/:containerId/logs', P.OnCallLogsPage),
      pageRoute('runbooks/:runbookId', P.OnCallRunbookPage),
      pageRoute('approvals', P.OnCallApprovalsPage),
      pageRoute('device', P.OnCallDevicePage),
      { path: '*', element: <Navigate to="/oncall/alerts" replace /> },
    ],
  },
  { ...pageRoute('/mfa-enrollment', P.MFAEnrollmentRequiredPage), errorElement: <ErrorPage /> },
  {
    element: <AuthLayout />,
    errorElement: <ErrorPage />,
    children: [
      pageRoute('/login', P.LoginPage, authHandle),
      pageRoute('/setup/database', P.DatabaseSetupPage, authHandle),
      pageRoute('/setup', P.SetupPage, authHandle),
      pageRoute('/force-password-change', P.ForcePasswordChangePage, authHandle),
      pageRoute('/reset-password', P.PasswordResetRequestPage, authHandle),
      pageRoute('/reset-password/:token', P.PasswordResetConsumePage, authHandle),
    ],
  },
  {
    element: (
      <MFAEnrollmentGuard>
        <AppLayout />
      </MFAEnrollmentGuard>
    ),
    errorElement: <ErrorPage />,
    children: appRoutes,
  },
];

export const router = createBrowserRouter(routes);
