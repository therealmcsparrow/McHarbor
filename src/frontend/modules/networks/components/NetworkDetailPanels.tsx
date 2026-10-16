// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconPlus, IconTrash } from '@tabler/icons-react';
import type { NetworkInfo } from '@core/types/docker';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Input } from '@resources/components/ui/Input';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';
import { KeyValueEditor } from '@resources/components/KeyValueEditor';
import { Select } from '@resources/components/ui/Select';
import { formatDate } from '@resources/utils/format';
import { DRIVER_OPTIONS } from './networkDriverConfig';
import { ToggleField } from './ToggleField';
import type { NetworkEditData } from '../hooks/useNetworkDetailState';

type Translation = (key: string) => string;

/**
 * The four blocks this component can draw. The detail page splits them across
 * tabs — general + IPAM on Overview, options + labels on Configuration — so it
 * asks for a subset rather than receiving all four and hiding half of them.
 * Omitting the prop keeps the original behaviour: every section, in order.
 */
export type NetworkPanelSection = 'general' | 'ipam' | 'options' | 'labels';

const ALL_SECTIONS: NetworkPanelSection[] = ['general', 'ipam', 'options', 'labels'];
const DEFAULT_IPAM_DRIVER = 'default';

type NetworkDetailPanelProps = {
  network: NetworkInfo;
  editData: NetworkEditData | null;
  editing: boolean;
  t: Translation;
  onUpdateEdit: <K extends keyof NetworkEditData>(key: K, value: NetworkEditData[K]) => void;
  onUpdateIPAMEntry: (index: number, field: 'subnet' | 'gateway' | 'ipRange', value: string) => void;
  onAddIPAMEntry: () => void;
  onRemoveIPAMEntry: (index: number) => void;
  sections?: NetworkPanelSection[];
};

