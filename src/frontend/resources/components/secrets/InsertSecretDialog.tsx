// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconKey, IconSearch } from '@tabler/icons-react';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Spinner } from '@resources/components/ui/Spinner';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { useSecretsList } from '@resources/hooks/useSecretsList';
import { buildSecretTree } from './insert-secret-tree';
import { InsertSecretTree } from './InsertSecretTree';
import { secretReference, type Secret } from '@core/types/secrets';

type InsertSecretDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called with the `${secret:NAME}` reference, not the bare name. */
  onSelect: (reference: string) => void;
};

/**
 * The picker that opens on a right-click in a text field.
 *
 * It lists names and scopes and nothing else. There is no value here and no
 * way to ask for one — the list endpoint does not return values, and reading
 * one costs an approved elevation, a step-up confirmation and an audit entry
 * (see the Secrets page). What this dialog inserts is a *reference*; the
 * substitution happens server-side at deploy time.
 *
 * Application credentials (`system = 1`) are excluded. They belong to a host
 * module, are not referenced by name, and would be noise in a list whose whole
 * job is "which of my secrets did I mean?".
 */
export function InsertSecretDialog({ open: isOpen, onOpenChange, onSelect }: InsertSecretDialogProps) {
  const { t } = useTranslation('secrets');
  const { data: secrets = [], isLoading } = useSecretsList();
  const [query, setQuery] = useState('');
  const [collapsed, setCollapsed] = useState<string[]>([]);

  if (!isOpen && query !== '') setQuery('');

  const matches = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (needle === '') return secrets;
    return secrets.filter(
      (secret) =>
        secret.name.toLowerCase().includes(needle) ||
        (secret.description ?? '').toLowerCase().includes(needle) ||
        (secret.stackName ?? '').toLowerCase().includes(needle),
    );
  }, [secrets, query]);

  const tree = useMemo(() => buildSecretTree(matches), [matches]);

  const toggle = (key: string) =>
    setCollapsed((current) => (current.includes(key) ? current.filter((entry) => entry !== key) : [...current, key]));

  const choose = (secret: Secret) => {
    onSelect(secretReference(secret.name));
    onOpenChange(false);
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('insert.title')}</DialogTitle>
          <DialogDescription>{t('insert.description')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="relative">
            <IconSearch className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              variant="outline"
              value={query}
              onChange={(evt) => setQuery(evt.target.value)}
              placeholder={t('insert.searchPlaceholder')}
              className="pl-8"
              aria-label={t('insert.searchPlaceholder')}
            />
          </div>

          {isLoading ? (
            <div className="flex justify-center py-8">
              <Spinner />
            </div>
          ) : matches.length === 0 ? (
            <EmptyState
              icon={<IconKey className="size-5" />}
              title={secrets.length === 0 ? t('insert.emptyTitle') : t('insert.noMatchTitle')}
              description={secrets.length === 0 ? t('insert.emptyDescription') : t('insert.noMatchDescription')}
            />
          ) : (
            <InsertSecretTree
              nodes={tree}
              collapsed={collapsed}
              onToggle={toggle}
              onSelect={choose}
              forceExpanded={query.trim() !== ''}
            />
          )}

          <p className="text-xs text-muted-foreground">{t('insert.hint')}</p>
        </div>
      </DialogContent>
    </Dialog>
  );
}
