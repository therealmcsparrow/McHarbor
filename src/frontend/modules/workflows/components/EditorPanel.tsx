// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconSettings,
  IconHelpCircle,
  IconBug,
  IconAlertTriangle,
} from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Button } from '@resources/components/ui/Button';
import { NODE_DEFINITION_MAP } from '../nodes';
import type { CanvasNode } from '../types';
import { NodeConfigPanel } from './NodeConfigPanel';
import { HelpTab } from './HelpTab';
import { DebugTab } from './DebugTab';
import type { DebugEntry } from './DebugTab';
import { ErrorTab } from './ErrorTab';
import type { ErrorEntry } from './ErrorTab';

type EditorPanelProps = {
  selectedNode: CanvasNode | null;
  debugMessages: DebugEntry[];
  errors: ErrorEntry[];
  onClearDebug: () => void;
  onClearErrors: () => void;
};

type TabId = 'settings' | 'help' | 'debug' | 'errors';

export function EditorPanel({ selectedNode, debugMessages, errors, onClearDebug, onClearErrors }: EditorPanelProps) {
  const { t } = useTranslation('common');
  const [activeTab, setActiveTab] = useState<TabId>('settings');

  const TABS: { id: TabId; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
    { id: 'settings', label: t('workflows.settingsTab'), icon: IconSettings },
    { id: 'help', label: t('workflows.helpTab'), icon: IconHelpCircle },
    { id: 'debug', label: t('workflows.debugTab'), icon: IconBug },
    { id: 'errors', label: t('workflows.errorsTab'), icon: IconAlertTriangle },
  ];
  const definition = selectedNode ? NODE_DEFINITION_MAP[selectedNode.action] : undefined;

  return (
    <div className="flex h-full w-80 flex-col border-l border-border bg-card">
      {/* Tab bar */}
      <div className="flex border-b border-border">
        {TABS.map((tab) => {
          const Icon = tab.icon;
          const isActive = activeTab === tab.id;
          const hasIndicator = (tab.id === 'debug' && debugMessages.length > 0) ||
                               (tab.id === 'errors' && errors.length > 0);
          return (
            <Button
              key={tab.id}
              variant="ghost"
              onClick={() => setActiveTab(tab.id)}
              className={cn(
                'relative flex flex-1 items-center justify-center gap-1 rounded-none py-2.5 text-[11px] h-auto',
                isActive
                  ? 'text-foreground border-b-2 border-primary'
                  : 'text-muted-foreground hover:text-foreground',
                tab.id === 'errors' && errors.length > 0 && !isActive && 'text-red-400',
              )}
            >
              <Icon className="size-3.5" />
              <span className="hidden sm:inline">{tab.label}</span>
              {hasIndicator && !isActive && (
                <span className={cn(
                  'absolute right-1.5 top-1.5 size-1.5 rounded-full',
                  tab.id === 'errors' ? 'bg-red-400' : 'bg-blue-400',
                )} />
              )}
            </Button>
          );
        })}
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto">
        {activeTab === 'settings' && (
          selectedNode ? (
            <NodeConfigPanel node={selectedNode} />
          ) : (
            <div className="flex flex-col items-center justify-center py-16 px-4">
              <IconSettings className="size-8 text-muted-foreground/20" />
              <p className="mt-3 text-xs text-muted-foreground">{t('workflows.selectNodeToConfigure')}</p>
            </div>
          )
        )}
        {activeTab === 'help' && <HelpTab definition={definition} />}
        {activeTab === 'debug' && <DebugTab messages={debugMessages} onClear={onClearDebug} />}
        {activeTab === 'errors' && <ErrorTab errors={errors} onClear={onClearErrors} />}
      </div>
    </div>
  );
}

