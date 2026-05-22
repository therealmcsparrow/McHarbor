// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createBrowserRouter, useRouteError, type RouteObject } from 'react-router';
import { useTranslation } from 'react-i18next';
import { AppLayout } from '@resources/layout/AppLayout';
import { AuthLayout } from '@resources/layout/AuthLayout';
import { Spinner } from '@resources/components/ui/Spinner';
import { Button } from '@resources/components/ui/Button';
import { lazy, Suspense } from 'react';

function ErrorPage() {
  const { t } = useTranslation('common');
  const error = useRouteError() as Error;
  return (
    <div className="flex h-screen items-center justify-center bg-background">
      <div className="max-w-md rounded-lg border border-border bg-card p-8 text-center">
        <h1 className="mb-2 text-xl font-bold text-foreground">{t('errors.somethingWentWrong')}</h1>
        <p className="mb-4 text-sm text-muted-foreground">
          {error?.message || t('errors.unexpectedError')}
        </p>
        <Button onClick={() => window.location.reload()}>
          {t('errors.reloadPage')}
        </Button>
      </div>
    </div>
  );
}

// Lazy-loaded pages
const RootPage = lazy(() => import('@core/auth/RootPage'));
const LoginPage = lazy(() => import('@core/auth/LoginPage'));
const SetupPage = lazy(() => import('@core/auth/SetupPage'));
const DashboardPage = lazy(() => import('@modules/dashboard/pages/DashboardPage'));
const ContainersPage = lazy(() => import('@modules/containers/pages/ContainersPage'));
const ContainerDetailPage = lazy(() => import('@modules/containers/pages/ContainerDetailPage'));
const ImagesPage = lazy(() => import('@modules/images/pages/ImagesPage'));
const ImageDetailPage = lazy(() => import('@modules/images/pages/ImageDetailPage'));
const VolumesPage = lazy(() => import('@modules/volumes/pages/VolumesPage'));
const NetworksPage = lazy(() => import('@modules/networks/pages/NetworksPage'));
const NetworkDetailPage = lazy(() => import('@modules/networks/pages/NetworkDetailPage'));
const StacksPage = lazy(() => import('@modules/stacks/pages/StacksPage'));
const StackDetailPage = lazy(() => import('@modules/stacks/pages/StackDetailPage'));
const TerminalPage = lazy(() => import('@modules/terminal/pages/TerminalPage'));
const LogsPage = lazy(() => import('@modules/logs/pages/LogsPage'));
const EnvironmentsPage = lazy(() => import('@modules/environments/pages/EnvironmentsPage'));
const EnvironmentDetailPage = lazy(() => import('@modules/environments/pages/EnvironmentDetailPage'));
const BlueprintsPage = lazy(() => import('@modules/blueprints/pages/BlueprintsPage'));
const GitPage = lazy(() => import('@modules/git/pages/GitPage'));
const ReconcilerPage = lazy(() => import('@modules/reconciler/pages/ReconcilerPage'));
const ActivityPage = lazy(() => import('@modules/activity/pages/ActivityPage'));
const AuditPage = lazy(() => import('@modules/audit/pages/AuditPage'));
const StorePage = lazy(() => import('@modules/appstore/pages/StorePage'));
const SecurityPage = lazy(() => import('@modules/security/pages/SecurityPage'));
const SettingsPage = lazy(() => import('@modules/settings/pages/SettingsPage'));
const NotificationsPage = lazy(() => import('@modules/notifications/pages/NotificationsPage'));
const WorkflowsPage = lazy(() => import('@modules/workflows/pages/WorkflowsPage'));
const WorkflowEditorPage = lazy(() => import('@modules/workflows/pages/WorkflowEditorPage'));
const WorkflowRunsPage = lazy(() => import('@modules/workflows/pages/WorkflowRunsPage'));

// Docker settings
const DockerPage = lazy(() => import('@modules/docker/pages/DockerPage'));

// Kubernetes pages
const PodsPage = lazy(() => import('@modules/pods/pages/PodsPage'));
const PodDetailPage = lazy(() => import('@modules/pods/pages/PodDetailPage'));
const DeploymentsPage = lazy(() => import('@modules/deployments/pages/DeploymentsPage'));
const DeploymentDetailPage = lazy(() => import('@modules/deployments/pages/DeploymentDetailPage'));
const K8sServicesPage = lazy(() => import('@modules/k8s-services/pages/K8sServicesPage'));
const NamespacesPage = lazy(() => import('@modules/namespaces/pages/NamespacesPage'));

