// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconPlayerPlayFilled,
  IconCheck,
  IconX,
  IconLoader2,
  IconPlayerTrackNext,
  IconBan,
} from "@tabler/icons-react";
import { cn } from "@resources/utils/cn";
import { Button } from "@resources/components/ui/Button";
import type { CanvasNode, NodeExecutionStatus } from "../types";
import {
  getEffectiveInputPorts,
  getEffectiveOutputPorts,
  getNodeHeightForPorts,
  JUNCTION_SIZE,
} from "../types";
import {
  CATEGORY_COLORS,
  CATEGORY_GLOW_RGB,
  NODE_DEFINITION_MAP,
} from "../nodes";
import { ct } from "../canvas-theme";

const NODE_WIDTH = 224;

type WorkflowNodeProps = {
  node: CanvasNode;
  isSelected: boolean;
  isConnecting: boolean;
  isExecuting?: boolean;
  executionStatus?: NodeExecutionStatus;
  onSelect: (e: React.MouseEvent) => void;
  onDragStart: (nodeId: string, e: React.MouseEvent) => void;
  onPortDragStart: (nodeId: string, port: string, x: number, y: number) => void;
  onPortDrop: (nodeId: string, port: string) => void;
  onContextMenu: (nodeId: string, e: React.MouseEvent) => void;
  onPortContextMenu: (
    nodeId: string,
    portKey: string,
    e: React.MouseEvent,
  ) => void;
  onRun?: (nodeId: string) => void;
};

