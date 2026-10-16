// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  useStack,
  useStackEnvVars,
  useStackEnvVarsRevealed,
  useUpdateStack,
  useUpdateStackEnvVars,
} from '@resources/hooks/useStacks';
import { PromoteToSecretDialog } from '@resources/components/secrets/PromoteToSecretDialog';
import { EnvironmentVarRow } from './EnvironmentVarRow';
import { EnvironmentTabEditor } from './EnvironmentTabEditor';
import { isRedactedValue } from '@resources/utils/env-redaction';

type EnvironmentTabProps = {
  stackName: string;
  editing?: boolean;
  readOnly?: boolean;
};

export function EnvironmentTab({ stackName, editing, readOnly = false }: EnvironmentTabProps) {
  const { t } = useTranslation('stacks');
  const { data: envVars, isLoading } = useStackEnvVars(stackName);
  const updateEnvVars = useUpdateStackEnvVars();
  const { data: stack } = useStack(stackName);
  const updateStack = useUpdateStack();
  const [visibleKeys, setVisibleKeys] = useState<Set<string>>(new Set());
  // Values the backend considers credentials arrive as a mask sentinel. The
  // real ones are fetched only once the user actually asks to see one, and
  // that request is audit-logged server-side.
  const wantsReveal = [...visibleKeys].some((key) => isRedactedValue(envVars?.[key]));
  const { data: revealedVars } = useStackEnvVarsRevealed(stackName, wantsReveal);
  // "Promote to secret" needs the plaintext to store, so the row has to be
  // revealed first — which is why the action is only offered on a visible row.
  const [promoteTarget, setPromoteTarget] = useState<{ key: string; value: string } | null>(null);
  const [editEntries, setEditEntries] = useState<Array<{ key: string; value: string }>>([]);
  const [seededFrom, setSeededFrom] = useState<{ envVars: typeof envVars; editing: boolean | undefined }>({
    envVars: undefined,
    editing: false,
  });
  if (seededFrom.envVars !== envVars || seededFrom.editing !== editing) {
    setSeededFrom({ envVars, editing });
    if (envVars && editing) {
      setEditEntries(
        Object.entries(envVars)
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([key, value]) => ({ key, value })),
      );
    }
  }

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
      <EnvironmentTabEditor
        entries={editEntries}
        onEntriesChange={setEditEntries}
        onSave={handleSave}
        saving={updateEnvVars.isPending}
        secretFileMode={stack?.secretFileMode ?? false}
        secretFileModeDisabled={!stack || updateStack.isPending}
        onSecretFileModeChange={(secretFileMode) => updateStack.mutate({ name: stackName, secretFileMode })}
      />
    );
  }

  const entries = Object.entries(envVars ?? {}).sort(([a], [b]) => a.localeCompare(b));

  if (entries.length === 0) {
    return <div className="py-12 text-center text-sm text-muted-foreground">{t('detail.noEnvVars')}</div>;
  }

  return (
    <div className="overflow-hidden rounded-lg border border-border">
      <table className="w-full text-sm">
        <thead className="bg-muted">
          <tr className="border-b border-border bg-muted/50">
            <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('detail.key')}</th>
            <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('detail.value')}</th>
            <th className="w-12 px-2 py-2.5">
              <span className="sr-only">{t('columns.actions')}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {entries.map(([key, value]) => (
            <EnvironmentVarRow
              key={key}
              envKey={key}
              maskedValue={value}
              revealedValue={revealedVars?.[key]}
              visible={visibleKeys.has(key)}
              onToggleVisibility={toggleVisibility}
              onPromote={(promoteKey, promoteValue) => setPromoteTarget({ key: promoteKey, value: promoteValue })}
            />
          ))}
        </tbody>
      </table>

      {promoteTarget && (
        <PromoteToSecretDialog
          open
          onOpenChange={(next) => !next && setPromoteTarget(null)}
          envKey={promoteTarget.key}
          value={promoteTarget.value}
          stackName={stackName}
          onPromoted={(reference) => {
            // Write the reference straight back so the editor shows what the
            // deploy will actually resolve.
            updateEnvVars.mutate({
              name: stackName,
              envVars: Object.fromEntries(
                entries.map(([entryKey, entryValue]) =>
                  entryKey === promoteTarget.key ? [entryKey, reference] : [entryKey, entryValue],
                ),
              ),
            });
            setPromoteTarget(null);
          }}
        />
      )}
    </div>
  );
}
