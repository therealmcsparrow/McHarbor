// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState, type ReactNode } from 'react';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  CATEGORY_TAG_COLORS,
  getNodeDocumentation,
  getNodeRequirement,
  isNodeDefinitionAvailable,
} from '../nodes';
import { useNodeAvailability } from '@resources/hooks/useNodeAvailability';
import type { ConfigField, ConfigFieldType, NodeDefinition } from '../types';

type HelpTabProps = {
  definition?: NodeDefinition;
};

type MarkdownSection = {
  title: string;
  paragraphs: string[];
  bullets: string[];
};

type NodeDocs = Awaited<ReturnType<typeof getNodeDocumentation>>;

export function HelpTab({ definition }: HelpTabProps) {
  if (definition) {
    return <NodeDocumentation definition={definition} />;
  }
  return <GeneralHelp />;
}

function NodeDocumentation({ definition }: { definition: NodeDefinition }) {
  const { t } = useTranslation('common');
  const { t: tn } = useTranslation('nodes');
  const capabilities = useNodeAvailability();
  const requirement = getNodeRequirement(definition);
  const isAvailable = isNodeDefinitionAvailable(definition, capabilities);
  const [docs, setDocs] = useState<NodeDocs>();
  const [loadingDocs, setLoadingDocs] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoadingDocs(true);
    setDocs(undefined);

    void getNodeDocumentation(definition.key).then((result) => {
      if (cancelled) {
        return;
      }

      setDocs(result);
      setLoadingDocs(false);
    });

    return () => {
      cancelled = true;
    };
  }, [definition.key]);

  const readmeSections = parseMarkdownSections(docs?.readme);
  const technicalSections = parseMarkdownSections(docs?.backendAction);
  const tagColor = CATEGORY_TAG_COLORS[definition.category] ?? '';

  return (
    <div className="space-y-4 p-4">
      <div>
        <div className="flex items-center gap-2">
          <span className={cn('rounded-md px-2 py-0.5 text-[10px] font-medium uppercase', tagColor)}>
            {definition.category}
          </span>
          <span className="font-medium text-sm text-foreground">
            {tn(`${definition.key}.label`, { defaultValue: definition.label })}
          </span>
        </div>
        <p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">
          {tn(`${definition.key}.description`, { defaultValue: definition.description })}
        </p>
      </div>

      {requirement && (
        <section
          className={cn(
            'rounded-lg border p-3',
            isAvailable
              ? 'border-emerald-500/20 bg-emerald-500/5'
              : 'border-amber-500/20 bg-amber-500/5',
          )}
        >
          <div className="flex items-center justify-between gap-3">
            <p className="text-xs font-semibold text-foreground">
              {t('workflows.requirementsSection', { defaultValue: 'Requirements' })}
            </p>
            <Badge variant="secondary" className="text-[10px]">
              {isAvailable
                ? t('workflows.configured', { defaultValue: 'Configured' })
                : t('workflows.unavailable', { defaultValue: 'Unavailable' })}
            </Badge>
          </div>
          <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
            {getRequirementMessage(requirement.kind, t)}
          </p>
        </section>
      )}

      <HelpSection title={t('workflows.behaviorSection', { defaultValue: 'Behavior' })}>
        {getBehaviorSummary(definition.category, t).map((paragraph) => (
          <p key={paragraph} className="text-[11px] leading-relaxed text-muted-foreground">
            {paragraph}
          </p>
        ))}
      </HelpSection>

      <HelpSection title={t('workflows.inputPorts', { defaultValue: 'Input Ports' })}>
        {definition.inputPorts.length > 0 ? (
          <div className="space-y-2">
            {definition.inputPorts.map((port) => (
              <PortCard
                key={port}
                label={port}
                description={describePort(port, 'input', t)}
              />
            ))}
          </div>
        ) : (
          <p className="text-[11px] leading-relaxed text-muted-foreground">
            {t('workflows.noInputs', { defaultValue: 'This node starts a flow and does not receive incoming connections.' })}
          </p>
        )}
      </HelpSection>

      <HelpSection title={t('workflows.outputPorts', { defaultValue: 'Output Ports' })}>
        {definition.outputPorts.length > 0 ? (
          <div className="space-y-2">
            {definition.outputPorts.map((port) => (
              <PortCard
                key={port}
                label={port}
                description={describePort(port, 'output', t)}
              />
            ))}
          </div>
        ) : (
          <p className="text-[11px] leading-relaxed text-muted-foreground">
            {t('workflows.noOutputs', { defaultValue: 'This node does not expose downstream outputs.' })}
          </p>
        )}
      </HelpSection>

      <HelpSection title={t('workflows.configurationSection', { defaultValue: 'Configuration' })}>
        {definition.configSchema.length > 0 ? (
          <div className="space-y-2">
            {definition.configSchema.map((field) => (
              <FieldCard key={field.key} definition={definition} field={field} />
            ))}
          </div>
        ) : (
          <p className="text-[11px] leading-relaxed text-muted-foreground">
            {t('workflows.noConfig', { defaultValue: 'This node has no configurable fields.' })}
          </p>
        )}
      </HelpSection>

      {loadingDocs && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Spinner size="sm" />
          <span>{t('loading')}</span>
        </div>
      )}

      {readmeSections.map((section) => (
        <DocSection key={`readme-${section.title}`} section={section} />
      ))}

      {technicalSections.length > 0 && (
        <HelpSection title={t('workflows.technicalDetailsSection', { defaultValue: 'Technical Details' })}>
          <div className="space-y-3">
            {technicalSections.map((section) => (
              <DocSectionContent key={`technical-${section.title}`} section={section} />
            ))}
          </div>
        </HelpSection>
      )}
    </div>
  );
}

