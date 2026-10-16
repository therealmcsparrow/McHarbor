// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { VM_PERM } from '../permissions';
import { useAddShare, useRemoveShare, useShareRoots, useShares } from '../hooks/useShares';
import { useHostFeature } from './HostFeatureGate';
import { VM_FEATURE } from '../hooks/useQEMUCapability';
import type { VM } from '../types';
import { SharesTable } from './SharesTable';

/**
 * A guest's virtiofs shares.
 *
 * Changes take effect at the next start, and the panel says so rather than
 * implying a live attach: detaching a vhost-user device from a guest that has
 * the filesystem mounted hangs every process inside it holding a file open.
 */
export function SharesPanel({ vm }: { vm: VM }) {
  const { t } = useTranslation('vm');
  const shares = useShares(vm.id);
  const roots = useShareRoots(vm.environmentId);
  const addShare = useAddShare(vm.id);
  const removeShare = useRemoveShare(vm.id);
  const feature = useHostFeature(vm.environmentId, VM_FEATURE.VIRTIOFS);

  const [adding, setAdding] = useState(false);
  const [tag, setTag] = useState('');
  const [root, setRoot] = useState('');
  const [subPath, setSubPath] = useState('');
  const [readOnly, setReadOnly] = useState(true);

  const availableRoots = roots.data?.roots ?? [];
  const canAdd = feature.supported && availableRoots.length > 0;

  function submit() {
    const base = root || availableRoots[0] || '';
    const trimmed = subPath.trim().replace(/^\/+/, '');
    const hostPath = trimmed ? `${base}/${trimmed}` : base;
    addShare.mutate(
      { tag: tag.trim(), hostPath, readOnly },
      {
        onSuccess: () => {
          setAdding(false);
          setTag('');
          setSubPath('');
        },
      },
    );
  }

  return (
    <section>
      <div className="mb-2 flex items-center justify-between gap-2">
        <div>
          <h3 className="font-medium text-foreground">{t('shares.title')}</h3>
          <p className="text-sm text-muted-foreground">{t('shares.description')}</p>
        </div>
        <Tooltip>
          <TooltipTrigger asChild>
            <span>
              <Button
                variant="outline"
                size="sm"
                permission={VM_PERM.PRIVILEGED}
                disabled={!canAdd}
                onClick={() => setAdding(true)}
              >
                <IconPlus className="mr-1.5 h-4 w-4" aria-hidden="true" />
                {t('shares.add')}
              </Button>
            </span>
          </TooltipTrigger>
          {!canAdd && (
            <TooltipContent className="max-w-[320px]">
              {feature.supported ? t('shares.noRoots') : feature.reason}
            </TooltipContent>
          )}
        </Tooltip>
      </div>

      {adding && (
        <div className="mb-3 space-y-3 rounded-lg border border-border bg-card p-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="space-y-1">
              <span className="text-sm text-muted-foreground">{t('shares.tag')}</span>
              <Input
                value={tag}
                onChange={(evt) => setTag(evt.target.value)}
                placeholder={t('shares.tagPlaceholder')}
              />
            </label>
            <label className="space-y-1">
              <span className="text-sm text-muted-foreground">{t('shares.root')}</span>
              <Select
                value={root || availableRoots[0] || ''}
                ariaLabel={t('shares.root')}
                options={availableRoots.map((entry) => ({ value: entry, label: entry }))}
                onChange={setRoot}
              />
            </label>
            <label className="space-y-1">
              <span className="text-sm text-muted-foreground">{t('shares.subPath')}</span>
              <Input
                value={subPath}
                onChange={(evt) => setSubPath(evt.target.value)}
                placeholder={t('shares.subPathPlaceholder')}
              />
            </label>
            <label className="space-y-1">
              <span className="text-sm text-muted-foreground">{t('shares.access')}</span>
              <Select
                value={readOnly ? 'ro' : 'rw'}
                ariaLabel={t('shares.access')}
                options={[
                  { value: 'ro', label: t('shares.readOnly') },
                  { value: 'rw', label: t('shares.readWrite') },
                ]}
                onChange={(value) => setReadOnly(value === 'ro')}
              />
            </label>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => setAdding(false)}>
              {t('actions.cancel', { ns: 'common' })}
            </Button>
            <Button
              size="sm"
              permission={VM_PERM.PRIVILEGED}
              disabled={!tag.trim() || addShare.isPending}
              onClick={submit}
            >
              {t('shares.add')}
            </Button>
          </div>
        </div>
      )}

      <SharesTable
        shares={shares.data ?? []}
        removing={removeShare.isPending}
        onRemove={(id) => removeShare.mutate(id)}
      />

      <p className="mt-2 text-sm text-muted-foreground">{t('shares.restartHint')}</p>
    </section>
  );
}
