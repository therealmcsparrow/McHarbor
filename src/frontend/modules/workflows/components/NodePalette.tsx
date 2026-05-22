// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconSearch,
  IconChevronDown,
  IconChevronRight,
  IconApi,
  IconArrowBackUp,
  IconArrowBarToLeft,
  IconArrowBarToRight,
  IconArrowMerge,
  IconArrowMergeBoth,
  IconArrowsSplit,
  IconArrowsSplit2,
  IconBell,
  IconBellPlus,
  IconBellRinging,
  IconBox,
  IconBraces,
  IconBrandDiscord,
  IconBrandJavascript,
  IconBrandSlack,
  IconBrandTeams,
  IconBrandTelegram,
  IconBrandWhatsapp,
  IconBug,
  IconCalendar,
  IconCalendarClock,
  IconChartBar,
  IconChartDots,
  IconChartLine,
  IconCircleDot,
  IconClock,
  IconClockPause,
  IconCloudDownload,
  IconCloudUpload,
  IconCodeDots,
  IconCopyOff,
  IconDatabase,
  IconDatabaseExport,
  IconDatabaseImport,
  IconDatabaseMinus,
  IconDatabasePlus,
  IconDatabaseSearch,
  IconDatabaseX,
  IconDeviceDesktop,
  IconEdit,
  IconFileAlert,
  IconFileCode,
  IconFileDownload,
  IconFileText,
  IconFileUpload,
  IconFilter,
  IconFingerprint,
  IconGauge,
  IconGitBranch,
  IconHammer,
  IconHandClick,
  IconHandGrab,
  IconHandOff,
  IconHeartbeat,
  IconHeartRateMonitor,
  IconHourglass,
  IconHtml,
  IconId,
  IconInfoCircle,
  IconLayoutList,
  IconLetterCase,
  IconList,
  IconLockCode,
  IconMail,
  IconMathFunction,
  IconMessage,
  IconMessageCircle,
  IconNetwork,
  IconNetworkOff,
  IconPencil,
  IconPhoto,
  IconPhotoX,
  IconPlayerPlay,
  IconPlayerStop,
  IconPlugConnected,
  IconRepeat,
  IconRocket,
  IconRoute,
  IconSend,
  IconSend2,
  IconServer,
  IconShieldCheck,
  IconShieldCheckFilled,
  IconSortAscending,
  IconSql,
  IconSquarePlus,
  IconStack2,
  IconTable,
  IconTag,
  IconTemplate,
  IconTerminal,
  IconTerminal2,
  IconTopologyComplex,
  IconTopologyStarRing3,
  IconTransform,
  IconTrashX,
  IconUpload,
  IconVariable,
  IconWebhook,
  IconWifi,
  IconWorld,
  IconWorldSearch,
  IconZoomScan,
} from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import {
  CATEGORY_TAG_COLORS,
  getAllNodeDefinitions,
  isNodeDefinitionAvailable,
} from '../nodes';
import { useCustomNodeSync } from '../hooks/useCustomNodeSync';
import { useNodeCatalogAvailability } from '../hooks/useNodeCatalog';
import { useCustomNodeRegistry } from '../stores/custom-node-registry';
import { useNodeAvailability } from '@resources/hooks/useNodeAvailability';
import type { NodeDefinition } from '../types';

