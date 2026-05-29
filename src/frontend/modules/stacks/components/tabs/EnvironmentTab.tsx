// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye, IconEyeOff, IconDeviceFloppy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { KeyValueEditor } from '@resources/components/KeyValueEditor';
import { useStackEnvVars, useUpdateStackEnvVars } from '../../hooks/useStacks';

type EnvironmentTabProps = {
  stackName: string;
  editing?: boolean;
  readOnly?: boolean;
};

export function EnvironmentTab({ stackName, editing, readOnly = false }: EnvironmentTabProps) {
  const { t } = useTranslation('stacks');
  const { data: envVars, isLoading } = useStackEnvVars(stackName);
  const updateEnvVars = useUpdateStackEnvVars();
  const [visibleKeys, setVisibleKeys] = useState<Set<string>>(new Set());
  const [editEntries, setEditEntries] = useState<Array<{ key: string; value: string }>>([]);

  useEffect(() => {
    if (envVars && editing) {
      const sorted = Object.entries(envVars)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([key, value]) => ({ key, value }));
      setEditEntries(sorted);
    }
  }, [envVars, editing]);

  const handleSave = useCallback(() => {
    const vars: Record<string, string> = {};
    for (const entry of editEntries) {
      if (entry.key.trim()) {
        vars[entry.key.trim()] = entry.value;
      }
    }
    updateEnvVars.mutate({ name: stackName, envVars: vars });
  }, [editEntries, stackName, updateEnvVars]);

  const toggleVisibility = (key: string) => {
    setVisibleKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  if (editing && !readOnly) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium">{t('detail.envVars')}</h3>
          <Button size="sm" onClick={handleSave} disabled={updateEnvVars.isPending}>
            <IconDeviceFloppy className="mr-1 size-3.5" />
            {updateEnvVars.isPending ? t('editStack.saving') : t('editStack.saveStack')}
          </Button>
        </div>
        <KeyValueEditor
          entries={editEntries}
          onChange={setEditEntries}
          keyLabel={t('detail.key')}
          valueLabel={t('detail.value')}
          addLabel={t('detail.key')}
        />
      </div>
    );
  }

  const entries = Object.entries(envVars ?? {}).sort(([a], [b]) => a.localeCompare(b));

  if (entries.length === 0) {
    return (
      <div className="py-12 text-center text-sm text-muted-foreground">
        {t('detail.noEnvVars')}
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border border-border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border bg-muted/50">
            <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">
              {t('detail.key')}
            </th>
            <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">
              {t('detail.value')}
            </th>
            <th className="w-12 px-2 py-2.5" />
          </tr>
        </thead>
        <tbody>
          {entries.map(([key, value]) => {
            const visible = visibleKeys.has(key);
            return (
              <tr key={key} className="border-b border-border last:border-0 hover:bg-muted/30">
                <td className="px-4 py-2.5 font-mono text-xs">{key}</td>
                <td className="px-4 py-2.5 font-mono text-xs text-muted-foreground">
                  {visible ? value : '••••••••'}
                </td>
                <td className="px-2 py-2.5">
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => toggleVisibility(key)}
                      >
                        {visible ? (
                          <IconEyeOff className="h-3.5 w-3.5 text-muted-foreground" />
                        ) : (
                          <IconEye className="h-3.5 w-3.5 text-muted-foreground" />
                        )}
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>
                      {visible ? t('detail.hideValue') : t('detail.showValue')}
                    </TooltipContent>
                  </Tooltip>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
