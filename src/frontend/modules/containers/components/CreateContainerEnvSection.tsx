// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Dispatch, SetStateAction } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTrash, IconVariable } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Flex } from '@resources/components/ui/Layout';
import { Section } from '@resources/components/ui/Form';
import { RepeaterRows } from '@resources/components/ui/RepeaterRows';
import { SecretFileModeToggle } from '@resources/components/secrets/SecretFileModeToggle';
import { updateList, type EnvRow } from './createContainerFormHelpers';

type CreateContainerEnvSectionProps = {
  env: EnvRow[];
  setEnv: Dispatch<SetStateAction<EnvRow[]>>;
  secretFileMode: boolean;
  setSecretFileMode: (v: boolean) => void;
};

export function CreateContainerEnvSection({
  env,
  setEnv,
  secretFileMode,
  setSecretFileMode,
}: CreateContainerEnvSectionProps) {
  const { t } = useTranslation('containers');
  return (
    <Section icon={<IconVariable className="size-3.5" />} title={t('create.form.env')} help={t('create.form.envHelp')}>
      <RepeaterRows<EnvRow>
        items={env}
        onChange={setEnv}
        onAdd={() => ({ key: '', value: '' })}
        addLabel={t('create.form.addEnv')}
        renderRow={(row, _idx, { id, remove }) => (
          <Flex gap={2} align="center">
            <Input
              value={row.key}
              onChange={(e) => updateList(setEnv, id, { ...row, key: e.target.value })}
              placeholder={t('placeholders.envKey')}
              aria-label={t('create.envEditor.key')}
              className="w-44 font-mono"
            />
            <Input
              value={row.value}
              onChange={(e) => updateList(setEnv, id, { ...row, value: e.target.value })}
              placeholder={t('placeholders.envValue')}
              aria-label={t('create.envEditor.value')}
              className="flex-1 font-mono"
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

      <div className="mt-3 border-t border-border pt-3">
        <SecretFileModeToggle checked={secretFileMode} onCheckedChange={setSecretFileMode} />
      </div>
    </Section>
  );
}