function FieldCard({ definition, field }: { definition: NodeDefinition; field: ConfigField }) {
  const { t } = useTranslation('common');
  const { t: tn } = useTranslation('nodes');
  const translatedLabel = tn(`${definition.key}.config.${field.key}`, { defaultValue: field.label });

  return (
    <div className="rounded-md border border-border p-2.5">
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-xs font-medium text-foreground">{translatedLabel}</span>
        <Badge variant="secondary" className="text-[10px]">
          {field.type}
        </Badge>
        <span className="text-[10px] text-muted-foreground">
          {field.required
            ? t('workflows.required', { defaultValue: 'required' })
            : t('labels.optional', { defaultValue: 'optional' })}
        </span>
      </div>
      <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
        {describeFieldType(field.type, t)}
      </p>
      {field.default != null && (
        <p className="mt-1 text-[10px] text-muted-foreground">
          {t('workflows.defaultValue', { defaultValue: 'Default' })}:{' '}
          <code className="rounded bg-muted px-1 py-0.5 text-[10px]">
            {formatDefaultValue(field.default)}
          </code>
        </p>
      )}
      {field.options && field.options.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1">
          {field.options.map((option) => (
            <code key={option.value} className="rounded bg-muted px-1 py-0.5 text-[10px]">
              {tn(`${definition.key}.options.${field.key}.${option.value}`, { defaultValue: option.label })}
            </code>
          ))}
        </div>
      )}
    </div>
  );
}

function PortCard({ label, description }: { label: string; description: string }) {
  return (
    <div className="rounded-md border border-border p-2.5">
      <div className="flex items-center gap-2">
        <Badge variant="secondary" className="text-[10px]">{label}</Badge>
      </div>
      <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">{description}</p>
    </div>
  );
}

function HelpSection({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section>
      <h3 className="mb-2 text-xs font-semibold text-foreground">{title}</h3>
      <div className="space-y-2">{children}</div>
    </section>
  );
}

function DocSection({ section }: { section: MarkdownSection }) {
  return (
    <HelpSection title={section.title}>
      <DocSectionContent section={section} />
    </HelpSection>
  );
}

