// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { Button } from "@resources/components/ui/Button";
import { ct } from "../canvas-theme";

type ValueTreeProps = {
  data: unknown;
  depth?: number;
  maxAutoExpand?: number;
};

function getArrayItemKey(item: unknown, index: number) {
  if (Array.isArray(item)) {
    return `array-${index + 1}-${item.length}`;
  }
  if (item && typeof item === "object") {
    return `object-${index + 1}-${Object.keys(item as Record<string, unknown>).join("|")}`;
  }
  return `value-${index + 1}-${String(item)}`;
}

export function ValueTree({
  data,
  depth = 0,
  maxAutoExpand = 1,
}: ValueTreeProps) {
  if (data === null || data === undefined) {
    return <span className={`${ct.text30} italic`}>null</span>;
  }

  if (typeof data === "string") {
    return <span className="text-emerald-400">&quot;{data}&quot;</span>;
  }

  if (typeof data === "number") {
    return <span className="text-blue-400">{data}</span>;
  }

  if (typeof data === "boolean") {
    return <span className="text-purple-400">{data ? "true" : "false"}</span>;
  }

  if (Array.isArray(data)) {
    return (
      <CollapsibleNode
        label={`Array (${data.length})`}
        depth={depth}
        maxAutoExpand={maxAutoExpand}
      >
        {data.map((item, i) => (
          <div
            key={getArrayItemKey(item, i)}
            className="flex items-start gap-1.5"
          >
            <span className="shrink-0 text-muted-foreground/50">{i} :</span>
            <ValueTree
              data={item}
              depth={depth + 1}
              maxAutoExpand={maxAutoExpand}
            />
          </div>
        ))}
      </CollapsibleNode>
    );
  }

  if (typeof data === "object") {
    const entries = Object.entries(data as Record<string, unknown>);
    return (
      <CollapsibleNode
        label={`Object {${entries.length}}`}
        depth={depth}
        maxAutoExpand={maxAutoExpand}
      >
        {entries.map(([key, value]) => (
          <div key={key} className="flex items-start gap-1.5">
            <span className="shrink-0 text-cyan-400/70">{key} :</span>
            <ValueTree
              data={value}
              depth={depth + 1}
              maxAutoExpand={maxAutoExpand}
            />
          </div>
        ))}
      </CollapsibleNode>
    );
  }

  return <span className={ct.text50}>{String(data)}</span>;
}

function CollapsibleNode({
  label,
  depth,
  maxAutoExpand,
  children,
}: {
  label: string;
  depth: number;
  maxAutoExpand: number;
  children: React.ReactNode;
}) {
  const [expanded, setExpanded] = useState(depth < maxAutoExpand);

  return (
    <div>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={() => setExpanded(!expanded)}
        className="h-auto gap-0.5 px-0 py-0 text-muted-foreground hover:bg-transparent hover:text-foreground"
      >
        {expanded ? (
          <IconChevronDown className="size-2.5 shrink-0" />
        ) : (
          <IconChevronRight className="size-2.5 shrink-0" />
        )}
        <span className={ct.text40}>{label}</span>
      </Button>
      {expanded && (
        <div
          className={`border-l border-white/5 pl-2 space-y-0.5 ${depth === 0 ? "" : "ml-1.5"}`}
        >
          {children}
        </div>
      )}
    </div>
  );
}