export function WorkflowNode({
  node,
  isSelected,
  isConnecting,
  isExecuting,
  executionStatus,
  onSelect,
  onDragStart,
  onPortDragStart,
  onPortDrop,
  onContextMenu,
  onPortContextMenu,
  onRun,
}: WorkflowNodeProps) {
  const { t } = useTranslation("common");
  const definition = NODE_DEFINITION_MAP[node.action];
  const category = definition?.category ?? node.type;
  const colors = CATEGORY_COLORS[category] ?? {
    header: "bg-blue-600",
    border: "border-blue-500/30",
  };
  const glowRgb = CATEGORY_GLOW_RGB[category] ?? "96, 165, 250";

  const inputPorts = useMemo(
    () => getEffectiveInputPorts(node, definition),
    [node, definition],
  );
  const outputPorts = useMemo(
    () => getEffectiveOutputPorts(node, definition),
    [node, definition],
  );
  const nodeHeight = getNodeHeightForPorts(
    Math.max(inputPorts.length, outputPorts.length),
  );

  const isRedPort = (port: string) => {
    if (port === "error") return true;
    if (node.action !== "condition") return false;
    return port === outputPorts[outputPorts.length - 1];
  };

  const isGreenPort = (port: string) =>
    node.action === "loop" && port === "done";

  // Track which port is hovered for glow + tooltip
  const [hoveredPort, setHoveredPort] = useState<string | null>(null);

  const getPortLabel = (portKey: string, fallback: string) =>
    node.portLabels?.[portKey] || fallback;

  const inputPortClass = (port: string) => {
    if (node.blockedPorts?.includes(`in:${port}`))
      return "border-dashed border-red-400/40 bg-red-400/10";
    if (hoveredPort === `in:${port}`)
      return "border-blue-400 bg-blue-400/30 ring-2 ring-blue-400/60 scale-150";
    if (isConnecting)
      return "border-blue-400 bg-blue-400/20 ring-2 ring-blue-400/50 animate-pulse scale-125";
    // see canvas-theme.ts
    return `${ct.border30} ${ct.portBg} group-hover:border-foreground/60 group-hover:bg-foreground/20`;
  };

  const outputPortClass = (port: string) => {
    if (node.blockedPorts?.includes(`out:${port}`))
      return "border-dashed border-red-400/40 bg-red-400/10";
    if (hoveredPort === `out:${port}`)
      return "border-blue-400 bg-blue-400/30 ring-2 ring-blue-400/60 scale-150";
    if (isRedPort(port)) return "border-red-400/50 bg-red-400/20";
    if (isGreenPort(port)) return "border-emerald-400/50 bg-emerald-400/20";
    // see canvas-theme.ts
    return `${ct.border30} ${ct.portBg} group-hover:border-foreground/60 group-hover:bg-foreground/20`;
  };

  const configSummary = useMemo(() => {
    const config = node.config;
    if (!config || Object.keys(config).length === 0) return "";
    return Object.values(config)
      .slice(0, 2)
      .map((v) => String(v).substring(0, 30))
      .join(", ");
  }, [node.config]);

  const executionRingClass =
    executionStatus === "running"
      ? "ring-2 ring-blue-400 animate-pulse"
      : executionStatus === "completed"
        ? "ring-2 ring-emerald-400"
        : executionStatus === "failed"
          ? "ring-2 ring-red-400"
          : "";

  const glowStyle: React.CSSProperties = isSelected
    ? {
        boxShadow: `0 0 20px rgba(${glowRgb}, 0.4), 0 0 0 2px rgba(${glowRgb}, 0.4)`,
      }
    : {};

  const onNodeMouseDown = (e: React.MouseEvent) => {
    if (e.button === 1) return; // middle-click — let canvas handle pan
    onSelect(e);
    onDragStart(node.id, e);
  };

  const getOutputPortPos = (index: number) => ({
    x:
      node.position.x +
      (node.action === "junction" ? JUNCTION_SIZE : NODE_WIDTH),
    y:
      node.position.y +
      ((node.action === "junction" ? JUNCTION_SIZE : nodeHeight) /
        (outputPorts.length + 1)) *
        (index + 1),
  });

  const getPortOffset = (count: number, index: number) =>
    (nodeHeight / (count + 1)) * (index + 1) - 12;

  // --- Junction: compact dot rendering ---
  if (node.action === "junction") {
    const half = JUNCTION_SIZE / 2;
    return (
      <div
        className="group absolute size-5 cursor-grab select-none"
        style={{ left: node.position.x, top: node.position.y }}
        onMouseDown={onNodeMouseDown}
        onContextMenu={(e) => {
          e.preventDefault();
          e.stopPropagation();
          onContextMenu(node.id, e);
        }}
      >
        {/* Dot */}
        <div
          className={cn(
            "size-5 rounded-full bg-slate-500 border-2 border-slate-400/60 transition-shadow duration-200",
            executionRingClass,
            node.disabled && "opacity-40",
          )}
          style={
            isSelected
              ? {
                  boxShadow:
                    "0 0 12px rgba(100,116,139,0.6), 0 0 0 2px rgba(100,116,139,0.5)",
                }
              : {}
          }
        />
        {/* Input port (hit area pushed outside the dot) */}
        <div
          className="absolute flex h-4 w-3 cursor-default items-center justify-center"
          style={{ left: -6, top: half - 8 }}
          onMouseEnter={() => setHoveredPort("in:input")}
          onMouseLeave={() => setHoveredPort(null)}
          onMouseUp={(e) => {
            e.stopPropagation();
            if (!node.blockedPorts?.includes("in:input")) {
              onPortDrop(node.id, "input");
            }
          }}
          onContextMenu={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onPortContextMenu(node.id, "in:input", e);
          }}
        >
          <div
            className={cn(
              "size-2 rounded-full border-2 transition-all duration-150",
              inputPortClass("input"),
            )}
          />
        </div>
        {/* Output port (hit area pushed outside the dot) */}
        <div
          className="absolute flex h-4 w-3 cursor-default items-center justify-center"
          style={{ right: -6, top: half - 8 }}
          onMouseEnter={() => setHoveredPort("out:output")}
          onMouseLeave={() => setHoveredPort(null)}
          onMouseDown={(e) => {
            e.stopPropagation();
            if (!node.blockedPorts?.includes("out:output")) {
              onPortDragStart(
                node.id,
                "output",
                node.position.x + JUNCTION_SIZE,
                node.position.y + half,
              );
            }
          }}
          onContextMenu={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onPortContextMenu(node.id, "out:output", e);
          }}
        >
          <div
            className={cn(
              "size-2 rounded-full border-2 transition-all duration-150",
              outputPortClass("output"),
            )}
          />
        </div>
        {/* Execution status indicators */}
        {executionStatus === "running" && (
          <IconLoader2 className="absolute -top-2 -right-2 size-3 animate-spin text-blue-400" />
        )}
        {executionStatus === "completed" && (
          <IconCheck className="absolute -top-2 -right-2 size-3 text-emerald-400" />
        )}
        {executionStatus === "failed" && (
          <IconX className="absolute -top-2 -right-2 size-3 text-red-400" />
        )}
      </div>
    );
  }

  return (
    <div
      className={cn(
        "group absolute flex w-56 cursor-grab flex-col select-none rounded-lg border shadow-lg shadow-black/20 transition-shadow duration-300",
        colors.border,
        executionRingClass,
        node.disabled && "opacity-40",
        node.skip && "border-dashed",
      )}
      style={{
        left: node.position.x,
        top: node.position.y,
        height: nodeHeight,
        ...glowStyle,
      }}
      onMouseDown={onNodeMouseDown}
      onContextMenu={(e) => {
        e.preventDefault();
        e.stopPropagation();
        onContextMenu(node.id, e);
      }}
    >
      {/* Header */}
      <div
        className={cn(
          `flex items-center gap-2 rounded-t-lg px-3 py-2 text-xs font-medium ${ct.nodeHeaderText}`,
          colors.header,
          node.skip && "bg-opacity-60",
        )}
      >
        <span
          className={cn(
            "truncate flex-1",
            node.skip && "line-through opacity-70",
          )}
        >
          {node.label}
        </span>
        {node.skip && (
          <IconPlayerTrackNext
            className="size-3 text-amber-300"
            title={t("workflows.skipped")}
          />
        )}
        {node.disabled && (
          <IconBan
            className="size-3 text-red-300"
            title={t("workflows.disabled")}
          />
        )}
        {node.debug && !node.skip && !node.disabled && (
          <span className="text-[10px] opacity-60">DBG</span>
        )}
        {executionStatus === "running" && (
          <IconLoader2 className={`size-3 animate-spin ${ct.text80}`} />
        )}
        {executionStatus === "completed" && (
          <IconCheck className="size-3 text-emerald-300" />
        )}
        {executionStatus === "failed" && (
          <IconX className="size-3 text-red-300" />
        )}
      </div>

      {/* Body */}
      <div className={`flex-1 rounded-b-lg ${ct.nodeBg} px-3 py-2.5`}>
        {configSummary ? (
          <p className={`truncate text-[0.625rem] ${ct.text40}`}>
            {configSummary}
          </p>
        ) : (
          <p className={`text-[0.625rem] ${ct.text25} italic`}>
            {t("workflows.noConfiguration")}
          </p>
        )}
      </div>

      {/* Input ports */}
      {inputPorts.map((port, i) => (
        <div
          key={`in-${port}`}
          className="absolute flex h-6 w-6 cursor-default items-center justify-center"
          style={{ left: -12, top: getPortOffset(inputPorts.length, i) }}
          onMouseEnter={() => setHoveredPort(`in:${port}`)}
          onMouseLeave={() => setHoveredPort(null)}
          onMouseDown={(e) => e.stopPropagation()}
          onMouseUp={(e) => {
            e.stopPropagation();
            if (!node.blockedPorts?.includes(`in:${port}`))
              onPortDrop(node.id, port);
          }}
          onContextMenu={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onPortContextMenu(node.id, `in:${port}`, e);
          }}
        >
          <div
            className={cn(
              "size-2.5 rounded-full border-2 transition-all duration-150",
              inputPortClass(port),
            )}
          />
          {hoveredPort === `in:${port}` && (
            <div
              className={`absolute right-full mr-1.5 whitespace-nowrap rounded ${ct.tooltipBg} px-1.5 py-0.5 text-[9px] ${ct.text80} pointer-events-none`}
            >
              {getPortLabel(`in:${port}`, port)}
            </div>
          )}
        </div>
      ))}

      {/* Output ports */}
      {outputPorts.map((port, i) => (
        <div
          key={`out-${port}`}
          className="absolute flex h-6 w-6 cursor-default items-center justify-center"
          style={{ right: -12, top: getPortOffset(outputPorts.length, i) }}
          onMouseEnter={() => setHoveredPort(`out:${port}`)}
          onMouseLeave={() => setHoveredPort(null)}
          onMouseDown={(e) => {
            e.stopPropagation();
            if (!node.blockedPorts?.includes(`out:${port}`)) {
              const pos = getOutputPortPos(i);
              onPortDragStart(node.id, port, pos.x, pos.y);
            }
          }}
          onContextMenu={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onPortContextMenu(node.id, `out:${port}`, e);
          }}
        >
          <div
            className={cn(
              "size-2.5 rounded-full border-2 transition-all duration-150",
              outputPortClass(port),
            )}
          />
          {hoveredPort === `out:${port}` && (
            <div
              className={`absolute left-full ml-1.5 whitespace-nowrap rounded ${ct.tooltipBg} px-1.5 py-0.5 text-[9px] ${ct.text80} pointer-events-none`}
            >
              {getPortLabel(`out:${port}`, port)}
            </div>
          )}
        </div>
      ))}

      {/* Trigger run button */}
      {node.action === "manual-trigger" && onRun && (
        <Button
          type="button"
          size="icon-sm"
          disabled={isExecuting}
          aria-label={
            isExecuting
              ? t("workflows.workflowRunning")
              : t("workflows.runWorkflow")
          }
          className={cn(
            `absolute rounded-full ${ct.runBtnText} shadow-md transition-all`,
            isExecuting
              ? "cursor-not-allowed bg-slate-600 ring-2 ring-emerald-500"
              : "cursor-pointer bg-emerald-600 hover:scale-110 hover:bg-emerald-500",
          )}
          style={{ left: -36, top: nodeHeight / 2 - 14 }}
          title={
            isExecuting
              ? t("workflows.workflowRunning")
              : t("workflows.runWorkflow")
          }
          onMouseDown={(e) => e.stopPropagation()}
          onClick={(e) => {
            e.stopPropagation();
            onRun(node.id);
          }}
        >
          <IconPlayerPlayFilled className="size-3.5" />
        </Button>
      )}
    </div>
  );
}
