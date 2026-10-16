// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { useHostInterfaces } from '../hooks/useNetworks';
import type { HostLinkFormValue } from '../utils/hostLinkForm';

interface HostLinkFieldsProps {
  value: HostLinkFormValue;
  onChange: (value: HostLinkFormValue) => void;
  enableIPv6: boolean;
  mode: string;
}

export function HostLinkFields({ value, onChange, enableIPv6, mode }: HostLinkFieldsProps) {
  const { t } = useTranslation('networks');
  const { data } = useHostInterfaces();

  const unavailableReasons = {
    agent_unreachable: t('hostLink.unavailable.agent_unreachable'),
    agent_outdated: t('hostLink.unavailable.agent_outdated'),
    containerized: t('hostLink.unavailable.containerized'),
  };
  let blocked: string | null = null;
  if (mode === 'passthru') blocked = t('hostLink.passthru');
  else if (data && !data.hostLinkAvailable) blocked = unavailableReasons[data.hostLinkUnavailable ?? 'containerized'];
  else if (!data) blocked = t('create.parentLoading');
  const on = value.enabled && blocked === null;
  const set = (patch: Partial<HostLinkFormValue>) => onChange({ ...value, ...patch });

  return (
    <div className="space-y-3 border-t border-border pt-4">
      <div className="flex items-start justify-between gap-3 rounded-md border border-border px-3 py-2">
        <div className="min-w-0">
          <p className="text-sm">{t('hostLink.switch')}</p>
          <p className="text-xs text-muted-foreground">{blocked ?? t('hostLink.hint')}</p>
        </div>
        <Switch
          checked={on}
          disabled={blocked !== null}
          aria-label={t('hostLink.switch')}
          onCheckedChange={(enabled) => set({ enabled })}
        />
      </div>
      {on && (
        <div className="space-y-3">
          <div>
            <Label className="mb-1">
              {t('hostLink.name')} <span className="text-destructive">*</span>
            </Label>
            <Input
              variant="outline"
              type="text"
              value={value.name}
              onChange={(e) => set({ name: e.target.value })}
              placeholder={t('hostLink.namePlaceholder')}
            />
          </div>
          <div>
            <Label className="mb-1">{t('hostLink.ipv4')}</Label>
            <Input
              variant="outline"
              type="text"
              value={value.ipv4}
              onChange={(e) => set({ ipv4: e.target.value })}
              placeholder={t('hostLink.ipv4Placeholder')}
            />
          </div>
          {enableIPv6 && (
            <div className="space-y-2">
              <Label className="mb-1">{t('hostLink.ipv6')}</Label>
              <Input
                variant="outline"
                type="text"
                value={value.ipv6}
                disabled={value.ipv6Auto}
                onChange={(e) => set({ ipv6: e.target.value })}
                placeholder={t('hostLink.ipv6Placeholder')}
              />
              <div className="flex items-start justify-between gap-3 rounded-md border border-border px-3 py-2">
                <p className="text-xs text-muted-foreground">{t('hostLink.autoHint')}</p>
                <Switch
                  checked={value.ipv6Auto}
                  aria-label={t('hostLink.auto')}
                  onCheckedChange={(ipv6Auto) => set({ ipv6Auto })}
                />
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
