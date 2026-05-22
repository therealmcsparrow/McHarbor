// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlus, IconTrash, IconCloudDownload } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import { Select } from '@resources/components/ui/Select';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';
import { useGroups } from '../hooks/useGroups';
import { useFetchProviderGroups } from '../hooks/useIdentityProviders';
import type { GroupMapping } from '../hooks/useIdentityProviders';

type GroupMappingEditorProps = {
  autoImport: boolean;
  enabled: boolean;
  mappings: GroupMapping[];
  providerId?: string;
  onAutoImportChange: (autoImport: boolean) => void;
  onEnabledChange: (enabled: boolean) => void;
  onMappingsChange: (mappings: GroupMapping[]) => void;
};

export function GroupMappingEditor({
  autoImport,
  enabled,
  mappings,
  providerId,
  onAutoImportChange,
  onEnabledChange,
  onMappingsChange,
}: GroupMappingEditorProps) {
  const { t } = useTranslation('security');
  const { data: groups = [] } = useGroups();
  const fetchGroups = useFetchProviderGroups();

  const groupOptions = groups.map((g) => ({
    value: g.id,
    label: g.name,
  }));
  const mappingKeys = useStableListKeys(
    mappings,
    (mapping) => `${mapping.providerGroup}\u0000${mapping.mcharborGroupId}`,
  );

  function addMapping() {
    onMappingsChange([...mappings, { providerGroup: '', mcharborGroupId: '' }]);
  }

  function removeMapping(index: number) {
    onMappingsChange(mappings.filter((_, i) => i !== index));
  }

  function updateMapping(index: number, partial: Partial<GroupMapping>) {
    onMappingsChange(
      mappings.map((m, i) => (i === index ? { ...m, ...partial } : m))
    );
  }

  function handleFetchGroups() {
    if (!providerId) return;
    fetchGroups.mutate(providerId, {
      onSuccess: (providerGroups) => {
        // Pre-populate mapping rows with fetched group names (leave McHarbor group empty)
        const existingNames = new Set(mappings.map((m) => m.providerGroup));
        const newMappings = providerGroups
          .filter((g) => !existingNames.has(g.name))
          .map((g) => ({ providerGroup: g.name, mcharborGroupId: '' }));
        if (newMappings.length > 0) {
          onMappingsChange([...mappings, ...newMappings]);
        }
      },
    });
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between rounded-lg border border-border p-3">
        <div>
          <p className="text-sm font-medium text-foreground">{t('identity.autoImportGroups')}</p>
          <p className="text-xs text-muted-foreground">{t('identity.autoImportGroupsDescription')}</p>
        </div>
        <Switch checked={autoImport} onCheckedChange={onAutoImportChange} />
      </div>

      <div className="flex items-center justify-between rounded-lg border border-border p-3">
        <div>
          <p className="text-sm font-medium text-foreground">{t('identity.groupMapping')}</p>
          <p className="text-xs text-muted-foreground">{t('identity.groupMappingDescription')}</p>
        </div>
        <Switch checked={enabled} onCheckedChange={onEnabledChange} />
      </div>

      {enabled && (
        <div className="space-y-2 rounded-lg border border-border p-3">
          {mappings.length > 0 && (
            <div className="grid grid-cols-[1fr_1fr_auto] gap-2 text-xs font-medium text-muted-foreground">
              <span>{t('identity.providerGroup')}</span>
              <span>{t('identity.mcharborGroup')}</span>
              <span className="w-8" />
            </div>
          )}

          {mappings.map((mapping, index) => (
            <div key={mappingKeys[index]} className="grid grid-cols-[1fr_1fr_auto] items-center gap-2">
              <Input
                variant="outline"
                value={mapping.providerGroup}
                onChange={(e) => updateMapping(index, { providerGroup: e.target.value })}
                placeholder={t('identity.providerGroupPlaceholder')}
                className="text-sm"
              />
              <Select
                variant="outline"
                value={mapping.mcharborGroupId}
                onChange={(value) => updateMapping(index, { mcharborGroupId: value })}
                options={groupOptions}
                placeholder={t('identity.selectGroup')}
                searchable={groupOptions.length > 5}
              />
              <Button
                variant="ghost"
                size="icon"
                onClick={() => removeMapping(index)}
                aria-label={t('identity.removeMapping')}
                className="size-8 text-muted-foreground hover:text-destructive"
              >
                <IconTrash className="size-4" />
              </Button>
            </div>
          ))}

          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={addMapping}
              className="gap-1.5"
            >
              <IconPlus className="size-3.5" />
              {t('identity.addMapping')}
            </Button>

            {providerId && (
              <Button
                variant="outline"
                size="sm"
                onClick={handleFetchGroups}
                disabled={fetchGroups.isPending}
                className="gap-1.5"
              >
                <IconCloudDownload className="size-3.5" />
                {fetchGroups.isPending ? '...' : t('identity.fetchGroups')}
              </Button>
            )}
          </div>

          {mappings.length > 0 && (
            <p className="text-xs text-muted-foreground">
              {t('identity.groupMappingHint')}
            </p>
          )}
        </div>
      )}
    </div>
  );
}
