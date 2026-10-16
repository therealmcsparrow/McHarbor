// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import type { Explanation, SimulateResult } from '../../hooks/useGovernance';

const REASON_KINDS = [
  'role',
  'wildcard_role',
  'elevation',
  'break_glass',
  'override_grant',
  'override_deny',
  'abac_allow',
  'abac_deny',
  'no_grant',
] as const;

function reasonKey(kind: string): string {
  return (REASON_KINDS as readonly string[]).includes(kind)
    ? `governance.simulator.reasons.${kind}`
    : 'governance.simulator.reasons.other';
}

function Decision({ exp }: { exp: Explanation }) {
  const { t } = useTranslation('security');
  return (
    <div className="space-y-1 rounded-md border border-border p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-xs text-foreground">{exp.permission}</span>
        <Badge variant={exp.allowed ? 'success' : 'destructive'}>
          {exp.allowed ? t('governance.simulator.allow') : t('governance.simulator.deny')}
        </Badge>
        <TypographyText size="xs" tone="muted">
          {t('governance.simulator.scope', { scope: exp.scope })}
        </TypographyText>
      </div>
      <ul className="space-y-0.5">
        {exp.reasons.map((r) => (
          <li key={`${r.kind}-${r.id ?? ''}`} className="text-xs text-muted-foreground">
            {t(reasonKey(r.kind), { name: r.name ?? r.id ?? '' })}
          </li>
        ))}
      </ul>
      {exp.policyAllowed !== exp.allowed && (
        <TypographyText size="xs" tone="muted">
          {exp.policyAllowed ? t('governance.simulator.policyPathAllows') : t('governance.simulator.policyPathDenies')}
        </TypographyText>
      )}
    </div>
  );
}

export function SimulationResultView({ result }: { result: SimulateResult }) {
  const { t } = useTranslation('security');
  return (
    <div className="space-y-3" aria-live="polite">
      <Alert
        variant={result.allowed ? 'success' : 'destructive'}
        title={result.allowed ? t('governance.simulator.allowed') : t('governance.simulator.denied')}
        description={result.route ? `${result.method ?? ''} ${result.route}` : undefined}
      />
      {result.route && result.gates.length === 0 && (
        <TypographyText size="xs" tone="muted">
          {result.exempt
            ? t('governance.simulator.exempt', { reason: result.exempt })
            : t('governance.simulator.ungated')}
        </TypographyText>
      )}
      {result.gates.map((gate) => (
        <div key={`${gate.scope}-${gate.decisions.map((d) => d.permission).join(',')}`} className="space-y-2">
          {gate.any && gate.decisions.length > 1 && (
            <TypographyText size="xs" tone="muted">
              {t('governance.simulator.anyOf')}
            </TypographyText>
          )}
          {gate.decisions.map((exp) => (
            <Decision key={exp.permission} exp={exp} />
          ))}
        </div>
      ))}
    </div>
  );
}
