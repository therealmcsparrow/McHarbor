// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import type { CreateVMForm } from '../../hooks/useCreateVMForm';
import { networkLabel } from '../../utils/nicAttachment';

export function CreateNetworkStep({ form }: { form: CreateVMForm }) {
  const { t } = useTranslation('vm');
  const { networkId, setNetworkId, networks } = form;

  return (
    <>
      <h3 className="font-medium text-foreground">{t('create.network.title')}</h3>
      {(networks ?? []).length === 0 ? (
        <p className="text-sm text-warning">{t('create.network.noNetworks')}</p>
      ) : (
        <div>
          <Label>{t('create.network.network')}</Label>
          <Select
            value={networkId}
            ariaLabel={t('create.network.network')}
            options={(networks ?? []).map((entry) => ({
              value: entry.id,
              label: networkLabel(entry),
            }))}
            onChange={setNetworkId}
          />
        </div>
      )}
    </>
  );
}
