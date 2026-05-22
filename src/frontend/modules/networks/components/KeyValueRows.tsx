// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconPlus, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';

interface KeyValuePair {
  key: string;
  value: string;
}

interface KeyValueRowsProps {
  items: KeyValuePair[];
  onChange: (items: KeyValuePair[]) => void;
  keyPlaceholder?: string;
  valuePlaceholder?: string;
  addLabel?: string;
}

export type { KeyValuePair };

export function KeyValueRows({
  items,
  onChange,
  keyPlaceholder = 'Key',
  valuePlaceholder = 'Value',
  addLabel = 'Add',
}: KeyValueRowsProps) {
  return (
    <div className="space-y-2">
      {items.map((item, i) => (
        <div key={`kv-${i}`} className="flex gap-2">
          <Input
            variant="outline"
            type="text"
            value={item.key}
            onChange={(e) => {
              const next = [...items];
              next[i] = { key: e.target.value, value: item.value };
              onChange(next);
            }}
            placeholder={keyPlaceholder}
            className="flex-1"
          />
          <Input
            variant="outline"
            type="text"
            value={item.value}
            onChange={(e) => {
              const next = [...items];
              next[i] = { key: item.key, value: e.target.value };
              onChange(next);
            }}
            placeholder={valuePlaceholder}
            className="flex-1"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label="Remove row"
            onClick={() => onChange(items.filter((_, j) => j !== i))}
          >
            <IconTrash className="h-3.5 w-3.5 text-muted-foreground" />
          </Button>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => onChange([...items, { key: '', value: '' }])}
      >
        <IconPlus className="h-3.5 w-3.5" /> {addLabel}
      </Button>
    </div>
  );
}
