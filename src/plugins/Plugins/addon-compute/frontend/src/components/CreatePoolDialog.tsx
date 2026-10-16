// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useHostEnvironment } from '../hooks/useHostEnvironment';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { COMPUTE_PERM } from '../permissions';
import { useCreatePool, usePoolBackends } from '../hooks/useCompute';
import { formatBytes } from '../format';
import { BackendOption, sortBackends } from './BackendOption';
import type { ComputeBackend } from '../types';

/**
 * Create a storage pool.
 *
 * The dialog's real job is the recommendation. Choosing `dir` when the host has
 * ZFS is the difference between clones that take three minutes and clones that
 * take no time at all, and an operator picking from a flat list of four words
 * has no way to know that. So detected backends are ordered with the instant
 * ones first, marked, and the reason is stated in plain language.
 *
 * Unavailable backends are shown too, greyed, with what to install — an
 * operator who cannot see that btrfs exists has no way to learn that installing
 * btrfs-progs would unlock it.
 */

interface CreatePoolDialogProps {
  environments: { id: string; name: string }[];
  defaultEnvironmentId?: string;
  onClose: () => void;
  onCreated?: (poolId: string) => void;
}

export function CreatePoolDialog({ environments, defaultEnvironmentId, onClose, onCreated }: CreatePoolDialogProps) {
  const { t } = useTranslation('compute');

  // The host's topbar selection comes before whatever sorted first, which is
  // what this used to offer regardless of what the user had chosen.
  const hostEnvironmentId = useHostEnvironment();
  const [environmentId, setEnvironmentId] = useState(
    defaultEnvironmentId || hostEnvironmentId || environments[0]?.id || '',
  );
  const [poolName, setPoolName] = useState('');
  const [backend, setBackend] = useState<ComputeBackend | ''>('');
  const [target, setTarget] = useState('');
  const [isDefault, setIsDefault] = useState(true);

  const { data: backends, isLoading } = usePoolBackends(environmentId || undefined);
  const create = useCreatePool();

  // Instant backends first, then available ones, then the rest. The order is
  // the recommendation.
  const ordered = useMemo(() => sortBackends(backends ?? []), [backends]);

  // Preselect the best available backend once detection lands, so the common
  // case is "type a name and confirm".
  if (!backend && ordered.length > 0) {
    const best = ordered.find((entry) => entry.available);
    if (best) {
      setBackend(best.backend);
      const candidate = best.candidates?.[0];
      if (candidate) setTarget(candidate.target);
    }
  }

  const selected = ordered.find((entry) => entry.backend === backend);
  const canSubmit = Boolean(environmentId && poolName.trim() && backend && target.trim());

  const submit = () => {
    if (!canSubmit) return;
    create.mutate(
      { environmentId, name: poolName.trim(), backend, target: target.trim(), isDefault },
      {
        onSuccess: (result) => {
          const poolId = (result as { id?: string } | undefined)?.id;
          if (poolId && onCreated) onCreated(poolId);
          onClose();
        },
      },
    );
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('pools.create.title')}</DialogTitle>
          <DialogDescription>{t('pools.subtitle')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div>
            <Label>{t('pools.create.environment')}</Label>
            <Select
              value={environmentId}
              ariaLabel={t('pools.create.environment')}
              options={environments.map((env) => ({ value: env.id, label: env.name }))}
              onChange={(value) => {
                setEnvironmentId(value);
                // Backends are per host, so a host change invalidates both.
                setBackend('');
                setTarget('');
              }}
            />
          </div>

          <div>
            <Label htmlFor="pool-name">{t('pools.create.name')}</Label>
            <Input id="pool-name" value={poolName} onChange={(ev) => setPoolName(ev.target.value)} autoComplete="off" />
          </div>

          <fieldset>
            <legend className="mb-2 text-sm font-medium text-foreground">{t('pools.create.backend')}</legend>

            {isLoading ? (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Spinner className="h-4 w-4" />
                {t('actions.refresh')}
              </div>
            ) : (
              <div className="space-y-2">
                {ordered.map((entry) => (
                  <BackendOption
                    key={entry.backend}
                    entry={entry}
                    selected={backend === entry.backend}
                    onSelect={() => {
                      setBackend(entry.backend);
                      const candidate = entry.candidates?.[0];
                      setTarget(candidate ? candidate.target : '');
                    }}
                  />
                ))}
              </div>
            )}
          </fieldset>

          {backend && (
            <div>
              <Label>{t('pools.create.target')}</Label>
              {/*
                A picker when the host offered candidates, a free-text field
                when it did not. Making the operator retype a dataset name the
                host already listed is how typos become failed creates.

                A directory is the exception: any writable path is valid, the
                candidate is only a suggestion, and locking the field to it
                would make "put the disks over there" unreachable.
              */}
              {backend !== 'dir' && selected?.candidates && selected.candidates.length > 0 ? (
                <Select
                  value={target}
                  ariaLabel={t('pools.create.target')}
                  options={selected.candidates.map((candidate) => ({
                    value: candidate.target,
                    label: candidate.freeBytes
                      ? `${candidate.target} — ${formatBytes(candidate.freeBytes)} free`
                      : candidate.target,
                  }))}
                  onChange={setTarget}
                />
              ) : (
                <Input
                  id="pool-target"
                  value={target}
                  onChange={(ev) => setTarget(ev.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                />
              )}
              <p className="mt-1 text-xs text-muted-foreground">{t(`pools.create.targetHint.${backend}`)}</p>
            </div>
          )}

          <div className="flex items-center justify-between gap-3">
            <Label htmlFor="pool-default" className="text-sm font-normal">
              {t('pools.create.makeDefault')}
            </Label>
            <Switch id="pool-default" checked={isDefault} onCheckedChange={setIsDefault} />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('actions.cancel')}
          </Button>
          <Button permission={COMPUTE_PERM.MANAGE} onClick={submit} disabled={!canSubmit || create.isPending}>
            {create.isPending && <Spinner className="mr-2 h-4 w-4" />}
            {t('pools.create.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
