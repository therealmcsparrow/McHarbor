// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { TablePage } from '@resources/layout/TablePage';
import { Button } from '@resources/components/ui/Button';
import { Switch } from '@resources/components/ui/Switch';
import type { SearchMode } from '@resources/utils/search-filter';
import { PERM } from '@core/auth/permissionKeys';
import { useSecrets } from '@resources/hooks/useSecrets';
import { useSecretColumns } from '../pages/useSecretColumns';
import { SECRET_GROUP_ALL, useSecretGroups } from '../hooks/useSecretGroups';
import { SecretEditDialog } from './SecretEditDialog';
import { SecretFormDialog } from './SecretFormDialog';
import { RevealSecretDialog } from './RevealSecretDialog';
import { DeleteSecretDialog } from './DeleteSecretDialog';
import { RotationBanner } from './RotationBanner';
import { SecretImpactDialog } from './SecretImpactDialog';
import type { Secret } from '../types';

/**
 * The vault list, mounted as the Secrets destination of the Security page.
 *
 * It lived under Docker while the only thing it served was stack environment
 * variables. It now holds registry passwords, git credentials, SMTP settings,
 * TLS certificates, OIDC client secrets and agent enrolment tokens — none of
 * which is a Docker concept — so it sits with the rest of the application's
 * access control.
 */
export function SecretsTab() {
  const { t } = useTranslation('secrets');

  // System secrets are the credentials McHarbor manages for itself. They are
  // hidden by default so the page does not open on fifty rows nobody typed.
  const [showSystem, setShowSystem] = useState(false);
  const {
    data: secrets = [],
    isLoading,
    isFetching,
    refetch,
  } = useSecrets({
    includeSystem: showSystem,
  });

  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  const [createOpen, setCreateOpen] = useState(false);
  const [rotateTarget, setRotateTarget] = useState<Secret | null>(null);
  const [revealTarget, setRevealTarget] = useState<Secret | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Secret | null>(null);
  const [rotatedSecret, setRotatedSecret] = useState<Secret | null>(null);
  const [impactTarget, setImpactTarget] = useState<Secret | null>(null);
  const [editTarget, setEditTarget] = useState<Secret | null>(null);

  const [group, setGroup] = useState<string>(SECRET_GROUP_ALL);
  const [grouped, setGrouped] = useState(false);
  const [collapsed, setCollapsed] = useState<string[]>([]);
  const { names: groupNames, kinds: kindNames, facets: groupFacets, filter: filterByGroup } = useSecretGroups(secrets);
  const visible = useMemo(() => filterByGroup(secrets, group), [filterByGroup, secrets, group]);

  const grouping = useMemo(
    () =>
      grouped
        ? {
            key: (secret: Secret) => secret.group?.trim() ?? '',
            label: (key: string) => key || t('groups.ungrouped'),
            collapsed,
            onToggle: (key: string) =>
              setCollapsed((current) =>
                current.includes(key) ? current.filter((entry) => entry !== key) : [...current, key],
              ),
          }
        : undefined,
    [grouped, collapsed, t],
  );

  const columns = useSecretColumns({
    onRotate: (secret) => {
      setRotateTarget(secret);
      setRotatedSecret(secret);
    },
    onReveal: setRevealTarget,
    onDelete: setDeleteTarget,
    onReviewImpact: setImpactTarget,
    onEdit: setEditTarget,
  }) as ColumnDef<Secret, unknown>[];

  const banner = useMemo(
    () =>
      rotatedSecret ? (
        <RotationBanner
          secret={rotatedSecret}
          onDismiss={() => setRotatedSecret(null)}
          onReview={() => setImpactTarget(rotatedSecret)}
        />
      ) : undefined,
    [rotatedSecret],
  );

  return (
    <>
      <TablePage
        id="secrets"
        header={false}
        title={t('title')}
        description={t('description', { count: secrets.length })}
        banner={banner}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('searchPlaceholder'),
          fields: (secret: Secret) => [secret.name, secret.description, secret.stackName, secret.group],
        }}
        facets={{ items: groupFacets, value: group, onChange: setGroup }}
        savedFilters={{
          state: { query, mode: searchMode, group, showSystem },
          onRestore: (s) => {
            setQuery(typeof s.query === 'string' ? s.query : '');
            if (s.mode === 'contains' || s.mode === 'exact' || s.mode === 'regex') setSearchMode(s.mode);
            if (typeof s.group === 'string') setGroup(s.group);
            if (typeof s.showSystem === 'boolean') setShowSystem(s.showSystem);
          },
          pristine: query === '' && group === SECRET_GROUP_ALL && !showSystem,
          defaults: { query: '', mode: 'contains', group: SECRET_GROUP_ALL, showSystem: false },
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-secrets' },
          actions: (
            <>
              <label className="flex items-center gap-2 text-xs text-muted-foreground">
                <Switch checked={grouped} onCheckedChange={setGrouped} />
                {t('groups.toggle')}
              </label>
              <label className="flex items-center gap-2 text-xs text-muted-foreground">
                <Switch checked={showSystem} onCheckedChange={setShowSystem} />
                {t('showSystem')}
              </label>
              <Button permission={PERM.SECRETS_MANAGE} onClick={() => setCreateOpen(true)}>
                <IconPlus className="size-4" /> {t('create.title')}
              </Button>
            </>
          ),
        }}
        columns={columns}
        data={visible}
        grouping={grouping}
        loading={isLoading}
        empty={{ title: t('empty.title'), description: t('empty.description') }}
        getRowId={(row) => row.id}
      />

      <SecretFormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        groupSuggestions={groupNames}
        kindSuggestions={kindNames}
      />
      <SecretFormDialog
        open={rotateTarget !== null}
        onOpenChange={(isOpen2) => !isOpen2 && setRotateTarget(null)}
        secret={rotateTarget}
      />
      <RevealSecretDialog
        open={revealTarget !== null}
        onOpenChange={(isOpen2) => !isOpen2 && setRevealTarget(null)}
        secret={revealTarget}
      />
      <DeleteSecretDialog
        open={deleteTarget !== null}
        onOpenChange={(dialogOpen) => !dialogOpen && setDeleteTarget(null)}
        secret={deleteTarget}
      />
      <SecretEditDialog
        open={editTarget !== null}
        onOpenChange={(openState) => !openState && setEditTarget(null)}
        secret={editTarget}
        groupSuggestions={groupNames}
      />
      <SecretImpactDialog
        open={impactTarget !== null}
        onOpenChange={(isOpen) => !isOpen && setImpactTarget(null)}
        secret={impactTarget}
      />
    </>
  );
}