function SuspenseWrapper({ children }: { children: React.ReactNode }) {
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

const routes: RouteObject[] = [
  {
    path: '/',
    errorElement: <ErrorPage />,
    element: <SuspenseWrapper><RootPage /></SuspenseWrapper>,
  },
  {
    element: <AuthLayout />,
    errorElement: <ErrorPage />,
    children: [
      { path: '/login', element: <SuspenseWrapper><LoginPage /></SuspenseWrapper> },
      { path: '/setup', element: <SuspenseWrapper><SetupPage /></SuspenseWrapper> },
    ],
  },
  {
    element: <AppLayout />,
    errorElement: <ErrorPage />,
    children: [
      { path: 'dashboard', element: <SuspenseWrapper><DashboardPage /></SuspenseWrapper> },
      { path: 'containers', element: <SuspenseWrapper><ContainersPage /></SuspenseWrapper> },
      { path: 'containers/:id', element: <SuspenseWrapper><ContainerDetailPage /></SuspenseWrapper> },
      { path: 'images', element: <SuspenseWrapper><ImagesPage /></SuspenseWrapper> },
      { path: 'images/:id', element: <SuspenseWrapper><ImageDetailPage /></SuspenseWrapper> },
      { path: 'volumes', element: <SuspenseWrapper><VolumesPage /></SuspenseWrapper> },
      { path: 'networks', element: <SuspenseWrapper><NetworksPage /></SuspenseWrapper> },
      { path: 'networks/:id', element: <SuspenseWrapper><NetworkDetailPage /></SuspenseWrapper> },
      { path: 'stacks', element: <SuspenseWrapper><StacksPage /></SuspenseWrapper> },
      { path: 'stacks/:name', element: <SuspenseWrapper><StackDetailPage /></SuspenseWrapper> },
      { path: 'terminal', element: <SuspenseWrapper><TerminalPage /></SuspenseWrapper> },
      { path: 'logs', element: <SuspenseWrapper><LogsPage /></SuspenseWrapper> },
      { path: 'environments', element: <SuspenseWrapper><EnvironmentsPage /></SuspenseWrapper> },
      { path: 'environments/:id', element: <SuspenseWrapper><EnvironmentDetailPage /></SuspenseWrapper> },
      { path: 'blueprints', element: <SuspenseWrapper><BlueprintsPage /></SuspenseWrapper> },
      { path: 'git', element: <SuspenseWrapper><GitPage /></SuspenseWrapper> },
      { path: 'reconciler', element: <SuspenseWrapper><ReconcilerPage /></SuspenseWrapper> },
      { path: 'activity', element: <SuspenseWrapper><ActivityPage /></SuspenseWrapper> },
      { path: 'audit', element: <SuspenseWrapper><AuditPage /></SuspenseWrapper> },
      { path: 'store', element: <SuspenseWrapper><StorePage /></SuspenseWrapper> },
      { path: 'docker', element: <SuspenseWrapper><DockerPage /></SuspenseWrapper> },
      // Kubernetes routes
      { path: 'pods', element: <SuspenseWrapper><PodsPage /></SuspenseWrapper> },
      { path: 'pods/:namespace/:name', element: <SuspenseWrapper><PodDetailPage /></SuspenseWrapper> },
      { path: 'deployments', element: <SuspenseWrapper><DeploymentsPage /></SuspenseWrapper> },
      { path: 'deployments/:namespace/:name', element: <SuspenseWrapper><DeploymentDetailPage /></SuspenseWrapper> },
      { path: 'k8s-services', element: <SuspenseWrapper><K8sServicesPage /></SuspenseWrapper> },
      { path: 'namespaces', element: <SuspenseWrapper><NamespacesPage /></SuspenseWrapper> },
      { path: 'workflows', element: <SuspenseWrapper><WorkflowsPage /></SuspenseWrapper> },
      { path: 'workflows/:id', element: <SuspenseWrapper><WorkflowEditorPage /></SuspenseWrapper> },
      { path: 'workflows/:id/runs', element: <SuspenseWrapper><WorkflowRunsPage /></SuspenseWrapper> },
      { path: 'security', element: <SuspenseWrapper><SecurityPage /></SuspenseWrapper> },
      { path: 'settings', element: <SuspenseWrapper><SettingsPage /></SuspenseWrapper> },
      { path: 'notifications', element: <SuspenseWrapper><NotificationsPage /></SuspenseWrapper> },
    ],
  },
];

export const router = createBrowserRouter(routes);