const ICON_MAP: Record<string, React.ComponentType<{ className?: string }>> = {
  IconApi,
  IconArrowBackUp,
  IconArrowBarToLeft,
  IconArrowBarToRight,
  IconArrowMerge,
  IconArrowMergeBoth,
  IconArrowsSplit,
  IconArrowsSplit2,
  IconBell,
  IconBellPlus,
  IconBellRinging,
  IconBox,
  IconBraces,
  IconBrandDiscord,
  IconBrandJavascript,
  IconBrandSlack,
  IconBrandTeams,
  IconBrandTelegram,
  IconBrandWhatsapp,
  IconBug,
  IconCalendar,
  IconCalendarClock,
  IconChartBar,
  IconChartDots,
  IconChartLine,
  IconCircleDot,
  IconClock,
  IconClockPause,
  IconCloudDownload,
  IconCloudUpload,
  IconCodeDots,
  IconCopyOff,
  IconDatabase,
  IconDatabaseExport,
  IconDatabaseImport,
  IconDatabaseMinus,
  IconDatabasePlus,
  IconDatabaseSearch,
  IconDatabaseX,
  IconDeviceDesktop,
  IconEdit,
  IconFileAlert,
  IconFileCode,
  IconFileDownload,
  IconFileText,
  IconFileUpload,
  IconFilter,
  IconFingerprint,
  IconGauge,
  IconGitBranch,
  IconHammer,
  IconHandClick,
  IconHandGrab,
  IconHandOff,
  IconHeartbeat,
  IconHeartRateMonitor,
  IconHourglass,
  IconHtml,
  IconId,
  IconInfoCircle,
  IconLayoutList,
  IconLetterCase,
  IconList,
  IconLockCode,
  IconMail,
  IconMathFunction,
  IconMessage,
  IconMessageCircle,
  IconNetwork,
  IconNetworkOff,
  IconPencil,
  IconPhoto,
  IconPhotoX,
  IconPlayerPlay,
  IconPlayerStop,
  IconPlugConnected,
  IconRepeat,
  IconRocket,
  IconRoute,
  IconSend,
  IconSend2,
  IconServer,
  IconShieldCheck,
  IconShieldCheckFilled,
  IconSortAscending,
  IconSql,
  IconSquarePlus,
  IconStack2,
  IconTable,
  IconTag,
  IconTemplate,
  IconTerminal,
  IconTerminal2,
  IconTopologyComplex,
  IconTopologyStarRing3,
  IconTransform,
  IconTrashX,
  IconUpload,
  IconVariable,
  IconWebhook,
  IconWifi,
  IconWorld,
  IconWorldSearch,
  IconZoomScan,
};

const CATEGORY_ORDER = ['trigger', 'action', 'logic', 'utility', 'integration'];

