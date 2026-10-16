// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import type { SnapshotConfig, SnapshotConfigWrite } from './useSnapshotConfigs';
import { useSnapshotAdapters, type SnapshotAdapterInfo } from './useSnapshotAdapters';

const EMPTY: SnapshotConfigWrite = {
  name: '',
  driver: '',
  pluginSlug: '',
  enabled: true,
  poolName: '',
  datasetPrefix: '',
  sshTarget: '',
  useLxd: false,
  config: {},
};

type SnapshotConfigFormOptions = {
  open: boolean;
  config?: SnapshotConfig | null;
  /**
   * Driver to select on open, set when the operator clicked "Set up" on
   * a detected driver.
   */
  presetDriver?: string;
  onSave: (body: SnapshotConfigWrite) => void;
};

/**
 * Draft state for the snapshot-config dialog: the typed columns, the
 * adapter's declared schema fields, and the free-form JSON map the host
 * still accepts for plugins that declare an empty schema.
 */
export function useSnapshotConfigForm({ open: isOpen, config, presetDriver, onSave }: SnapshotConfigFormOptions) {
  const adapters = useSnapshotAdapters();
  const [form, setForm] = useState<SnapshotConfigWrite>(EMPTY);
  // Free-form JSON kept for backwards compatibility — most snapshot
  // adapters use the typed schema below, but the host still accepts a
  // raw config map for plugins that declare an empty schema.
  const [configJson, setConfigJson] = useState('{}');
  const [configError, setConfigError] = useState<string | null>(null);

  // Form value for each ConfigFieldDecl keyed by field.key.
  const [schemaValues, setSchemaValues] = useState<Record<string, string>>({});

  const adapterList = useMemo<SnapshotAdapterInfo[]>(() => {
    return Object.values(adapters.data ?? {}).sort((a, b) => a.DisplayName.localeCompare(b.DisplayName));
  }, [adapters.data]);

  const selectedAdapter = useMemo<SnapshotAdapterInfo | null>(() => {
    if (!form.driver) return null;
    return (adapters.data ?? {})[form.driver] ?? null;
  }, [adapters.data, form.driver]);

  const [seeded, setSeeded] = useState<{ isOpen: boolean; config: unknown; presetDriver: unknown; data: unknown }>({
    isOpen: false,
    config: undefined,
    presetDriver: undefined,
    data: undefined,
  });
  if (
    seeded.isOpen !== isOpen ||
    seeded.config !== config ||
    seeded.presetDriver !== presetDriver ||
    seeded.data !== adapters.data
  ) {
    setSeeded({ isOpen, config, presetDriver, data: adapters.data });
    if (isOpen) seedFromProps();
  }

  function seedFromProps() {
    if (config) {
      setForm({
        name: config.name,
        driver: config.driver,
        pluginSlug: config.pluginSlug,
        enabled: config.enabled,
        poolName: config.poolName ?? '',
        datasetPrefix: config.datasetPrefix ?? '',
        sshTarget: config.sshTarget ?? '',
        useLxd: config.useLxd,
        config: config.config,
      });
      setConfigJson(JSON.stringify(config.config, null, 2));
      // Seed schema fields from existing config map. The host stores
      // everything (typed columns + schema keys) inside `config`, so
      // we read back whichever side this driver originally wrote.
      const seed: Record<string, string> = {};
      for (const [k, v] of Object.entries(config.config)) {
        seed[k] = v;
      }
      // Pre-fill legacy typed columns for the zfs driver so an edit
      // round-trips without forcing the operator to re-enter values.
      if (config.driver === 'zfs') {
        if (config.poolName && seed.pool_name === undefined) seed.pool_name = config.poolName;
        if (config.datasetPrefix && seed.dataset_prefix === undefined) seed.dataset_prefix = config.datasetPrefix;
        if (config.sshTarget && seed.ssh_target === undefined) seed.ssh_target = config.sshTarget;
        if (seed.use_lxd === undefined) seed.use_lxd = config.useLxd ? 'true' : 'false';
      }
      setSchemaValues(seed);
    } else {
      const spec = presetDriver ? (adapters.data ?? {})[presetDriver] : undefined;
      setForm(spec ? { ...EMPTY, driver: spec.Driver, pluginSlug: spec.Slug ?? '', name: spec.DisplayName } : EMPTY);
      setConfigJson('{}');
      setSchemaValues({});
    }
    setConfigError(null);
  }

  function pickAdapter(driver: string) {
    const spec = (adapters.data ?? {})[driver];
    if (!spec) return;
    setForm({
      ...form,
      driver: spec.Driver,
      pluginSlug: spec.Slug ?? '',
      // Reset pool/prefix/ssh — these are zfs-specific. The schema
      // fields below carry the same values when applicable.
      poolName: '',
      datasetPrefix: '',
      sshTarget: '',
      config: {},
    });
    setSchemaValues({});
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    let parsed: Record<string, string> = {};
    if (configJson.trim().length > 0) {
      try {
        const raw = JSON.parse(configJson);
        if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) {
          setConfigError('JSON must be an object');
          return;
        }
        parsed = Object.fromEntries(
          Object.entries(raw).map(([k, v]) => [k, typeof v === 'string' ? v : JSON.stringify(v)]),
        );
      } catch (err) {
        setConfigError(err instanceof Error ? err.message : 'invalid JSON');
        return;
      }
    }
    // Merge schema values in: schema fields take precedence when set.
    const mergedConfig = { ...parsed, ...schemaValues };
    // Backfill legacy typed columns when the operator picked the zfs
    // driver — keeps the existing writeArchive / probe paths happy
    // without forcing the operator to fill the same value twice.
    const submit: SnapshotConfigWrite = {
      ...form,
      config: mergedConfig,
    };
    if (form.driver === 'zfs') {
      submit.poolName = schemaValues.pool_name ?? '';
      submit.datasetPrefix = schemaValues.dataset_prefix ?? '';
      submit.sshTarget = schemaValues.ssh_target ?? '';
      submit.useLxd = schemaValues.use_lxd === 'true';
    }
    onSave(submit);
  }

  /** Editing the JSON clears the parse error it produced. */
  function editConfigJson(next: string) {
    setConfigJson(next);
    setConfigError(null);
  }

  return {
    adapters,
    adapterList,
    selectedAdapter,
    form,
    setForm,
    configJson,
    setConfigJson: editConfigJson,
    configError,
    schemaValues,
    setSchemaValues,
    pickAdapter,
    handleSubmit,
  };
}
