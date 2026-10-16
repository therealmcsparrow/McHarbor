// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { lazy } from 'react';

export const RootPage = lazy(() => import('@core/auth/RootPage'));
export const LoginPage = lazy(() => import('@core/auth/LoginPage'));
export const SetupPage = lazy(() => import('@core/auth/SetupPage'));
export const ForcePasswordChangePage = lazy(() => import('@core/auth/ForcePasswordChangePage'));
export const PasswordResetRequestPage = lazy(() => import('@core/auth/PasswordResetRequestPage'));
export const PasswordResetConsumePage = lazy(() => import('@core/auth/PasswordResetConsumePage'));
export const DatabaseSetupPage = lazy(() => import('@modules/settings/database/pages/DatabaseSetupPage'));
export const MFAEnrollmentRequiredPage = lazy(() => import('@modules/profile/pages/MFAEnrollmentRequiredPage'));

export const OnCallAlertsPage = lazy(() => import('@modules/oncall/pages/AlertsPage'));
export const OnCallFleetPage = lazy(() => import('@modules/oncall/pages/FleetPage'));
export const OnCallContainerPage = lazy(() => import('@modules/oncall/pages/ContainerPage'));
export const OnCallLogsPage = lazy(() => import('@modules/oncall/pages/LogsPage'));
export const OnCallApprovalsPage = lazy(() => import('@modules/oncall/pages/ApprovalsPage'));
export const OnCallDevicePage = lazy(() => import('@modules/oncall/pages/DevicePage'));
export const OnCallRunbookPage = lazy(() => import('@modules/oncall/pages/RunbookPage'));

export const RunbooksPage = lazy(() => import('@modules/runbooks/pages/RunbooksPage'));
export const RunbookEditRedirect = lazy(() => import('@modules/runbooks/pages/RunbookEditRedirect'));
export const DeviceApprovePage = lazy(() => import('@modules/security/pages/DeviceApprovePage'));
export const DashboardPage = lazy(() => import('@modules/dashboard/pages/DashboardPage'));
export const ContainersPage = lazy(() => import('@modules/containers/pages/ContainersPage'));
export const ContainerDetailPage = lazy(() => import('@modules/containers/pages/ContainerDetailPage'));
export const ContainerEditPage = lazy(() => import('@modules/containers/pages/ContainerEditPage'));
export const ImagesPage = lazy(() => import('@modules/images/pages/ImagesPage'));
export const ImageDetailPage = lazy(() => import('@modules/images/pages/ImageDetailPage'));
export const VolumesPage = lazy(() => import('@modules/volumes/pages/VolumesPage'));
export const VolumeDetailPage = lazy(() => import('@modules/volumes/pages/VolumeDetailPage'));
export const NetworksPage = lazy(() => import('@modules/networks/pages/NetworksPage'));
export const NetworkDetailPage = lazy(() => import('@modules/networks/pages/NetworkDetailPage'));
export const StacksPage = lazy(() => import('@modules/stacks/pages/StacksPage'));
export const StackDetailPage = lazy(() => import('@modules/stacks/pages/StackDetailPage'));
export const TerminalPage = lazy(() => import('@modules/terminal/pages/TerminalPage'));
export const LogsPage = lazy(() => import('@modules/logs/pages/LogsPage'));
export const BackupsPage = lazy(() => import('@modules/containers/pages/BackupsPage'));
export const EnvironmentsPage = lazy(() => import('@modules/environments/pages/EnvironmentsPage'));
export const EnvironmentDetailPage = lazy(() => import('@modules/environments/pages/EnvironmentDetailPage'));
export const GitPage = lazy(() => import('@modules/git/pages/GitPage'));
export const GitOpsPage = lazy(() =>
  import('@modules/gitops/pages/GitOpsPage').then((m) => ({ default: m.GitOpsPage })),
);
export const DriftReportPage = lazy(() => import('@modules/reconciler/pages/DriftReportPage'));
export const ProposalsPage = lazy(() => import('@modules/reconciler/pages/ProposalsPage'));
export const SourcesPage = lazy(() => import('@modules/reconciler/pages/SourcesPage'));
export const ReconcilerPage = lazy(() => import('@modules/reconciler/pages/ReconcilerPage'));
export const IncidentsPage = lazy(() => import('@modules/incidents/pages/IncidentsPage'));
export const IncidentDetailPage = lazy(() => import('@modules/incidents/pages/IncidentDetailPage'));
export const FindingsPage = lazy(() => import('@modules/incidents/pages/FindingsPage'));
export const ActivityPage = lazy(() => import('@modules/activity/pages/ActivityPage'));
export const LifecycleLogPage = lazy(() => import('@modules/lifecycle/pages/LifecycleLogPage'));
export const AuditPage = lazy(() => import('@modules/audit/pages/AuditPage'));

export const EnterpriseOverviewPage = lazy(() => import('@modules/enterprise/pages/EnterpriseOverviewPage'));
export const HighAvailabilityPage = lazy(() => import('@modules/enterprise/pages/HighAvailabilityPage'));
export const SupportBundlePage = lazy(() => import('@modules/enterprise/pages/SupportBundlePage'));
export const DiagnosticsPage = lazy(() => import('@modules/enterprise/pages/DiagnosticsPage'));
export const StateInventoryPage = lazy(() => import('@modules/enterprise/pages/StateInventoryPage'));
export const CompliancePage = lazy(() => import('@modules/enterprise/pages/CompliancePage'));
export const CompliancePacksPage = lazy(() => import('@modules/enterprise/pages/CompliancePacksPage'));
export const EnvReferencePage = lazy(() => import('@modules/enterprise/pages/EnvReferencePage'));

export const StorePage = lazy(() => import('@modules/appstore/pages/StorePage'));
export const SecurityPage = lazy(() => import('@modules/security/pages/SecurityPage'));
export const ScanHistoryPage = lazy(() => import('@modules/security/pages/ScanHistoryPage'));
export const UserDetailPage = lazy(() => import('@modules/security/pages/UserDetailPage'));
export const GroupDetailPage = lazy(() => import('@modules/security/pages/GroupDetailPage'));
export const APIKeyDetailPage = lazy(() => import('@modules/security/pages/APIKeyDetailPage'));
export const PoliciesPage = lazy(() => import('@modules/abac/pages/PoliciesPage'));
export const SettingsPage = lazy(() => import('@modules/settings/pages/SettingsPage'));
export const NotificationsPage = lazy(() => import('@modules/notifications/pages/NotificationsPage'));
export const ProfilePage = lazy(() => import('@modules/profile/pages/ProfilePage'));
export const WorkflowsPage = lazy(() => import('@modules/workflows/pages/WorkflowsPage'));
export const WorkflowEditorPage = lazy(() => import('@modules/workflows/pages/WorkflowEditorPage'));
export const WorkflowRunsPage = lazy(() => import('@modules/workflows/pages/WorkflowRunsPage'));
export const CustomNodesPage = lazy(() => import('@modules/workflows/pages/CustomNodesPage'));
export const DockerPage = lazy(() => import('@modules/docker/pages/DockerPage'));

// Single static route covers every installed plugin — PluginPageHost
// reads `:slug` from params and lazy-loads the matching UMD bundle.
export const PluginPageHost = lazy(() =>
  import('@core/plugins/PluginPageHost').then((m) => ({ default: m.PluginPageHost })),
);