export function NetworkDetailPanels({
  network,
  editData,
  editing,
  t,
  onUpdateEdit,
  onUpdateIPAMEntry,
  onAddIPAMEntry,
  onRemoveIPAMEntry,
  sections = ALL_SECTIONS,
}: NetworkDetailPanelProps) {
  const showGeneral = sections.includes('general');
  const showIpam = sections.includes('ipam');
  const showOptions = sections.includes('options');
  const showLabels = sections.includes('labels');
  // The edit form's IPAM rows can be removed; the read-only panel below
  // renders straight from the API and never reorders, so it keeps its index.
  const ipamEditKeys = useStableListKeys(editData?.ipamConfig ?? []);

  if (!editing || !editData) {
    return (
      <>
        {showGeneral && (
          <div className="mc-glass rounded-lg p-6">
            <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
              {t('detail.general')}
            </h3>
            <InfoRow label={t('detail.name')}>{network.Name}</InfoRow>
            <InfoRow label={t('detail.networkId')}>
              <span className="font-mono text-xs">{network.Id}</span>
            </InfoRow>
            <InfoRow label={t('detail.driver')}>
              <Badge variant="default" className="px-1.5 py-0 text-[10px]">
                {network.Driver}
              </Badge>
            </InfoRow>
            <InfoRow label={t('detail.scope')}>
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                {network.Scope}
              </Badge>
            </InfoRow>
            <InfoRow label={t('detail.created')}>{formatDate(network.Created)}</InfoRow>
            <InfoRow label={t('detail.internal')}>
              {network.Internal ? <Badge variant="warning">{t('badges.yes')}</Badge> : t('badges.no')}
            </InfoRow>
            <InfoRow label={t('detail.attachable')}>
              {network.Attachable ? <Badge variant="success">{t('badges.yes')}</Badge> : t('badges.no')}
            </InfoRow>
            <InfoRow label={t('detail.ipv6')}>
              {network.EnableIPv6 ? <Badge variant="success">{t('badges.yes')}</Badge> : t('badges.no')}
            </InfoRow>
          </div>
        )}

        {showIpam && (
          <div className="mc-glass rounded-lg p-6">
            <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
              {t('detail.ipam')}
            </h3>
            <InfoRow label={t('detail.ipamDriver')}>{network.IPAM?.Driver || DEFAULT_IPAM_DRIVER}</InfoRow>
            {network.IPAM?.Config?.length ? (
              network.IPAM.Config.map((config, index) => (
                <div key={`ipam-${config.Subnet ?? config.Gateway ?? index}`}>
                  {config.Subnet && (
                    <InfoRow label={t('detail.subnet')}>
                      <span className="font-mono text-xs">{config.Subnet}</span>
                    </InfoRow>
                  )}
                  {config.Gateway && (
                    <InfoRow label={t('detail.gateway')}>
                      <span className="font-mono text-xs">{config.Gateway}</span>
                    </InfoRow>
                  )}
                  {config.IPRange && (
                    <InfoRow label={t('detail.ipRange')}>
                      <span className="font-mono text-xs">{config.IPRange}</span>
                    </InfoRow>
                  )}
                </div>
              ))
            ) : (
              <p className="text-sm text-muted-foreground">{t('detail.noIpamConfig')}</p>
            )}
          </div>
        )}

        {showOptions && network.Options && Object.keys(network.Options).length > 0 && (
          <div className="mc-glass rounded-lg p-6">
            <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
              {t('detail.options')}
            </h3>
            {Object.entries(network.Options).map(([key, value]) => (
              <InfoRow key={key} label={key} variant="spread" title={String(value)}>
                <span className="font-mono text-xs">{String(value)}</span>
              </InfoRow>
            ))}
          </div>
        )}

        {showLabels && network.Labels && Object.keys(network.Labels).length > 0 && (
          <div className="mc-glass rounded-lg p-6">
            <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
              {t('detail.labels')}
            </h3>
            {Object.entries(network.Labels).map(([key, value]) => (
              <InfoRow key={key} label={key} variant="spread" title={String(value)}>
                <span className="font-mono text-xs">{String(value)}</span>
              </InfoRow>
            ))}
          </div>
        )}
      </>
    );
  }

  return (
    <>
      {showGeneral && (
        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('detail.general')}
          </h3>
          <div className="space-y-3">
            <div>
              <label className="text-xs font-medium text-muted-foreground">{t('detail.name')}</label>
              <Input
                variant="outline"
                value={editData.name}
                onChange={(e) => onUpdateEdit('name', e.target.value)}
                placeholder={t('edit.namePlaceholder')}
                className="mt-1"
              />
            </div>
            <InfoRow label={t('detail.networkId')}>
              <span className="font-mono text-xs">{network.Id}</span>
            </InfoRow>
            <div>
              <label className="text-xs font-medium text-muted-foreground">{t('detail.driver')}</label>
              <Select
                variant="outline"
                value={editData.driver}
                onChange={(value) => onUpdateEdit('driver', value)}
                options={DRIVER_OPTIONS.map((option) => ({ value: option.value, label: option.label }))}
                className="mt-1"
              />
            </div>
            <InfoRow label={t('detail.scope')}>
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                {network.Scope}
              </Badge>
            </InfoRow>
            <InfoRow label={t('detail.created')}>{formatDate(network.Created)}</InfoRow>
            <ToggleField
              label={t('detail.internal')}
              checked={editData.internal}
              onChange={(value) => onUpdateEdit('internal', value)}
            />
            <ToggleField
              label={t('detail.attachable')}
              checked={editData.attachable}
              onChange={(value) => onUpdateEdit('attachable', value)}
            />
            <ToggleField
              label={t('detail.ipv6')}
              checked={editData.enableIPv6}
              onChange={(value) => onUpdateEdit('enableIPv6', value)}
            />
          </div>
        </div>
      )}

      {showIpam && (
        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('detail.ipam')}
          </h3>
          <div className="space-y-4">
            <InfoRow label={t('detail.ipamDriver')}>{editData.ipamDriver}</InfoRow>
            {editData.ipamConfig.map((config, index) => (
              <div key={ipamEditKeys[index]} className="space-y-2 rounded-md border border-border p-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-medium text-muted-foreground">
                    {t('detail.subnet')} #{index + 1}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t('edit.removeIpamConfig')}
                    onClick={() => onRemoveIPAMEntry(index)}
                    className="size-6 text-muted-foreground hover:text-danger"
                  >
                    <IconTrash className="size-3" />
                  </Button>
                </div>
                <Input
                  variant="outline"
                  value={config.subnet}
                  onChange={(e) => onUpdateIPAMEntry(index, 'subnet', e.target.value)}
                  placeholder={t('create.subnetPlaceholder')}
                />
                <Input
                  variant="outline"
                  value={config.gateway}
                  onChange={(e) => onUpdateIPAMEntry(index, 'gateway', e.target.value)}
                  placeholder={t('create.gatewayPlaceholder')}
                />
                <Input
                  variant="outline"
                  value={config.ipRange}
                  onChange={(e) => onUpdateIPAMEntry(index, 'ipRange', e.target.value)}
                  placeholder={t('create.ipRangePlaceholder')}
                />
              </div>
            ))}
            <Button variant="outline" size="sm" onClick={onAddIPAMEntry}>
              <IconPlus className="mr-1 size-3.5" />
              {t('edit.addIpamConfig')}
            </Button>
          </div>
        </div>
      )}

      {showOptions && (
        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('detail.options')}
          </h3>
          <KeyValueEditor
            entries={editData.options}
            onChange={(entries) => onUpdateEdit('options', entries)}
            keyLabel={t('create.optionKey')}
            valueLabel={t('create.optionValue')}
            addLabel={t('create.add')}
          />
        </div>
      )}

      {showLabels && (
        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('detail.labels')}
          </h3>
          <KeyValueEditor
            entries={editData.labels}
            onChange={(entries) => onUpdateEdit('labels', entries)}
            keyLabel={t('create.labelKey')}
            valueLabel={t('create.labelValue')}
            addLabel={t('create.add')}
          />
        </div>
      )}
    </>
  );
}
