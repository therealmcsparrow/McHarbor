// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// StackDetailTabContent — what the stack tab panel shows.
//
// Logs and Compose stay mounted and hidden rather than unmounting, because
// both own a long-lived thing the user would lose on a tab switch: the log
// stream's position and backlog, and the Compose editor's unsaved buffer and
// cursor. Every other tab is cheap to rebuild and so is rendered on demand.

import type { StackDetail } from '@resources/hooks/useStacks';
import { OverviewTab } from './tabs/OverviewTab';
import { ServicesTab } from './tabs/ServicesTab';
import { LogsTab } from './tabs/LogsTab';
import { ComposeTab } from './tabs/ComposeTab';
import { EnvironmentTab } from './tabs/EnvironmentTab';
import { LayersTab } from './tabs/LayersTab';
import { WebhooksTab } from './tabs/WebhooksTab';

type StackDetailTabContentProps = {
  activeTab: string;
  stack: StackDetail;
  isManaged: boolean;
  locked: boolean;
  editing: boolean;
  description: string;
  onDescriptionChange: (next: string) => void;
};

export function StackDetailTabContent({
  activeTab,
  stack,
  isManaged,
  locked,
  editing,
  description,
  onDescriptionChange,
}: StackDetailTabContentProps) {
  return (
    <>
      {activeTab === 'overview' && (
        <OverviewTab
          stack={stack}
          editing={editing}
          description={description}
          onDescriptionChange={onDescriptionChange}
        />
      )}
      {activeTab === 'services' && <ServicesTab stackName={stack.name} />}
      {activeTab === 'layers' && <LayersTab stackName={stack.name} services={stack.services} />}

      <div className={activeTab !== 'logs' ? 'hidden' : 'flex min-h-0 flex-1 flex-col'}>
        <LogsTab stackName={stack.name} />
      </div>

      <div className={activeTab !== 'compose' ? 'hidden' : 'flex min-h-0 flex-1 flex-col'}>
        <ComposeTab stackName={stack.name} isManaged={isManaged} editing={editing && !locked} readOnly={locked} />
      </div>

      {activeTab === 'environment' && isManaged && (
        <EnvironmentTab stackName={stack.name} editing={editing && !locked} readOnly={locked} />
      )}
      {activeTab === 'webhooks' && isManaged && <WebhooksTab stackName={stack.name} />}
    </>
  );
}
