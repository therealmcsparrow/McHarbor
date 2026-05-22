// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconCpu,
  IconDatabase,
  IconFileText,
  IconFolder,
  IconInfoCircle,
  IconListDetails,
  IconNetwork,
  IconShield,
  IconTag,
  IconTerminal,
  IconVariable,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';

export const detailTabIDs = [
  'overview',
  'environment',
  'labels',
  'network',
  'mounts',
  'resources',
  'security',
  'logs',
  'terminal',
  'processes',
  'files',
] as const;

export type DetailTabId = (typeof detailTabIDs)[number];

const detailTabIcons: Record<DetailTabId, typeof IconInfoCircle> = {
  overview: IconInfoCircle,
  environment: IconVariable,
  labels: IconTag,
  network: IconNetwork,
  mounts: IconDatabase,
  resources: IconCpu,
  security: IconShield,
  logs: IconFileText,
  terminal: IconTerminal,
  processes: IconListDetails,
  files: IconFolder,
};

type ContainerDetailTabsProps = {
  activeTab: DetailTabId;
  onTabChange: (tab: DetailTabId) => void;
  labelFor: (tab: DetailTabId) => string;
};

export function ContainerDetailTabs({
  activeTab,
  onTabChange,
  labelFor,
}: ContainerDetailTabsProps) {
  return (
    <div className="border-b border-border bg-card px-5 py-3">
      <nav className="flex w-fit gap-x-1 rounded-lg bg-muted p-1">
        {detailTabIDs.map((tabId) => {
          const Icon = detailTabIcons[tabId];
          return (
            <Button
              key={tabId}
              variant="ghost"
              size="sm"
              className={cn(
                'inline-flex items-center gap-x-1.5 rounded-lg px-3.5 py-2 text-sm font-medium',
                activeTab === tabId
                  ? 'bg-card text-foreground shadow-sm hover:bg-card'
                  : 'bg-transparent text-muted-foreground hover:text-primary',
              )}
              onClick={() => onTabChange(tabId)}
            >
              <Icon className="size-4" />
              {labelFor(tabId)}
            </Button>
          );
        })}
      </nav>
    </div>
  );
}