function DocSectionContent({ section }: { section: MarkdownSection }) {
  return (
    <>
      {section.paragraphs.map((paragraph) => (
        <p key={paragraph} className="text-[11px] leading-relaxed text-muted-foreground">
          {paragraph}
        </p>
      ))}
      {section.bullets.length > 0 && (
        <ul className="space-y-1">
          {section.bullets.map((bullet) => (
            <li key={bullet} className="text-[11px] leading-relaxed text-muted-foreground">
              - {bullet}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

function parseMarkdownSections(markdown?: string): MarkdownSection[] {
  if (!markdown) {
    return [];
  }

  const normalized = markdown.replace(/\r/g, '').trim();
  const sections: MarkdownSection[] = [];
  const chunks = normalized.includes('\n## ')
    ? normalized
        .split('\n## ')
        .map((chunk) => chunk.trim())
        .filter(Boolean)
        .slice(1)
    : [normalized];

  for (const chunk of chunks) {
    const lines = chunk.split('\n').map((line) => line.trim()).filter(Boolean);
    if (lines.length === 0) {
      continue;
    }

    const [rawTitle = '', ...contentLines] = lines;
    const section: MarkdownSection = {
      title: rawTitle.replace(/^#+\s+/, '').trim(),
      paragraphs: [],
      bullets: [],
    };

    for (const line of contentLines) {
      if (line.startsWith('- ')) {
        section.bullets.push(line.slice(2).trim());
        continue;
      }
      section.paragraphs.push(line);
    }

    const hiddenSections = new Set([
      'backend binding',
      'configuration guide',
      'execution notes',
      'files',
      'port behavior',
      'requirements',
      'what this node does',
      'when to use it',
    ]);

    if (hiddenSections.has(section.title.toLowerCase())) {
      continue;
    }

    sections.push(section);
  }

  return sections;
}

function getRequirementMessage(kind: NodeRequirementKind, t: TFunction<'common'>) {
  switch (kind) {
    case 'email-server':
      return t('workflows.nodeRequiresEmailServer', {
        defaultValue:
          'This node only works when at least one enabled email server is configured in Settings.',
      });
    case 'communication-channel':
      return t('workflows.nodeRequiresCommunicationChannel', {
        defaultValue:
          'This node only works when at least one enabled communication channel is configured in Settings.',
      });
    case 'environment':
      return t('workflows.nodeRequiresEnvironment', {
        defaultValue:
          'This node only works when at least one compatible environment is configured in McHarbor.',
      });
    default:
      return t('workflows.nodeUnavailable', {
        defaultValue:
          'This node requires a service that is not yet configured. Set it up in Settings first.',
      });
  }
}

type NodeRequirementKind = 'email-server' | 'communication-channel' | 'environment' | 'capability';

function getBehaviorSummary(category: string, t: TFunction<'common'>): string[] {
  switch (category) {
    case 'trigger':
      return [
        t('workflows.helpTriggerBehavior1', {
          defaultValue:
            'Trigger nodes start a new workflow run and create the first msg object for the flow.',
        }),
        t('workflows.helpTriggerBehavior2', {
          defaultValue:
            'Use trigger nodes at the beginning of a chain. They do not wait for an incoming message from another node.',
        }),
      ];
    case 'logic':
      return [
        t('workflows.helpLogicBehavior1', {
          defaultValue:
            'Logic nodes inspect the current msg and decide which path should run next.',
        }),
        t('workflows.helpLogicBehavior2', {
          defaultValue:
            'They are best used for branching, filtering, matching, or routing without performing an external side effect.',
        }),
      ];
    case 'utility':
      return [
        t('workflows.helpUtilityBehavior1', {
          defaultValue:
            'Utility nodes help inspect, reshape, or annotate the msg while keeping the workflow readable.',
        }),
        t('workflows.helpUtilityBehavior2', {
          defaultValue:
            'They usually pass the message along after performing a small internal operation or side effect.',
        }),
      ];
    case 'integration':
      return [
        t('workflows.helpIntegrationBehavior1', {
          defaultValue:
            'Integration nodes call an external system or a configured transport and then continue with the result.',
        }),
        t('workflows.helpIntegrationBehavior2', {
          defaultValue:
            'Use the error output when you want failures to branch into recovery, retries, or alerts.',
        }),
      ];
    default:
      return [
        t('workflows.helpActionBehavior1', {
          defaultValue:
            'Action nodes receive the current msg, perform work, and usually pass an updated msg to the next step.',
        }),
        t('workflows.helpActionBehavior2', {
          defaultValue:
            'They are the main building blocks for changing state, calling APIs, or transforming workflow data.',
        }),
      ];
  }
}

function describePort(
  port: string,
  direction: 'input' | 'output',
  t: TFunction<'common'>,
): string {
  if (direction === 'output') {
    switch (port) {
      case 'output':
        return t('workflows.primarySuccessPath', {
          defaultValue: 'Primary success path. Messages continue here when the node completes normally.',
        });
      case 'error':
        return t('workflows.errorPath', {
          defaultValue: 'Failure path. Messages route here when the node returns an error.',
        });
      case 'true':
        return t('workflows.conditionTruePath', {
          defaultValue: 'Used when the condition evaluates to true.',
        });
      case 'false':
      case 'else':
        return t('workflows.conditionFalsePath', {
          defaultValue: 'Used when the condition does not match the primary branch.',
        });
      case 'default':
        return t('workflows.defaultPath', {
          defaultValue: 'Fallback path for values that did not match any explicit case.',
        });
    }
  }

  if (port.startsWith('condition_')) {
    return t('workflows.extraConditionPath', {
      defaultValue: 'Routes messages for this extra condition branch.',
    });
  }

  if (port.startsWith('case_')) {
    return t('workflows.switchCasePath', {
      defaultValue: 'Routes messages for this configured switch case.',
    });
  }

  if (port.startsWith('input_')) {
    return t('workflows.multiInputPath', {
      defaultValue: 'One of the multiple input lanes handled by this node.',
    });
  }

  return direction === 'input'
    ? t('workflows.genericInputPath', {
        defaultValue: 'Receives the incoming msg on this port.',
      })
    : t('workflows.genericOutputPath', {
        defaultValue: 'Sends the outgoing msg on this port.',
      });
}

function describeFieldType(type: ConfigFieldType, t: TFunction<'common'>): string {
  switch (type) {
    case 'textarea':
      return t('workflows.fieldTypeTextarea', {
        defaultValue: 'Multi-line text. Useful for message bodies, templates, or longer expressions.',
      });
    case 'number':
      return t('workflows.fieldTypeNumber', {
        defaultValue: 'Numeric value. Use whole numbers or decimals as expected by the node.',
      });
    case 'select':
      return t('workflows.fieldTypeSelect', {
        defaultValue: 'Choose one of the built-in options provided by this node.',
      });
    case 'toggle':
      return t('workflows.fieldTypeToggle', {
        defaultValue: 'Turns a node feature on or off.',
      });
    case 'json':
      return t('workflows.fieldTypeJson', {
        defaultValue: 'Enter valid JSON. Objects and arrays are kept as structured data.',
      });
    case 'key-value':
      return t('workflows.fieldTypeKeyValue', {
        defaultValue: 'Build an object as key/value pairs.',
      });
    case 'expression':
      return t('workflows.fieldTypeExpression', {
        defaultValue: 'Evaluated against the current msg, flow, and global values.',
      });
    case 'cron':
      return t('workflows.fieldTypeCron', {
        defaultValue: 'Cron expression used to schedule when the trigger should fire.',
      });
    case 'container-select':
      return t('workflows.fieldTypeContainerSelect', {
        defaultValue: 'Selects a container from the active environment.',
      });
    case 'environment-select':
      return t('workflows.fieldTypeEnvironmentSelect', {
        defaultValue: 'Selects one of the configured McHarbor environments.',
      });
    case 'metric-conditions':
      return t('workflows.fieldTypeMetricConditions', {
        defaultValue: 'Define one or more threshold rules for metric-based triggers.',
      });
    case 'code':
      return t('workflows.fieldTypeCode', {
        defaultValue: 'Code or script content interpreted by the node.',
      });
    case 'link-output-select':
      return t('workflows.fieldTypeLinkOutputSelect', {
        defaultValue: 'Select the matching Link Out target inside this workflow.',
      });
    case 'email-server-select':
      return t('workflows.fieldTypeEmailServerSelect', {
        defaultValue:
          'Uses a configured email server. Leave it empty to fall back to the default enabled server.',
      });
    case 'communication-channel-select':
      return t('workflows.fieldTypeCommunicationChannelSelect', {
        defaultValue:
          'Uses a configured communication channel. Leave it empty to fall back to the default enabled channel.',
      });
    case 'text':
    default:
      return t('workflows.fieldTypeText', {
        defaultValue: 'Single-line text value.',
      });
  }
}

function formatDefaultValue(value: unknown): string {
  if (typeof value === 'string') {
    return value;
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function GeneralHelp() {
  const { t } = useTranslation('common');
  return (
    <div className="space-y-5 p-4">
      <section>
        <h3 className="text-xs font-semibold text-foreground">{t('workflows.helpAddingNodes')}</h3>
        <p className="mt-1 text-[11px] text-muted-foreground leading-relaxed">
          {t('workflows.helpAddingNodesDesc')}
        </p>
      </section>

      <section>
        <h3 className="text-xs font-semibold text-foreground">{t('workflows.helpConnectingNodes')}</h3>
        <p className="mt-1 text-[11px] text-muted-foreground leading-relaxed">
          {t('workflows.helpConnectingNodesDesc')}
        </p>
      </section>

      <section>
        <h3 className="text-xs font-semibold text-foreground">{t('workflows.helpKeyboardShortcuts')}</h3>
        <div className="mt-1.5 space-y-1">
          {[
            ['Delete / Backspace', t('workflows.helpDeleteSelected')],
            ['Ctrl + Z', t('workflows.helpUndo')],
            ['Ctrl + Y', t('workflows.helpRedo')],
            ['Ctrl + S', t('workflows.helpSaveWorkflow')],
            ['Shift + drag', t('workflows.helpPanCanvas')],
            ['Scroll', t('workflows.helpZoomInOut')],
          ].map(([key, desc]) => (
            <div key={key} className="flex items-center gap-2">
              <kbd className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">{key}</kbd>
              <span className="text-[11px] text-muted-foreground">{desc}</span>
            </div>
          ))}
        </div>
      </section>

      <section>
        <h3 className="text-xs font-semibold text-foreground">{t('workflows.helpExpressionVariables')}</h3>
        <p className="mt-1 text-[11px] text-muted-foreground leading-relaxed">
          {t('workflows.helpExpressionVariablesDesc')}
        </p>
        <div className="mt-2 space-y-1.5">
          {[
            { ns: 'msg.payload', color: 'text-indigo-400', desc: t('workflows.helpMsgPayload') },
            { ns: 'msg.topic', color: 'text-cyan-400', desc: t('workflows.helpMsgTopic') },
            { ns: 'msg._msgid', color: 'text-amber-400', desc: t('workflows.helpMsgId') },
            { ns: 'flow.*', color: 'text-emerald-400', desc: t('workflows.helpFlowVars') },
            { ns: 'global.*', color: 'text-purple-400', desc: t('workflows.helpGlobalVars') },
          ].map(({ ns, color, desc }) => (
            <div key={ns}>
              <code className={cn('text-[10px] font-mono', color)}>{`{{ ${ns} }}`}</code>
              <p className="text-[10px] text-muted-foreground">{desc}</p>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
