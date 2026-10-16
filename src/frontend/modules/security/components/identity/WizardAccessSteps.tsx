// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import type { IdentityAdapterSpec } from '@resources/hooks/useIdentityAdapters';
import { useRoles } from '../../hooks/useRoles';
import { useGroups } from '../../hooks/useGroups';
import { GroupMappingEditor } from '../GroupMappingEditor';
import { MappingPreviewPanel } from './MappingPreviewPanel';
import type { ProviderWizard } from './useProviderWizard';

const NO_ROLE = '__none__';

export function WizardAccessStep({ wizard, spec }: { wizard: ProviderWizard; spec: IdentityAdapterSpec | null }) {
  const { t } = useTranslation('security');
  const { data: roles = [] } = useRoles();
  const roleOptions = [
    { value: NO_ROLE, label: t('wizard.noDefaultRole', { ns: 'access' }) },
    ...roles.map((r) => ({ value: r.id, label: r.name })),
  ];
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between rounded-lg border border-border p-3">
        <div>
          <p className="text-sm font-medium text-foreground">{t('identity.autoProvision')}</p>
          <p className="text-xs text-muted-foreground">{t('identity.autoProvisionDescription')}</p>
        </div>
        <Switch
          checked={wizard.autoProvision}
          onCheckedChange={wizard.setAutoProvision}
          aria-label={t('identity.autoProvision')}
        />
      </div>
      <div>
        <Label className="mb-1">{t('wizard.defaultRole', { ns: 'access' })}</Label>
        <Select
          value={wizard.defaultRoleId || NO_ROLE}
          onChange={(v) => wizard.setDefaultRoleId(v === NO_ROLE ? '' : v)}
          options={roleOptions}
          ariaLabel={t('wizard.defaultRole', { ns: 'access' })}
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('wizard.defaultRoleHint', { ns: 'access' })}</p>
      </div>
      <GroupMappingEditor
        autoImport={wizard.autoImportGroups}
        enabled={wizard.groupMappingEnabled}
        mappings={wizard.groupMappings}
        supportsFetchGroups={false}
        onAutoImportChange={wizard.setAutoImportGroups}
        onEnabledChange={wizard.setGroupMappingEnabled}
        onMappingsChange={wizard.setGroupMappings}
      />
      {spec?.Capabilities.SupportsGroupFetch && (
        <p className="text-xs text-muted-foreground">{t('wizard.fetchGroupsAfterSave', { ns: 'access' })}</p>
      )}
      <MappingPreviewPanel providerType={wizard.providerType ?? ''} config={wizard.mapping} />
    </div>
  );
}

export function WizardReviewStep({ wizard, spec }: { wizard: ProviderWizard; spec: IdentityAdapterSpec | null }) {
  const { t } = useTranslation('access');
  const { data: roles = [] } = useRoles();
  const { data: groups = [] } = useGroups();
  const roleName = roles.find((r) => r.id === wizard.defaultRoleId)?.name;
  const groupName = (id: string) => groups.find((g) => g.id === id)?.name ?? id;
  const configured = Object.keys(wizard.config).filter((k) => k !== 'name' && wizard.config[k]);
  const rows: [string, string][] = [
    [t('wizard.review.name'), wizard.config.name ?? ''],
    [t('wizard.review.type'), spec?.DisplayName ?? wizard.providerType ?? ''],
    [t('wizard.review.fields'), configured.join(', ') || t('wizard.review.none')],
    [t('wizard.review.autoProvision'), wizard.autoProvision ? t('wizard.review.yes') : t('wizard.review.no')],
    [t('wizard.review.defaultRole'), roleName ?? t('wizard.review.none')],
    [t('wizard.review.autoImport'), wizard.autoImportGroups ? t('wizard.review.yes') : t('wizard.review.no')],
    [
      t('wizard.review.mappings'),
      wizard.groupMappingEnabled && wizard.mapping.groupMappings.length > 0
        ? wizard.mapping.groupMappings.map((m) => `${m.providerGroup} → ${groupName(m.mcharborGroupId)}`).join(', ')
        : t('wizard.review.none'),
    ],
  ];
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">{t('wizard.review.description')}</p>
      <dl className="divide-y divide-border rounded-lg border border-border text-sm">
        {rows.map(([label, value]) => (
          <div key={label} className="flex gap-3 px-3 py-2">
            <dt className="w-40 shrink-0 text-muted-foreground">{label}</dt>
            <dd className="min-w-0 break-words text-foreground">{value}</dd>
          </div>
        ))}
      </dl>
      <p className="text-xs text-muted-foreground">{t('wizard.review.disabledNote')}</p>
    </div>
  );
}
