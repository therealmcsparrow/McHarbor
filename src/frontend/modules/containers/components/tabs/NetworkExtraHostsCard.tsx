// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';

type NetworkExtraHostsCardProps = {
  editing: boolean;
  extraHostKeys: string[];
  extraHosts: string[];
  onAddExtraHost: () => void;
  onExtraHostChange: (index: number, value: string) => void;
  onRemoveExtraHost: (index: number) => void;
};

export function NetworkExtraHostsCard({
  editing,
  extraHostKeys,
  extraHosts,
  onAddExtraHost,
  onExtraHostChange,
  onRemoveExtraHost,
}: NetworkExtraHostsCardProps) {
  const { t } = useTranslation('containers');
  return (
    <div className="mc-glass rounded-lg p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('networkTab.extraHosts')}
      </h3>
      {editing ? (
        <div className="space-y-2">
          {extraHosts.map((host, index) => (
            <div key={extraHostKeys[index]} className="flex gap-2">
              <input
                type="text"
                value={host}
                onChange={(changeEvent) => onExtraHostChange(index, changeEvent.target.value)}
                placeholder={t('placeholders.extraHost')}
                className="mc-control flex-1 rounded-md border border-border bg-muted px-2 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
              />
              <Button
                variant="ghost"
                size="icon"
                onClick={() => onRemoveExtraHost(index)}
                aria-label={t('network.removeHost')}
                className="size-7 text-muted-foreground hover:text-danger"
              >
                <IconTrash className="size-3.5" />
              </Button>
            </div>
          ))}
          <Button variant="outline" size="sm" onClick={onAddExtraHost} className="mt-1">
            <IconPlus className="mr-1 size-3.5" />
            {t('networkTab.addExtraHost')}
          </Button>
        </div>
      ) : extraHosts.length > 0 ? (
        <div className="space-y-1">
          {extraHosts.map((host) => (
            <div key={host} className="rounded-md bg-muted px-3 py-1.5 font-mono text-xs text-foreground">
              {host}
            </div>
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">{t('networkTab.noExtraHosts')}</p>
      )}
    </div>
  );
}
