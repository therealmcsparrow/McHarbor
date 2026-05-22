// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback } from 'react';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';

type KeyValueEditorProps = {
  entries: Array<{ key: string; value: string }>;
  onChange: (entries: Array<{ key: string; value: string }>) => void;
  keyLabel: string;
  valueLabel: string;
  addLabel: string;
};

export function KeyValueEditor({ entries, onChange, keyLabel, valueLabel, addLabel }: KeyValueEditorProps) {
  const entryKeys = useStableListKeys(entries, (entry) => `${entry.key}\u0000${entry.value}`);

  const handleChange = useCallback(
    (index: number, field: 'key' | 'value', val: string) => {
      const updated = entries.map((entry, itemIndex) => (itemIndex === index ? { ...entry, [field]: val } : entry));
      onChange(updated);
    },
    [entries, onChange],
  );

  const handleAdd = useCallback(() => {
    onChange([...entries, { key: '', value: '' }]);
  }, [entries, onChange]);

  const handleRemove = useCallback(
    (index: number) => {
      onChange(entries.filter((_, itemIndex) => itemIndex !== index));
    },
    [entries, onChange],
  );

  return (
    <div className="space-y-2">
      {entries.length > 0 && (
        <div className="grid grid-cols-[1fr_1fr_auto] gap-2 text-xs font-medium text-muted-foreground">
          <span>{keyLabel}</span>
          <span>{valueLabel}</span>
          <span className="w-8" />
        </div>
      )}
      {entries.map((entry, index) => (
        <div key={entryKeys[index]} className="grid grid-cols-[1fr_1fr_auto] gap-2">
          <input
            type="text"
            value={entry.key}
            onChange={(event) => handleChange(index, 'key', event.target.value)}
            className="rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
            placeholder={keyLabel}
          />
          <input
            type="text"
            value={entry.value}
            onChange={(event) => handleChange(index, 'value', event.target.value)}
            className="rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
            placeholder={valueLabel}
          />
          <Button
            variant="ghost"
            size="icon"
            onClick={() => handleRemove(index)}
            aria-label="Remove"
            className="size-7 text-muted-foreground hover:text-red-500"
          >
            <IconTrash className="size-3.5" />
          </Button>
        </div>
      ))}
      <Button variant="outline" size="sm" onClick={handleAdd} className="mt-1">
        <IconPlus className="mr-1 size-3.5" />
        {addLabel}
      </Button>
    </div>
  );
}
