// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { UserLabelsInspector } from '@resources/components/security/abac/UserLabelsInspector';
import { IconPlayerPlay } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Textarea } from '@resources/components/ui/Textarea';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { type PolicyTestAttributes } from '@resources/hooks/usePolicies';
import { usePolicyTester } from '@resources/hooks/usePolicyTester';
import { PERM } from '@core/auth/permissionKeys';

type PolicyTesterDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  policyId: string;
  policyName: string;
  policyExpression: string;
  policyResourceType: string;
};

/**
 * Dry-run a single policy's expression against an admin-supplied
 * attribute bundle. Submits to POST /rbac/policies/{id}/test which
 * runs the same goja sandbox the live authoriser uses, so the
 * preview matches production behavior byte-for-byte.
 */
export function PolicyTesterDialog({
  open: isOpen,
  onOpenChange,
  policyId,
  policyName,
  policyExpression,
  policyResourceType,
}: PolicyTesterDialogProps) {
  const { t } = useTranslation('abac');
  const {
    rawJson,
    setRawJson,
    parsed,
    parseError,
    result,
    test,
    run: handleRun,
  } = usePolicyTester(policyId, policyResourceType, isOpen);

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('tester.title', { name: policyName })}</DialogTitle>
          <DialogDescription>{t('tester.description')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-4">
          <div className="rounded-lg border border-border bg-muted/30 p-3">
            <TypographyText size="xs" tone="muted" className="mb-1">
              {t('tester.expressionLabel')}
            </TypographyText>
            <pre className="overflow-x-auto font-mono text-xs whitespace-pre-wrap text-foreground">
              {policyExpression || <span className="text-muted-foreground italic">∅</span>}
            </pre>
          </div>

          {/* Load a real user's labels into the bundle. Testing against
              hand-typed labels is how an expression passes here and fails in
              production. */}
          <UserLabelsInspector
            onApply={(labels) => {
              try {
                const current = JSON.parse(rawJson) as PolicyTestAttributes;
                setRawJson(JSON.stringify({ ...current, user: { ...current.user, labels } }, null, 2));
              } catch {
                // The bundle is mid-edit and unparseable; the existing parse
                // error below already says so, and silently rewriting it would
                // discard whatever the author was typing.
              }
            }}
          />

          <div>
            <TypographyText size="sm" weight="medium" className="mb-2">
              {t('tester.attributesLabel')}
            </TypographyText>
            <Textarea
              value={rawJson}
              onChange={(e) => setRawJson(e.target.value)}
              rows={14}
              className="font-mono text-xs"
              spellCheck={false}
              aria-label={t('tester.attributesAriaLabel')}
            />
            {parseError && (
              <TypographyText size="xs" className="mt-1 text-destructive">
                {t('tester.parseError', { error: parseError })}
              </TypographyText>
            )}
          </div>

          {result && (
            <div
              className={`rounded-lg border p-3 ${
                result.matched ? 'border-primary/30 bg-primary/5' : 'border-border bg-muted/30'
              }`}
            >
              <div className="flex items-center gap-2">
                <Badge variant={result.matched ? 'success' : 'secondary'}>
                  {result.matched ? t('tester.matched') : t('tester.denied')}
                </Badge>
                {result.error && <Badge variant="destructive">{t('tester.errorBadge')}</Badge>}
              </div>
              {result.reason && (
                <TypographyText size="sm" className="mt-2">
                  {result.reason}
                </TypographyText>
              )}
              {result.error && (
                <pre className="mt-2 overflow-x-auto font-mono text-xs whitespace-pre-wrap text-destructive">
                  {result.error}
                </pre>
              )}
            </div>
          )}
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('tester.close')}
          </Button>
          <Button permission={PERM.RBAC_POLICIES_MANAGE} onClick={handleRun} disabled={!parsed || test.isPending}>
            <IconPlayerPlay className="mr-1 size-3.5" />
            {test.isPending ? t('tester.running') : t('tester.run')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
