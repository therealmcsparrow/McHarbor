// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconFileText, IconHistory, IconHistoryToggle, IconListDetails } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { formatDate } from '@resources/utils/format';
import type { ContainerActivityItem } from '../hooks/useContainerActivity';

type ContainerActivityListProps = {
  items: ContainerActivityItem[];
};

const SOURCE_ICONS = {
  logs: IconFileText,
  history: IconHistory,
  changes: IconHistoryToggle,
  processes: IconListDetails,
};

export function ContainerActivityList({ items }: ContainerActivityListProps) {
  const { t } = useTranslation('containers');
  return (
    <ol className="relative space-y-3 before:absolute before:inset-y-4 before:left-5 before:w-px before:bg-border">
      {items.map((item) => {
        const Icon = SOURCE_ICONS[item.source];
        return (
          <li key={item.id} className="relative flex gap-3">
            <span className="mc-glass z-10 flex size-10 shrink-0 items-center justify-center text-muted-foreground">
              <Icon className="size-4" />
            </span>
            <div className="mc-glass min-w-0 flex-1 rounded-lg p-4">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">{t(`activity.source.${item.source}`)}</Badge>
                <Badge
                  variant={
                    item.severity === 'error' ? 'destructive' : item.severity === 'warn' ? 'warning' : 'secondary'
                  }
                >
                  {t(`activity.severity.${item.severity}`)}
                </Badge>
                <time dateTime={item.timestamp} className="ml-auto text-xs text-muted-foreground">
                  {formatDate(item.timestamp)}
                </time>
              </div>
              <p className="mt-2 text-sm font-medium break-words text-foreground">{item.title}</p>
              {item.description ? (
                <p className="mt-1 text-xs break-words text-muted-foreground">{item.description}</p>
              ) : null}
            </div>
          </li>
        );
      })}
    </ol>
  );
}
