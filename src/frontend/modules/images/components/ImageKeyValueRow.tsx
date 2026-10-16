// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ImageKeyValueRow — one mono `key → value` row.
//
// The Config card's environment list and the Labels card each had their own
// copy of this markup, and each carried the same two faults: the key was
// `shrink-0` with no ceiling, so a long key took the row before the value got
// any of it, and the value's `truncate` could never fire because a flex item
// keeps `min-width: auto` and refuses to shrink below its own content — so
// instead of ellipsising, the value ran off the panel.
//
// Same division of width as `InfoRow`'s `spread` variant, in the mono face
// these two lists use: key left at its natural width and never cut, value
// pushed to the right edge and ellipsised, full text on hover.

type ImageKeyValueRowProps = {
  itemKey: string;
  value: string;
};

export function ImageKeyValueRow({ itemKey, value }: ImageKeyValueRowProps) {
  return (
    <div className="flex min-w-0 items-baseline gap-4 border-b border-border py-1.5 last:border-0">
      <span className="shrink-0 font-mono text-xs font-medium break-all text-foreground">{itemKey}</span>
      <span title={value} className="ml-auto min-w-0 truncate text-right font-mono text-xs text-muted-foreground">
        {value}
      </span>
    </div>
  );
}