export function NodePalette() {
  const { t } = useTranslation('common');
  const { t: tn } = useTranslation('nodes');
  const [search, setSearch] = useState('');
  const [showUnavailable, setShowUnavailable] = useState(false);

  // Fetch and sync custom nodes from the API
  useCustomNodeSync();
  const customNodes = useCustomNodeRegistry((s) => s.customNodes);
  const { data: enabledMap = {} } = useNodeCatalogAvailability();
  const capabilities = useNodeAvailability();
  const allNodes = useMemo(
    () => getAllNodeDefinitions(customNodes).filter((definition) => enabledMap[definition.key] !== false),
    [customNodes, enabledMap],
  );
  const visibleNodes = useMemo(
    () =>
      showUnavailable
        ? allNodes
        : allNodes.filter((definition) => isNodeDefinitionAvailable(definition, capabilities)),
    [allNodes, capabilities, showUnavailable],
  );

  const CATEGORY_LABELS: Record<string, string> = {
    trigger: t('workflows.categoryTriggers'),
    action: t('workflows.categoryActions'),
    logic: t('workflows.categoryLogic'),
    utility: t('workflows.categoryUtility'),
    integration: t('workflows.categoryIntegrations'),
  };
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  const grouped = useMemo(() => {
    const lc = search.toLowerCase();
    const filtered = lc
      ? visibleNodes.filter((d) => {
          const label = tn(`${d.key}.label`, { defaultValue: d.label });
          const desc = tn(`${d.key}.description`, { defaultValue: d.description });
          return label.toLowerCase().includes(lc) || desc.toLowerCase().includes(lc);
        })
      : visibleNodes;

    const map: Record<string, NodeDefinition[]> = {};
    for (const def of filtered) {
      if (!map[def.category]) map[def.category] = [];
      map[def.category]!.push(def);
    }
    // Sort nodes ascending by translated label within each category
    for (const cat of Object.keys(map)) {
      map[cat]!.sort((a, b) => {
        const la = tn(`${a.key}.label`, { defaultValue: a.label });
        const lb = tn(`${b.key}.label`, { defaultValue: b.label });
        return la.localeCompare(lb);
      });
    }
    return map;
  }, [search, tn, visibleNodes]);

  const toggleCategory = (cat: string) => {
    setCollapsed((prev) => ({ ...prev, [cat]: !prev[cat] }));
  };

  const onDragStart = (e: React.DragEvent, def: NodeDefinition) => {
    if (!isNodeDefinitionAvailable(def, capabilities)) {
      e.preventDefault();
      return;
    }
    e.dataTransfer.setData('workflow/node-action', def.key);
    e.dataTransfer.effectAllowed = 'copy';
  };

  return (
    <div className="flex w-[260px] flex-col border-r border-border bg-card">
      {/* Search */}
      <div className="border-b border-border p-3">
        <div className="relative">
          <IconSearch className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="text"
            placeholder={t('workflows.searchNodes')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-8 pl-8 text-xs"
          />
        </div>
        <div className="mt-2 flex items-center justify-between gap-3">
          <span className="text-[10px] text-muted-foreground">
            {t('workflows.showUnavailableNodes', { defaultValue: 'Show unavailable nodes' })}
          </span>
          <Switch
            checked={showUnavailable}
            onCheckedChange={setShowUnavailable}
            aria-label={t('workflows.showUnavailableNodes', { defaultValue: 'Show unavailable nodes' })}
          />
        </div>
      </div>

      {/* Node list */}
      <div className="flex-1 overflow-y-auto p-2">
        {CATEGORY_ORDER.map((cat) => {
          const defs = grouped[cat];
          if (!defs || defs.length === 0) return null;
          const isCollapsed = collapsed[cat] ?? false;

          return (
            <div key={cat} className="mb-1">
              <Button
                variant="ghost"
                onClick={() => toggleCategory(cat)}
                className="flex w-full items-center justify-start gap-1.5 rounded-md px-2 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground h-auto"
              >
                {isCollapsed ? (
                  <IconChevronRight className="size-3" />
                ) : (
                  <IconChevronDown className="size-3" />
                )}
                {CATEGORY_LABELS[cat] ?? cat}
                <span className="ml-auto text-[10px] font-normal text-muted-foreground/60">{defs.length}</span>
              </Button>

              {!isCollapsed && (
                <div className="space-y-0.5 pb-1">
                  {defs.map((def) => {
                    const Icon = ICON_MAP[def.icon];
                    const tagColor = CATEGORY_TAG_COLORS[def.category] ?? '';
                    const available = isNodeDefinitionAvailable(def, capabilities);
                    return (
                      <div
                        key={def.key}
                        draggable={available}
                        onDragStart={(e) => onDragStart(e, def)}
                        title={available ? undefined : t('workflows.nodeUnavailable')}
                        className={cn(
                          'group flex items-start gap-2.5 rounded-lg px-2.5 py-2 transition-colors',
                          available
                            ? 'cursor-grab hover:bg-muted/50 active:cursor-grabbing'
                            : 'cursor-not-allowed opacity-40',
                        )}
                      >
                        <div className={cn('mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md', tagColor)}>
                          {Icon ? <Icon className="size-3.5" /> : null}
                        </div>
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-xs font-medium text-foreground">{tn(`${def.key}.label`, { defaultValue: def.label })}</p>
                          <p className="truncate text-[10px] text-muted-foreground">{tn(`${def.key}.description`, { defaultValue: def.description })}</p>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}

        {Object.keys(grouped).length === 0 && (
          <p className="py-8 text-center text-xs text-muted-foreground">{t('workflows.noNodesMatch')}</p>
        )}
      </div>
    </div>
  );
}

