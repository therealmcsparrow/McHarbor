// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys consumed by this primitive (callers should add them to common.json):
//   common:repeater.{removeRow,removeRowLabel,empty}

import { useCallback, useMemo, type ReactNode } from 'react';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { Box, Flex } from './Layout';
import { Caption } from './Typography';
import { Button } from './Button';
import { cn } from '@resources/utils/cn';

/**
 * RepeaterRows - a small primitive for "list of homogeneous rows + add button"
 * patterns (port mappings, env vars, volume mounts, etc.).
 *
 * The parent owns the data; this component assigns a stable id per item
 * behind the scenes and hands it to the row renderer via the
 * `helpers.id` argument so callers can use it as a key + pass it back
 * to callbacks like `remove`.
 */
type RepeaterRowsRenderArgs = {
  /** Stable id for this row — use as the React key when rendering. */
  id: string;
  /** Remove this row from the list. */
  remove: () => void;
};

type RepeaterEmptyState = {
  icon?: ReactNode;
  title: string;
  description?: string;
};

type RepeaterRowsProps<T> = {
  items: T[];
  onChange: (items: T[]) => void;
  renderRow: (item: T, index: number, helpers: RepeaterRowsRenderArgs) => ReactNode;
  onAdd: () => T;
  /** Label of the trailing "Add row" button. */
  addLabel: string;
  /** Optional empty state shown when items is empty. */
  emptyState?: RepeaterEmptyState;
  className?: string;
  rowClassName?: string;
};

interface Identified {
  __id?: string;
}

/**
 * RepeaterRows renders a stack of rows, each driven by `renderRow`, with a
 * trailing "add" button and an optional empty-state block. State management
 * stays in the parent; this component is purely presentational and is
 * responsible for assigning stable ids + handling removal.
 */
export function RepeaterRows<T extends object>({
  items,
  onChange,
  renderRow,
  onAdd,
  addLabel,
  emptyState,
  className,
  rowClassName,
}: RepeaterRowsProps<T>) {
  const ids = useIdMap(items);

  const handleRemove = useCallback(
    (id: string) => {
      const idx = ids.indexOf(id);
      if (idx === -1) return;
      onChange(items.filter((_, i) => i !== idx));
    },
    [items, ids, onChange],
  );

  const handleAdd = useCallback(() => {
    onChange([...items, onAdd()]);
  }, [items, onAdd, onChange]);

  const renderedRows = useMemo(
    () =>
      items.map((item, idx) => {
        const id = ids[idx] ?? mintId();
        return (
          <Box key={id} className={rowClassName}>
            {renderRow(item, idx, { id, remove: () => handleRemove(id) })}
          </Box>
        );
      }),
    [items, ids, rowClassName, renderRow, handleRemove],
  );

  return (
    <Box className={cn('space-y-2', className)}>
      {items.length === 0 && emptyState ? <RepeaterEmpty emptyState={emptyState} /> : renderedRows}
      <Flex justify="start">
        <Button variant="ghost" size="sm" onClick={handleAdd} type="button">
          <IconPlus className="size-4" />
          {addLabel}
        </Button>
      </Flex>
    </Box>
  );
}

function RepeaterEmpty({ emptyState }: { emptyState: RepeaterEmptyState }) {
  return (
    <Flex
      direction="col"
      align="center"
      justify="center"
      gap={1}
      className="rounded-md border border-dashed border-border bg-muted/20 px-3 py-4 text-center"
    >
      {emptyState.icon && <Box className="text-muted-foreground/70">{emptyState.icon}</Box>}
      <Caption className="text-foreground">{emptyState.title}</Caption>
      {emptyState.description && <Caption>{emptyState.description}</Caption>}
    </Flex>
  );
}

/**
 * useIdMap keeps a stable id per item so that list re-renders don't drop input
 * focus. An item carrying `__id` keeps it; otherwise the id is minted once and
 * remembered against the item object itself.
 */
const mintedIds = new WeakMap<object, string>();

function useIdMap<T extends object>(items: T[]): string[] {
  return items.map((item) => {
    const carried = (item as Identified).__id;
    if (typeof carried === 'string' && carried.length > 0) return carried;
    let id = mintedIds.get(item);
    if (!id) {
      id = mintId();
      mintedIds.set(item, id);
    }
    return id;
  });
}

let counter = 0;
function mintId(): string {
  counter += 1;
  return `r-${counter.toString(36)}`;
}

/**
 * Re-export of IconTrash for convenience — callers that render their own
 * remove button can import it next to `RepeaterRows`. Keeping it here avoids
 * creating a separate util file just for one icon.
 */
export { IconTrash };
