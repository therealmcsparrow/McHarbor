// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Textarea } from '@resources/components/ui/Textarea';
import { PERM } from '@core/auth/permissionKeys';
import {
  parseGroupList,
  useMappingPreview,
  type MappingConfig,
  type MappingPreview,
} from '../../hooks/useIdentitySetup';
import { useFetchProviderGroups } from '../../hooks/useIdentityProviders';
import { RoleGrantBadge, RoleImpactList } from './RoleImpactList';

type Props = {
  providerId?: string;
  providerType: string;
  config: MappingConfig;
  suggestedGroups?: string[];
  canFetchGroups?: boolean;
};

function SampleResult({ preview }: { preview: MappingPreview }) {
  const { t } = useTranslation('access');
  const sample = preview.sample;
  return (
    <div className="space-y-2">
      {sample.groups.length === 0 && <p className="text-xs text-muted-foreground">{t('preview.noGroups')}</p>}
      <ul className="space-y-2" aria-label={t('preview.groupsJoined')}>
        {sample.groups.map((group) => (
          <li key={group.groupId ?? `new-${group.groupName}`} className="rounded-md border border-border p-2 text-xs">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium text-foreground">{group.groupName}</span>
              {group.new && <Badge variant="info">{t('preview.newGroup')}</Badge>}
              <span className="text-muted-foreground">{t('preview.from', { groups: group.from.join(', ') })}</span>
            </div>
            <div className="mt-1 flex flex-wrap gap-1">
              {group.roles.length === 0 ? (
                <span className="text-muted-foreground">{t('preview.noRoles')}</span>
              ) : (
                group.roles.map((grant) => (
                  <RoleGrantBadge
                    key={`${grant.roleId}-${grant.environmentId ?? ''}-${grant.stackName ?? ''}`}
                    grant={grant}
                  />
                ))
              )}
            </div>
          </li>
        ))}
      </ul>
      {sample.defaultRole && (
        <p className="text-xs text-muted-foreground">
          {t('preview.defaultRole', { role: sample.defaultRole.roleName || sample.defaultRole.roleId })}
        </p>
      )}
      {sample.unmapped.length > 0 && (
        <p className="text-xs text-muted-foreground">{t('preview.unmapped', { groups: sample.unmapped.join(', ') })}</p>
      )}
    </div>
  );
}

export function MappingPreviewPanel({ providerId, providerType, config, suggestedGroups, canFetchGroups }: Props) {
  const { t } = useTranslation('access');
  const [text, setText] = useState('');
  const preview = useMappingPreview();
  const fetchGroups = useFetchProviderGroups();

  const run = () => preview.mutate({ providerId, providerType, config, sampleGroups: parseGroupList(text) });

  return (
    <div className="space-y-3 rounded-lg border border-border p-3">
      <div>
        <p className="text-sm font-medium text-foreground">{t('preview.title')}</p>
        <p className="text-xs text-muted-foreground">{t('preview.description')}</p>
      </div>
      <div>
        <Label htmlFor="mapping-sample" className="mb-1">
          {t('preview.sampleGroups')}
        </Label>
        <Textarea
          id="mapping-sample"
          rows={3}
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={t('preview.samplePlaceholder')}
        />
      </div>
      <div className="flex flex-wrap gap-2">
        {suggestedGroups && suggestedGroups.length > 0 && (
          <Button variant="ghost" size="sm" onClick={() => setText(suggestedGroups.join('\n'))}>
            {t('preview.useTestGroups', { count: suggestedGroups.length })}
          </Button>
        )}
        {providerId && canFetchGroups && (
          <Button
            variant="ghost"
            size="sm"
            permission={PERM.IDENTITY_PROVIDERS_MANAGE}
            disabled={fetchGroups.isPending}
            onClick={() =>
              fetchGroups.mutate(providerId, { onSuccess: (groups) => setText(groups.map((g) => g.name).join('\n')) })
            }
          >
            {t('preview.loadGroups')}
          </Button>
        )}
        <Button
          variant="outline"
          size="sm"
          permission={PERM.IDENTITY_PROVIDERS_MANAGE}
          disabled={preview.isPending}
          onClick={run}
        >
          <IconEye className="size-4" />
          {t('preview.run')}
        </Button>
      </div>
      {fetchGroups.isError && <Alert variant="destructive">{fetchGroups.error.message}</Alert>}
      {preview.isError && <Alert variant="destructive">{preview.error.message}</Alert>}
      {preview.data && <SampleResult preview={preview.data} />}
      {preview.data?.impact && <RoleImpactList impact={preview.data.impact} />}
    </div>
  );
}
