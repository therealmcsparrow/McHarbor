// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { useApplySecret, usePruneSecretReferences, useSecretImpact } from '@resources/hooks/useSecrets';
import { SecretImpactRow } from './SecretImpactRow';
import type { Secret, SecretApplyResult, SecretApplyStatus, SecretImpactResource } from '../types';

type SecretImpactDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  secret: Secret | null;
};

const resourceKey = (resource: { resourceType: string; resourceId: string }) =>
  `${resource.resourceType}/${resource.resourceId}`;

export function SecretImpactDialog({ open: isOpen, onOpenChange, secret }: SecretImpactDialogProps) {
  const { t } = useTranslation('secrets');
  const { data: impact, isLoading } = useSecretImpact(isOpen ? (secret?.id ?? null) : null);
  const applySecret = useApplySecret();
  const pruneReferences = usePruneSecretReferences();

  const [selected, setSelected] = useState<string[]>([]);
  const [results, setResults] = useState<SecretApplyResult[]>([]);

  const resources = useMemo<SecretImpactResource[]>(() => impact?.resources ?? [], [impact]);

  const [seeded, setSeeded] = useState<{ isOpen: boolean; resources: SecretImpactResource[] | null }>({
    isOpen: false,
    resources: null,
  });
  if (seeded.isOpen !== isOpen || seeded.resources !== resources) {
    setSeeded({ isOpen, resources });
    if (!isOpen) {
      setResults([]);
    } else {
      setSelected(
        resources.filter((resource) => resource.stale && resource.supported && !resource.missing).map(resourceKey),
      );
    }
  }

  const statusByResource = useMemo(() => {
    const map: Record<string, SecretApplyStatus> = {};
    for (const result of results) map[resourceKey(result)] = result.status;
    return map;
  }, [results]);

  const toggle = (key: string, next: boolean) => {
    setSelected((current) => (next ? [...current, key] : current.filter((entry) => entry !== key)));
  };

  const handleApply = () => {
    if (!secret || selected.length === 0) return;
    applySecret.mutate(
      {
        id: secret.id,
        resources: resources
          .filter((resource) => selected.includes(resourceKey(resource)))
          .map((resource) => ({
            resourceType: resource.resourceType,
            resourceId: resource.resourceId,
          })),
      },
      { onSuccess: setResults },
    );
  };

  const failed = results.some((result) => result.status === 'failed');
  const prunableCount = impact?.prunableCount ?? 0;

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('impact.title', { name: secret?.name ?? '' })}</DialogTitle>
          <DialogDescription>
            {t('impact.description', { version: impact?.version ?? secret?.version ?? 0 })}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          {isLoading ? (
            <div className="flex justify-center py-6">
              <Spinner />
            </div>
          ) : resources.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('impact.empty')}</p>
          ) : (
            <>
              {failed && (
                <Alert variant="destructive" title={t('apply.failedTitle')}>
                  {t('apply.failedBody')}
                </Alert>
              )}
              {prunableCount > 0 && (
                <Alert variant="info" title={t('impact.missingTitle', { count: prunableCount })}>
                  <div className="space-y-2">
                    <p>{t('impact.missingBody')}</p>
                    <Button
                      permission={PERM.SECRETS_MANAGE}
                      size="sm"
                      variant="outline"
                      disabled={pruneReferences.isPending}
                      onClick={() => secret && pruneReferences.mutate({ id: secret.id })}
                    >
                      {t('impact.prune', { count: prunableCount })}
                    </Button>
                  </div>
                </Alert>
              )}

              {!failed && (impact?.staleCount ?? 0) > 0 && (
                <Alert variant="warning" title={t('impact.staleTitle', { count: impact?.staleCount ?? 0 })}>
                  {t('impact.staleBody')}
                </Alert>
              )}

              <ul className="max-h-80 space-y-2 overflow-auto">
                {resources.map((resource) => {
                  const key = resourceKey(resource);
                  return (
                    <SecretImpactRow
                      key={key}
                      resource={resource}
                      version={impact?.version ?? 0}
                      selected={selected.includes(key)}
                      onSelectedChange={(next) => toggle(key, next)}
                      status={statusByResource[key]}
                      disabled={applySecret.isPending}
                    />
                  );
                })}
              </ul>

              <p className="text-xs text-muted-foreground">{t('apply.warning')}</p>
            </>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.close', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.SECRETS_APPLY}
            onClick={handleApply}
            disabled={selected.length === 0 || applySecret.isPending}
          >
            {t('apply.submit', { count: selected.length })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
