// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import { getNodeDocumentation } from '../nodes';
import type { ConfigFieldType } from '../types';

export type MarkdownSection = {
  title: string;
  paragraphs: string[];
  bullets: string[];
};

export type NodeDocs = Awaited<ReturnType<typeof getNodeDocumentation>>;

type NodeRequirementKind = 'email-server' | 'communication-channel' | 'environment' | 'capability';

export function parseMarkdownSections(markdown?: string): MarkdownSection[] {
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

export function getRequirementMessage(kind: NodeRequirementKind, t: TFunction<'common'>) {
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

export function getBehaviorSummary(category: string, t: TFunction<'common'>): string[] {
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

export function describePort(
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

export function describeFieldType(type: ConfigFieldType, t: TFunction<'common'>): string {
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

export function formatDefaultValue(value: unknown): string {
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
