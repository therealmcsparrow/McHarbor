// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Navigate, type RouteObject } from 'react-router';
import { pageRoute } from './routeHelpers';
import * as P from './lazyPages';

const data = { scroll: 'data' } as const;
const page = { scroll: 'page' } as const;
const dataFlush = { scroll: 'data', padded: false } as const;
const pageFlush = { scroll: 'page', padded: false } as const;

/** Children of the AppLayout route, in matching order. */
export const appRoutes: RouteObject[] = [
  pageRoute('dashboard', P.DashboardPage, dataFlush),
  pageRoute('containers', P.ContainersPage, data),
  pageRoute('containers/:id', P.ContainerDetailPage, { scroll: 'data', padded: false, sidebar: true }),
  pageRoute('containers/:id/edit', P.ContainerEditPage, pageFlush),
  pageRoute('images', P.ImagesPage, data),
  pageRoute('images/:id', P.ImageDetailPage, pageFlush),
  pageRoute('volumes', P.VolumesPage, data),
  // The vault moved from Docker to Security. Kept as a redirect rather
  // than removed: /secrets shipped in a build, so bookmarks and any link
  // written against it still land in the right place.
  { path: 'secrets', element: <Navigate to="/security?tab=secrets" replace /> },
  pageRoute('volumes/:name', P.VolumeDetailPage, pageFlush),
  pageRoute('networks', P.NetworksPage, data),
  pageRoute('networks/:id', P.NetworkDetailPage, pageFlush),
  pageRoute('stacks', P.StacksPage, data),
  pageRoute('stacks/:name', P.StackDetailPage, dataFlush),
  pageRoute('terminal', P.TerminalPage, data),
  pageRoute('backups', P.BackupsPage, page),
  pageRoute('logs', P.LogsPage, data),
  pageRoute('environments', P.EnvironmentsPage, data),
  pageRoute('environments/:id', P.EnvironmentDetailPage, pageFlush),
  pageRoute('git', P.GitPage, data),
  pageRoute('gitops', P.GitOpsPage, data),
  pageRoute('reconciler', P.ReconcilerPage, data),
  pageRoute('reconciler/report', P.DriftReportPage, data),
  pageRoute('reconciler/proposals', P.ProposalsPage, data),
  pageRoute('reconciler/sources', P.SourcesPage, data),
  pageRoute('incidents', P.IncidentsPage, data),
  pageRoute('runbooks', P.RunbooksPage, data),
  pageRoute('runbooks/:id', P.RunbookEditRedirect, page),
  pageRoute('incidents/findings', P.FindingsPage, data),
  pageRoute('incidents/:id', P.IncidentDetailPage),
  pageRoute('activity', P.ActivityPage, data),
  pageRoute('lifecycle', P.LifecycleLogPage, data),
  pageRoute('audit', P.AuditPage, data),
  pageRoute('enterprise', P.EnterpriseOverviewPage, page),
  pageRoute('enterprise/ha', P.HighAvailabilityPage, page),
  pageRoute('enterprise/support-bundle', P.SupportBundlePage, page),
  pageRoute('enterprise/diagnostics', P.DiagnosticsPage, page),
  pageRoute('enterprise/state-inventory', P.StateInventoryPage, page),
  pageRoute('enterprise/compliance', P.CompliancePage, page),
  pageRoute('enterprise/compliance/packs', P.CompliancePacksPage, page),
  pageRoute('enterprise/environment-variables', P.EnvReferencePage, page),
  pageRoute('store', P.StorePage, data),
  pageRoute('docker', P.DockerPage, page),
  pageRoute('workflows', P.WorkflowsPage, data),
  pageRoute('workflows/custom-nodes', P.CustomNodesPage, data),
  pageRoute('workflows/:id', P.WorkflowEditorPage, dataFlush),
  pageRoute('workflows/:id/runs', P.WorkflowRunsPage, data),
  // Plugin pages — single static wildcard that resolves to the
  // installed plugin's UMD bundle via PluginPageHost. The host
  // component owns 404 handling for unknown slugs.
  pageRoute('plugins/:slug/*', P.PluginPageHost, data),
  pageRoute('security', P.SecurityPage, page),
  pageRoute('security/users/:id', P.UserDetailPage, page),
  pageRoute('security/groups/:id', P.GroupDetailPage, page),
  pageRoute('security/api-keys/:id', P.APIKeyDetailPage, page),
  pageRoute('security/scans', P.ScanHistoryPage, page),
  pageRoute('security/policies', P.PoliciesPage, page),
  pageRoute('settings', P.SettingsPage, page),
  pageRoute('notifications', P.NotificationsPage, data),
  pageRoute('profile', P.ProfilePage, page),
  pageRoute('device', P.DeviceApprovePage, { scroll: 'page', onPhone: true }),
];
