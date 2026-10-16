// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type ComplianceStatus = 'pass' | 'warn' | 'fail' | 'info' | 'unknown';

export type FactKind = 'bool' | 'number' | 'percent' | 'date' | 'days' | 'text' | 'code';

export type ComplianceFact = {
  key: string;
  kind: FactKind;
  value: string;
  sensitive: boolean;
};

export type ComplianceSection = {
  id: string;
  status: ComplianceStatus;
  facts: ComplianceFact[];
  reasons: string[];
};

export type ComplianceReport = {
  generatedAt: string;
  version: string;
  build: string;
  redacted: boolean;
  summary: Record<ComplianceStatus, number>;
  sections: ComplianceSection[];
};

export const COMPLIANCE_SECTIONS = [
  'audit',
  'keys',
  'instanceBackup',
  'tls',
  'passwords',
  'mfa',
  'identitySync',
  'plugins',
  'ha',
  'selfEnvAccess',
  'supplyChain',
] as const;

export type ComplianceSectionId = (typeof COMPLIANCE_SECTIONS)[number];

export const COMPLIANCE_SECTION_LINKS: Partial<Record<ComplianceSectionId, string>> = {
  audit: '/settings?tab=retention',
  keys: '/settings?tab=instance-backup',
  instanceBackup: '/settings?tab=instance-backup',
  tls: '/security?tab=https',
  passwords: '/security?tab=password',
  mfa: '/security?tab=users',
  identitySync: '/security?tab=identity',
  plugins: '/settings?tab=plugins',
  ha: '/enterprise/ha',
  selfEnvAccess: '/security?tab=roles',
};

export type ComplianceQuery = {
  sections?: readonly string[];
  redact?: boolean;
  export?: boolean;
};

export function complianceParams(query: ComplianceQuery): Record<string, string> {
  const params: Record<string, string> = {};
  if (query.sections && query.sections.length > 0) params.sections = query.sections.join(',');
  if (query.redact !== undefined) params.redact = query.redact ? '1' : '0';
  if (query.export) params.export = '1';
  return params;
}

export function fetchComplianceReport(query: ComplianceQuery): Promise<ComplianceReport> {
  return api.get<ComplianceReport>('/compliance/posture', complianceParams(query)).then(assertSuccess);
}

export function useCompliancePosture(enabled: boolean) {
  return useQuery({
    queryKey: ['compliance', 'posture'],
    enabled,
    queryFn: () => fetchComplianceReport({}),
    staleTime: 30_000,
  });
}
