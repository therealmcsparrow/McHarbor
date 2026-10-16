// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Dispatch, SetStateAction } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCircleDashed, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Caption } from '@resources/components/ui/Typography';
import { Flex } from '@resources/components/ui/Layout';
import { Section } from '@resources/components/ui/Form';
import { RepeaterRows } from '@resources/components/ui/RepeaterRows';
import type { PortBinding } from '../hooks/useCreateContainer';
import { EMPTY_PORT, updateList } from './createContainerFormHelpers';

type CreateContainerPortsSectionProps = {
  ports: PortBinding[];
  setPorts: Dispatch<SetStateAction<PortBinding[]>>;
};

export function CreateContainerPortsSection({ ports, setPorts }: CreateContainerPortsSectionProps) {
  const { t } = useTranslation('containers');
  return (
    <Section
      icon={<IconCircleDashed className="size-3.5" />}
      title={t('create.form.ports')}
      help={t('create.form.portsHelp')}
    >
      <RepeaterRows<PortBinding>
        items={ports}
        onChange={setPorts}
        onAdd={() => ({ ...EMPTY_PORT })}
        addLabel={t('create.form.addPort')}
        renderRow={(row, _idx, { id, remove }) => (
          <Flex gap={2} align="center">
            <Input
              value={row.host}
              onChange={(e) => updateList(setPorts, id, { ...row, host: e.target.value })}
              placeholder={t('create.form.portsHost')}
              aria-label={t('create.form.portsHost')}
              className="w-24 font-mono"
            />
            <Caption className="text-muted-foreground">:</Caption>
            <Input
              value={row.container}
              onChange={(e) => updateList(setPorts, id, { ...row, container: e.target.value })}
              placeholder={t('create.form.portsContainer')}
              aria-label={t('create.form.portsContainer')}
              className="w-24 font-mono"
            />
            <Select
              value={row.protocol}
              onChange={(v) => updateList(setPorts, id, { ...row, protocol: v as 'tcp' | 'udp' })}
              options={[
                { value: 'tcp', label: t('create.form.portsProtocol') + ' TCP' },
                { value: 'udp', label: t('create.form.portsProtocol') + ' UDP' },
              ]}
              className="w-20"
              searchable={false}
              ariaLabel={t('create.form.portsProtocol')}
            />
            <Button
              variant="ghost"
              size="icon"
              aria-label={t('repeater.removeRowLabel', { ns: 'common' })}
              onClick={remove}
            >
              <IconTrash className="size-4 text-muted-foreground" />
            </Button>
          </Flex>
        )}
      />
    </Section>
  );
}
