// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { useCreateNetwork } from '../hooks/useNetworks';
import type { CreateNetworkRequest, IPAMConfig } from '../hooks/useNetworks';
import type { KeyValuePair } from './KeyValueRows';
import { DRIVER_CONFIG, DRIVER_OPTIONS } from './networkDriverConfig';
import type { Driver } from './networkDriverConfig';
import { NetworkDriverFields } from './NetworkDriverFields';
import { NetworkIPAMSection } from './NetworkIPAMSection';
import { NetworkAdvancedSection } from './NetworkAdvancedSection';

interface NetworkCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function NetworkCreateDialog({ open, onOpenChange }: NetworkCreateDialogProps) {
  const { t } = useTranslation('networks');
  const { t: tc } = useTranslation('common');
  const createNetwork = useCreateNetwork();

  const [name, setName] = useState('');
  const [driver, setDriver] = useState<Driver>('bridge');
  const [parent, setParent] = useState('');
  const [mode, setMode] = useState('');
  const [internal, setInternal] = useState(false);
  const [attachable, setAttachable] = useState(false);
  const [ipamConfigs, setIpamConfigs] = useState<IPAMConfig[]>([{ Subnet: '', Gateway: '', IPRange: '' }]);
  const [customOptions, setCustomOptions] = useState<KeyValuePair[]>([]);
  const [labels, setLabels] = useState<KeyValuePair[]>([]);
  const [advancedOpen, setAdvancedOpen] = useState(false);

  const cfg = DRIVER_CONFIG[driver];

  const resetForm = useCallback(() => {
    setName(''); setDriver('bridge'); setParent(''); setMode('');
    setInternal(false); setAttachable(false);
    setIpamConfigs([{ Subnet: '', Gateway: '', IPRange: '' }]);
    setCustomOptions([]); setLabels([]); setAdvancedOpen(false);
  }, []);

  const handleDriverChange = (d: Driver) => {
    setDriver(d); setParent('');
    setMode(DRIVER_CONFIG[d].modes?.[0] ?? '');
    setInternal(false); setAttachable(false);
  };

  const handleClose = (isOpen: boolean) => { if (!isOpen) resetForm(); onOpenChange(isOpen); };

  const canSubmit = name.trim() !== '' && (!cfg.hasParent || parent.trim() !== '') && !createNetwork.isPending;

  const handleSubmit = () => {
    if (!canSubmit) return;
    const options: Record<string, string> = {};
    if (cfg.hasParent && parent.trim()) options['parent'] = parent.trim();
    if (cfg.hasMode && mode) options[driver === 'macvlan' ? 'macvlan_mode' : 'ipvlan_mode'] = mode;
    for (const kv of customOptions) { if (kv.key.trim()) options[kv.key.trim()] = kv.value; }
    const filteredLabels: Record<string, string> = {};
    for (const kv of labels) { if (kv.key.trim()) filteredLabels[kv.key.trim()] = kv.value; }
    const filteredIpam = ipamConfigs.filter((c) => c.Subnet || c.Gateway || c.IPRange);

    const req: CreateNetworkRequest = { name: name.trim(), driver };
    if (cfg.hasToggles && internal) req.internal = true;
    if (cfg.hasToggles && attachable) req.attachable = true;
    if (cfg.hasIpam && filteredIpam.length > 0) req.ipam = { Config: filteredIpam };
    if (Object.keys(options).length > 0) req.options = options;
    if (Object.keys(filteredLabels).length > 0) req.labels = filteredLabels;
    createNetwork.mutate(req, { onSuccess: () => handleClose(false) });
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('create.title')}</DialogTitle>
          <DialogDescription>{t('create.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-3">
            <div>
              <Label className="mb-1">{t('create.nameLabel')} <span className="text-destructive">*</span></Label>
              <Input variant="outline" type="text" value={name} onChange={(e) => setName(e.target.value)} placeholder={t('create.namePlaceholder')} />
            </div>
            <div>
              <Label className="mb-1">{t('create.driverLabel')}</Label>
              <Select variant="outline" value={driver} onChange={(v) => handleDriverChange(v as Driver)} options={[...DRIVER_OPTIONS]} />
            </div>
          </div>
          <NetworkDriverFields driver={driver} cfg={cfg} parent={parent} onParentChange={setParent} mode={mode} onModeChange={setMode} internal={internal} onInternalChange={setInternal} attachable={attachable} onAttachableChange={setAttachable} />
          {cfg.hasIpam && <NetworkIPAMSection driver={driver} ipamConfigs={ipamConfigs} onIpamConfigsChange={setIpamConfigs} />}
          <NetworkAdvancedSection open={advancedOpen} onToggle={() => setAdvancedOpen(!advancedOpen)} customOptions={customOptions} onCustomOptionsChange={setCustomOptions} labels={labels} onLabelsChange={setLabels} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleClose(false)}>{tc('actions.cancel')}</Button>
          <Button onClick={handleSubmit} disabled={!canSubmit}>{createNetwork.isPending ? t('create.creating') : t('create.submit')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
