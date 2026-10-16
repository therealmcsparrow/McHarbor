// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import {
  backupsGroupIcons,
  backupsGroupLabelKey,
  backupsTabDescriptionKeys,
  groupForBackupsTab,
  type BackupsTabId,
} from './backups-tab-groups';

type BackupsSectionMetaProps = {
  activeTab: BackupsTabId;
  /** The environment the page is scoped to, when one is selected. */
  environmentName?: string;
};

export function BackupsSectionMeta({ activeTab, environmentName }: BackupsSectionMetaProps) {
  const { t } = useTranslation('containers');
  const group = groupForBackupsTab(activeTab);
  const GroupIcon = backupsGroupIcons[group];

  return (
    <div className="mt-1 flex flex-wrap items-center gap-1.5">
      <Badge variant="secondary" className="gap-1 px-1.5 py-0 text-[10px]">
        <GroupIcon className="size-3" />
        {t(backupsGroupLabelKey(group))}
      </Badge>
      {environmentName && (
        <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
          {environmentName}
        </Badge>
      )}
      <span className="min-w-0 text-xs text-muted-foreground">{t(backupsTabDescriptionKeys[activeTab])}</span>
    </div>
  );
}
