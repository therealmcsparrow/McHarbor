// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';

type RoleAssignmentStackPickerProps = {
  selectedStackNames: string[];
  stackFilter: string;
  onStackFilterChange: (value: string) => void;
  availableStackNames: string[];
  hasStacks: boolean;
  isLoading: boolean;
  onToggle: (stackName: string) => void;
  onSelectVisible: () => void;
  onClear: () => void;
};

export function RoleAssignmentStackPicker({
  selectedStackNames,
  stackFilter,
  onStackFilterChange,
  availableStackNames,
  hasStacks,
  isLoading,
  onToggle,
  onSelectVisible,
  onClear,
}: RoleAssignmentStackPickerProps) {
  const { t } = useTranslation('security');
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <label className="text-sm font-medium">{t('nav.stacks', { ns: 'common' })}</label>
        <span className="text-xs text-muted-foreground">
          {t('users.selectedStacksCount', { count: selectedStackNames.length })}
        </span>
      </div>

      <Input
        value={stackFilter}
        onChange={(evt) => onStackFilterChange(evt.target.value)}
        placeholder={t('select.searchPlaceholder', { ns: 'common' })}
        variant="outline"
      />

      <div className="flex items-center justify-between gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onSelectVisible}
          disabled={availableStackNames.length === 0}
        >
          {t('users.selectVisibleStacks')}
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={onClear} disabled={selectedStackNames.length === 0}>
          {t('actions.clear', { ns: 'common' })}
        </Button>
      </div>

      <div className="max-h-56 space-y-2 overflow-y-auto rounded-lg border border-border p-3">
        {isLoading ? (
          <p className="text-sm text-muted-foreground">{t('dataGrid.loading', { ns: 'common' })}</p>
        ) : !hasStacks ? (
          <p className="text-sm text-muted-foreground">{t('users.noStacksInEnvironment')}</p>
        ) : availableStackNames.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('users.noStacksMatch')}</p>
        ) : (
          availableStackNames.map((stackName) => (
            <label
              key={stackName}
              className="flex cursor-pointer items-center gap-3 rounded-md px-2 py-1.5 hover:bg-muted/30"
            >
              <Switch checked={selectedStackNames.includes(stackName)} onCheckedChange={() => onToggle(stackName)} />
              <span className="text-sm text-foreground">{stackName}</span>
            </label>
          ))
        )}
      </div>

      <p className="text-xs text-muted-foreground">{t('users.stackScopeHint')}</p>
    </div>
  );
}
