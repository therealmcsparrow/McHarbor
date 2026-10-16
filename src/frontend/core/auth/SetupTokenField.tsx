// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { readSetupToken, writeSetupToken } from './setupToken';

export function SetupTokenField() {
  const { t } = useTranslation('common');
  const [value, setValue] = useState(readSetupToken);

  return (
    <div className="space-y-1.5">
      <label htmlFor="setup-token" className="text-sm font-medium text-foreground">
        {t('setupToken.label')}
      </label>
      <Input
        id="setup-token"
        value={value}
        autoComplete="off"
        spellCheck={false}
        className="font-mono"
        onChange={(evt) => {
          setValue(evt.target.value);
          writeSetupToken(evt.target.value);
        }}
      />
      <p className="text-xs text-muted-foreground">{t('setupToken.hint')}</p>
    </div>
  );
}
