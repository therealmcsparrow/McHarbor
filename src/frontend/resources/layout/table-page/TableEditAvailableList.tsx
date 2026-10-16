// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';

type TableEditAvailableListProps = {
  ids: string[];
  labels: Record<string, string>;
  groups: Record<string, string | undefined>;
  onShow: (id: string) => void;
};

export function TableEditAvailableList({ ids, labels, groups, onShow }: TableEditAvailableListProps) {
  const { t } = useTranslation('common');
  const [query, setQuery] = useState('');

  const grouped = useMemo(() => {
    const needle = query.trim().toLowerCase();
    const map = new Map<string, string[]>();
    for (const id of ids) {
      const label = labels[id] ?? id;
      if (needle && !label.toLowerCase().includes(needle)) continue;
      const group = groups[id] ?? t('tableViews.groupGeneral');
      map.set(group, [...(map.get(group) ?? []), id]);
    }
    return [...map.entries()];
  }, [groups, ids, labels, query, t]);

  return (
    <div className="flex flex-col gap-3">
      <Input
        value={query}
        onChange={(evt) => setQuery(evt.target.value)}
        placeholder={t('tableViews.searchColumns')}
        aria-label={t('tableViews.searchColumns')}
        variant="outline"
      />
      {grouped.length === 0 && <p className="text-sm text-muted-foreground">{t('tableViews.noAvailable')}</p>}
      {grouped.map(([group, groupIds]) => (
        <section key={group} className="flex flex-col gap-1">
          <h3 className="text-xs font-medium text-muted-foreground uppercase">{group}</h3>
          {groupIds.map((id) => (
            <div key={id} className="flex items-center gap-2 rounded-md px-2 py-1 hover:bg-muted/50">
              <span className="min-w-0 flex-1 truncate text-sm text-foreground">{labels[id] ?? id}</span>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('tableViews.addColumn', { column: labels[id] ?? id })}
                onClick={() => onShow(id)}
              >
                <IconPlus className="size-4" />
              </Button>
            </div>
          ))}
        </section>
      ))}
    </div>
  );
}
