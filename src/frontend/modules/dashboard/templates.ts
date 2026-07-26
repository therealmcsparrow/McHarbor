// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconLayoutDashboard, IconLayoutGrid, IconHome, IconActivity, IconServer } from '@tabler/icons-react';

export type DashboardTemplateId = 'homelab' | 'devops' | 'production' | 'monitoring' | 'starter';

export type DashboardTemplateWidget = {
  typeId: string;
  span: number;
};

export type DashboardTemplate = {
  id: DashboardTemplateId;
  nameKey: string;
  descriptionKey: string;
  iconName: 'home' | 'activity' | 'server' | 'layout-grid' | 'layout-dashboard';
  widgets: DashboardTemplateWidget[];
};

export const DASHBOARD_TEMPLATES: DashboardTemplate[] = [
  {
    id: 'homelab',
    nameKey: 'dashboard.template.homelab.name',
    descriptionKey: 'dashboard.template.homelab.description',
    iconName: 'home',
    widgets: [
      { typeId: 'container-list', span: 6 },
      { typeId: 'host-info', span: 6 },
      { typeId: 'resource-summary', span: 4 },
      { typeId: 'quick-actions', span: 4 },
      { typeId: 'activity-feed', span: 4 },
    ],
  },
  {
    id: 'devops',
    nameKey: 'dashboard.template.devops.name',
    descriptionKey: 'dashboard.template.devops.description',
    iconName: 'activity',
    widgets: [
      { typeId: 'workflow-runs', span: 6 },
      { typeId: 'alert-summary', span: 3 },
      { typeId: 'deployment-status', span: 3 },
      { typeId: 'container-list', span: 6 },
      { typeId: 'vulnerability-summary', span: 6 },
    ],
  },
  {
    id: 'production',
    nameKey: 'dashboard.template.production.name',
    descriptionKey: 'dashboard.template.production.description',
    iconName: 'server',
    widgets: [
      { typeId: 'resource-donut', span: 3 },
      { typeId: 'alert-summary', span: 3 },
      { typeId: 'vulnerability-summary', span: 3 },
      { typeId: 'storage-breakdown', span: 3 },
      { typeId: 'host-info', span: 6 },
      { typeId: 'container-list', span: 6 },
    ],
  },
  {
    id: 'monitoring',
    nameKey: 'dashboard.template.monitoring.name',
    descriptionKey: 'dashboard.template.monitoring.description',
    iconName: 'layout-grid',
    widgets: [
      { typeId: 'metric-chart', span: 6 },
      { typeId: 'metric-chart', span: 6 },
      { typeId: 'top-consumers', span: 4 },
      { typeId: 'resource-donut', span: 4 },
      { typeId: 'restart-tracker', span: 4 },
    ],
  },
  {
    id: 'starter',
    nameKey: 'dashboard.template.starter.name',
    descriptionKey: 'dashboard.template.starter.description',
    iconName: 'layout-dashboard',
    widgets: [
      { typeId: 'container-list', span: 12 },
    ],
  },
];

export function getTemplateIcon(name: DashboardTemplate['iconName']) {
  switch (name) {
    case 'home':
      return IconHome;
    case 'activity':
      return IconActivity;
    case 'server':
      return IconServer;
    case 'layout-grid':
      return IconLayoutGrid;
    case 'layout-dashboard':
    default:
      return IconLayoutDashboard;
  }
}
