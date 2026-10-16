// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { FormField } from '@resources/components/ui/FormField';
import { Input } from '@resources/components/ui/Input';
import { PasswordInput } from '@resources/components/ui/PasswordInput';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Textarea } from '@resources/components/ui/Textarea';
import { useCreateCluster, useUpdateCluster, type Cluster } from '../hooks/useClusters';
import { ClusterEnvironmentField } from './ClusterEnvironmentField';
import { KUBE_PERM } from '../permissions';

/**
 * How the cluster is authenticated.
 *
 * These are the two shapes `connection.go` can turn into a rest.Config: a
 * kubeconfig blob, or a server URL with a bearer token. Anything else is
 * rejected by the create endpoint, so the form offers exactly these — the same
 * contract the host's `KubernetesConnectionFields` carried before the move.
 */
type AuthMethod = 'kubeconfig' | 'token';

const SERVER_PLACEHOLDER = 'https://10.0.0.1:6443';
const CA_PLACEHOLDER = '-----BEGIN CERTIFICATE-----';

type ClusterFormDialogProps = {
  open: boolean;
  /** null = create. */
  cluster: Cluster | null;
  onOpenChange: (open: boolean) => void;
};

export function ClusterFormDialog({ open: isOpen, cluster, onOpenChange }: ClusterFormDialogProps) {
  const { t } = useTranslation('kubernetes');
  const create = useCreateCluster();
  const update = useUpdateCluster();

  const [itemName, setName] = useState('');
  const [namespace, setNamespace] = useState('');
  const [serverUrl, setServerUrl] = useState('');
  const [kubeconfig, setKubeconfig] = useState('');
  const [bearerToken, setBearerToken] = useState('');
  const [caCert, setCaCert] = useState('');
  const [isDefault, setIsDefault] = useState(false);
  const [isActive, setIsActive] = useState(true);
  const [environmentId, setEnvironmentId] = useState('');
  const [authMethod, setAuthMethod] = useState<AuthMethod>('kubeconfig');

  // Reseed whenever a different cluster opens the dialog. Credentials are
  // never seeded: the API does not return them, and pre-filling a box with
  // placeholder text would make "leave unchanged" indistinguishable from
  // "set it to this".
  const [seed, setSeed] = useState<{ isOpen: boolean; cluster: Cluster | null }>({ isOpen: false, cluster: null });
  if (!isOpen && seed.isOpen) {
    setSeed({ isOpen: false, cluster });
  } else if (isOpen && (!seed.isOpen || seed.cluster !== cluster)) {
    setSeed({ isOpen: true, cluster });
    setName(cluster?.name ?? '');
    setNamespace(cluster?.namespace ?? '');
    setServerUrl(cluster?.serverUrl ?? '');
    setKubeconfig('');
    setBearerToken('');
    setCaCert('');
    setIsDefault(cluster?.isDefault ?? false);
    setIsActive(cluster?.isActive ?? true);
    setEnvironmentId(cluster?.environmentId ?? '');
    setAuthMethod(cluster?.hasToken && !cluster.hasKubeconfig ? 'token' : 'kubeconfig');
  }

  const editing = cluster !== null;
  const pending = create.isPending || update.isPending;
  // On create the credential is required; on edit an empty box means "keep
  // what is stored", which is why `hasKubeconfig` / `hasToken` exist.
  const credentialSatisfied =
    authMethod === 'kubeconfig'
      ? kubeconfig.trim() !== '' || Boolean(cluster?.hasKubeconfig)
      : (bearerToken.trim() !== '' || Boolean(cluster?.hasToken)) && serverUrl.trim() !== '';
  const canSubmit = itemName.trim() !== '' && credentialSatisfied && !pending;

  function submit() {
    if (!canSubmit) return;
    // Only send a credential the reader actually typed — an empty string would
    // clear the stored one.
    const body = {
      name: itemName.trim(),
      namespace: namespace.trim(),
      serverUrl: authMethod === 'token' ? serverUrl.trim() : '',
      isDefault,
      isActive,
      environmentId,
      ...(authMethod === 'kubeconfig' && kubeconfig.trim() ? { kubeconfig: kubeconfig } : {}),
      ...(authMethod === 'token' && bearerToken.trim() ? { bearerToken } : {}),
      ...(authMethod === 'token' && caCert.trim() ? { caCert } : {}),
    };
    const done = { onSuccess: () => onOpenChange(false) };
    if (editing) update.mutate({ id: cluster.id, ...body }, done);
    else create.mutate(body, done);
  }

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{editing ? t('clusters.form.editTitle') : t('clusters.form.addTitle')}</DialogTitle>
          <DialogDescription>{t('clusters.form.description')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-4">
          <FormField label={t('clusters.form.name')} htmlFor="k8s-cluster-name">
            <Input
              id="k8s-cluster-name"
              value={itemName}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('clusters.form.namePlaceholder')}
            />
          </FormField>

          <FormField
            label={t('clusters.form.namespace')}
            htmlFor="k8s-cluster-namespace"
            description={t('clusters.form.namespaceHelp')}
          >
            <Input
              id="k8s-cluster-namespace"
              value={namespace}
              onChange={(e) => setNamespace(e.target.value)}
              placeholder={t('values.allNamespaces')}
            />
          </FormField>

          <FormField label={t('clusters.form.authMethod')}>
            <Select
              value={authMethod}
              onChange={(v) => setAuthMethod(v as AuthMethod)}
              ariaLabel={t('clusters.form.authMethod')}
              options={[
                { value: 'kubeconfig', label: t('clusters.form.authKubeconfig') },
                { value: 'token', label: t('clusters.form.authToken') },
              ]}
            />
          </FormField>

          {authMethod === 'kubeconfig' ? (
            <FormField
              label={t('clusters.form.kubeconfig')}
              htmlFor="k8s-cluster-kubeconfig"
              description={
                cluster?.hasKubeconfig ? t('clusters.form.credentialOnFile') : t('clusters.form.kubeconfigHelp')
              }
            >
              <Textarea
                id="k8s-cluster-kubeconfig"
                rows={8}
                className="font-mono text-xs"
                value={kubeconfig}
                onChange={(e) => setKubeconfig(e.target.value)}
                placeholder={t('clusters.form.kubeconfigPlaceholder')}
              />
            </FormField>
          ) : (
            <>
              <FormField label={t('clusters.form.serverUrl')} htmlFor="k8s-cluster-server">
                <Input
                  id="k8s-cluster-server"
                  value={serverUrl}
                  onChange={(e) => setServerUrl(e.target.value)}
                  placeholder={SERVER_PLACEHOLDER}
                />
              </FormField>
              <FormField
                label={t('clusters.form.bearerToken')}
                htmlFor="k8s-cluster-token"
                description={cluster?.hasToken ? t('clusters.form.credentialOnFile') : undefined}
              >
                <PasswordInput
                  id="k8s-cluster-token"
                  value={bearerToken}
                  onChange={(e) => setBearerToken(e.target.value)}
                />
              </FormField>
              <FormField
                label={t('clusters.form.caCert')}
                htmlFor="k8s-cluster-ca"
                description={t('clusters.form.caCertHelp')}
              >
                <Textarea
                  id="k8s-cluster-ca"
                  rows={4}
                  className="font-mono text-xs"
                  value={caCert}
                  onChange={(e) => setCaCert(e.target.value)}
                  placeholder={CA_PLACEHOLDER}
                />
              </FormField>
            </>
          )}

          <ClusterEnvironmentField value={environmentId} disabled={pending} onChange={setEnvironmentId} />

          <div className="flex flex-wrap items-center gap-6">
            <div className="flex items-center gap-2">
              <Switch id="k8s-cluster-default" checked={isDefault} onCheckedChange={setIsDefault} />
              <Label htmlFor="k8s-cluster-default">{t('clusters.form.isDefault')}</Label>
            </div>
            <div className="flex items-center gap-2">
              <Switch id="k8s-cluster-active" checked={isActive} onCheckedChange={setIsActive} />
              <Label htmlFor="k8s-cluster-active">{t('clusters.form.isActive')}</Label>
            </div>
          </div>
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={pending}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={KUBE_PERM.CLUSTERS_MANAGE} onClick={submit} disabled={!canSubmit}>
            {editing ? t('actions.save', { ns: 'common' }) : t('clusters.actions.add')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
