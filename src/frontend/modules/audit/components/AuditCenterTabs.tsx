// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconClipboardList,
  IconClockHour4,
  IconDownload,
  IconLock,
  IconRobot,
  IconShieldCheck,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { TabsContent, TabsList, TabsTrigger } from '@resources/components/ui/Tabs';
import { PERM } from '@core/auth/permissionKeys';
import { AuditDenialsPanel } from './AuditDenialsPanel';
import { AuditAITrailPanel } from './AuditAITrailPanel';
import { AuditRetentionPanel } from './AuditRetentionPanel';

export function AuditHeaderActions({ onVerify, onExport }: { onVerify: () => void; onExport: () => void }) {
  const { t } = useTranslation('common');
  return (
    <>
      <Button variant="outline" permission={PERM.AUDIT_VIEW} onClick={onVerify}>
        <IconShieldCheck className="size-4" />
        {t('verify.button', { ns: 'audit' })}
      </Button>
      <Button permission={PERM.AUDIT_EXPORT} onClick={onExport}>
        <IconDownload className="size-4" />
        {t('audit.exportButton')}
      </Button>
    </>
  );
}

export function AuditTabsList() {
  const { t } = useTranslation('common');
  return (
    <div className="shrink-0 overflow-x-auto">
      <TabsList className="min-w-max">
        <TabsTrigger value="events" className="gap-1.5">
          <IconClipboardList className="size-4" />
          {t('audit.tabEvents')}
        </TabsTrigger>
        <TabsTrigger value="denials" className="gap-1.5">
          <IconLock className="size-4" />
          {t('tabs.denials', { ns: 'audit' })}
        </TabsTrigger>
        <TabsTrigger value="ai" className="gap-1.5">
          <IconRobot className="size-4" />
          {t('tabs.ai', { ns: 'audit' })}
        </TabsTrigger>
        <TabsTrigger value="retention" className="gap-1.5">
          <IconClockHour4 className="size-4" />
          {t('tabs.retention', { ns: 'audit' })}
        </TabsTrigger>
        <TabsTrigger value="coverage" className="gap-1.5">
          <IconShieldCheck className="size-4" />
          {t('audit.tabCoverage')}
        </TabsTrigger>
      </TabsList>
    </div>
  );
}

export function AuditCenterTabsContent({
  envName,
}: {
  envName: (id: string | null, storedName?: string | null) => string;
}) {
  return (
    <>
      <TabsContent value="denials" className="min-h-0 flex-1 overflow-auto">
        <AuditDenialsPanel envName={envName} />
      </TabsContent>
      <TabsContent value="ai" className="min-h-0 flex-1 overflow-auto">
        <AuditAITrailPanel />
      </TabsContent>
      <TabsContent value="retention" className="min-h-0 flex-1 overflow-auto">
        <AuditRetentionPanel />
      </TabsContent>
    </>
  );
}
