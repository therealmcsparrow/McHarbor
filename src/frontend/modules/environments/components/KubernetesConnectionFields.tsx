// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';

interface KubernetesConnectionFieldsProps {
  connMethod: 'kubeconfig' | 'bearer';
  onConnMethodChange: (value: 'kubeconfig' | 'bearer') => void;
  kubeconfig: string;
  onKubeconfigChange: (value: string) => void;
  k8sServerUrl: string;
  onServerUrlChange: (value: string) => void;
  k8sBearerToken: string;
  onBearerTokenChange: (value: string) => void;
  k8sNamespace: string;
  onNamespaceChange: (value: string) => void;
}

export function KubernetesConnectionFields({
  connMethod,
  onConnMethodChange,
  kubeconfig,
  onKubeconfigChange,
  k8sServerUrl,
  onServerUrlChange,
  k8sBearerToken,
  onBearerTokenChange,
  k8sNamespace,
  onNamespaceChange,
}: KubernetesConnectionFieldsProps) {
  const { t } = useTranslation('environments');

  return (
    <>
      <div>
        <Label className="mb-2">{t('create.connectionMethod')}</Label>
        <Select
          value={connMethod}
          onChange={(v) => onConnMethodChange(v as 'kubeconfig' | 'bearer')}
          options={[
            { value: 'kubeconfig', label: t('create.kubeconfig') },
            { value: 'bearer', label: t('create.bearerToken') },
          ]}
        />
      </div>
      {connMethod === 'kubeconfig' ? (
        <div>
          <Label className="mb-2">{t('create.kubeconfig')}</Label>
          <textarea
            className="flex min-h-[120px] w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm font-mono placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            value={kubeconfig}
            onChange={(e) => onKubeconfigChange(e.target.value)}
            placeholder={t('create.kubeconfigPlaceholder')}
          />
        </div>
      ) : (
        <>
          <div>
            <Label className="mb-2">{t('create.serverUrl')}</Label>
            <Input
              type="text"
              value={k8sServerUrl}
              onChange={(e) => onServerUrlChange(e.target.value)}
              placeholder="https://192.168.1.100:6443"
            />
          </div>
          <div>
            <Label className="mb-2">{t('create.bearerToken')}</Label>
            <Input
              type="password"
              value={k8sBearerToken}
              onChange={(e) => onBearerTokenChange(e.target.value)}
              placeholder="eyJhbGciOiJ..."
            />
          </div>
        </>
      )}
      <div>
        <Label className="mb-2">{t('create.defaultNamespace')}</Label>
        <Input
          type="text"
          value={k8sNamespace}
          onChange={(e) => onNamespaceChange(e.target.value)}
          placeholder="default"
        />
      </div>
    </>
  );
}
