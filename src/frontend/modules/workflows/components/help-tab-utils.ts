// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import { getNodeDocumentation } from '../nodes';
export { describeFieldType } from './help-tab-field-types';

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
    const lines = chunk
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean);
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
      return t('workflows.nodeRequiresEmailServer');
    case 'communication-channel':
      return t('workflows.nodeRequiresCommunicationChannel');
    case 'environment':
      return t('workflows.nodeRequiresEnvironment');
    default:
      return t('workflows.nodeUnavailable');
  }
}

export function getBehaviorSummary(category: string, t: TFunction<'common'>): string[] {
  switch (category) {
    case 'trigger':
      return [t('workflows.helpTriggerBehavior1'), t('workflows.helpTriggerBehavior2')];
    case 'logic':
      return [t('workflows.helpLogicBehavior1'), t('workflows.helpLogicBehavior2')];
    case 'utility':
      return [t('workflows.helpUtilityBehavior1'), t('workflows.helpUtilityBehavior2')];
    case 'integration':
      return [t('workflows.helpIntegrationBehavior1'), t('workflows.helpIntegrationBehavior2')];
    default:
      return [t('workflows.helpActionBehavior1'), t('workflows.helpActionBehavior2')];
  }
}

export function describePort(port: string, direction: 'input' | 'output', t: TFunction<'common'>): string {
  if (direction === 'output') {
    switch (port) {
      case 'output':
        return t('workflows.primarySuccessPath');
      case 'error':
        return t('workflows.errorPath');
      case 'true':
        return t('workflows.conditionTruePath');
      case 'false':
      case 'else':
        return t('workflows.conditionFalsePath');
      case 'default':
        return t('workflows.defaultPath');
    }
  }

  if (port.startsWith('condition_')) {
    return t('workflows.extraConditionPath');
  }

  if (port.startsWith('case_')) {
    return t('workflows.switchCasePath');
  }

  if (port.startsWith('input_')) {
    return t('workflows.multiInputPath');
  }

  return direction === 'input' ? t('workflows.genericInputPath') : t('workflows.genericOutputPath');
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
