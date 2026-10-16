// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconScan, IconLoader2 } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useImageScans, useStartScan, useDeleteScan } from '../../hooks/useScans';
import { ScanRow } from './ScanRow';
import { VulnerabilityList } from './VulnerabilityList';
import { useAvailableScanners, type ScannerInfo } from '@resources/hooks/useScannerSettings';

type ScanSectionProps = {
  imageRef: string;
};

export function ScanSection({ imageRef }: ScanSectionProps) {
  const { t } = useTranslation('containers');
  const envId = useEnvironmentStore((s) => s.currentId);
  const { data: scansData } = useImageScans(imageRef);
  const { data: scannersData } = useAvailableScanners();
  const startScan = useStartScan();
  const deleteScan = useDeleteScan();

  const availableScanners: ScannerInfo[] = (scannersData?.scanners ?? []).filter((s) => s.available);
  // Fall back to the first available scanner rather than the literal
  // "trivy" this used to assume. Trivy is a plugin now and may not be
  // the one installed, so a hard-coded default would preselect an
  // engine that does not exist on this host.
  const defaultScanner = scannersData?.defaultScannerId ?? availableScanners[0]?.id ?? '';

  const [selectedScanner, setSelectedScanner] = useState<string | null>(null);
  const [selectedScanId, setSelectedScanId] = useState<string | null>(null);

  const activeScanner = selectedScanner ?? defaultScanner;

  const scans = scansData?.items ?? [];
  const isRunning = scans.some((s) => s.status === 'running' || s.status === 'pending');

  function handleStartScan() {
    startScan.mutate({
      imageRef,
      scanner: activeScanner,
      environmentId: envId,
    });
  }

  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('scan.vulnerabilities')}
      </h3>

      {/* Scan controls */}
      <div className="mb-4 flex items-center gap-3">
        <Select
          value={activeScanner}
          onChange={setSelectedScanner}
          // Value is the config id, not the engine name: an estate can
          // run two Clairs, and only the id says which one to scan with.
          // Label is the operator's own name for it.
          options={availableScanners.map((s) => ({
            value: s.id,
            label: s.label || s.name,
          }))}
          ariaLabel={t('scan.scanner')}
          variant="outline"
          className="w-44"
        />
        <Button size="sm" onClick={handleStartScan} disabled={isRunning || startScan.isPending || !imageRef}>
          {isRunning || startScan.isPending ? (
            <IconLoader2 className="mr-1.5 size-3.5 animate-spin" />
          ) : (
            <IconScan className="mr-1.5 size-3.5" />
          )}
          {isRunning ? t('scan.scanning') : t('scan.scanImage')}
        </Button>
      </div>

      {/* Scan history */}
      <h4 className="mb-2 text-xs font-medium text-muted-foreground">{t('scan.scanHistory')}</h4>

      {scans.length === 0 ? (
        <p className="py-4 text-center text-xs text-muted-foreground">{t('scan.noScans')}</p>
      ) : (
        <div className="space-y-2">
          {scans.map((scan) => (
            <ScanRow
              key={scan.id}
              scan={scan}
              selected={selectedScanId === scan.id}
              onSelect={() => setSelectedScanId(selectedScanId === scan.id ? null : scan.id)}
              onDelete={() => deleteScan.mutate(scan.id)}
            />
          ))}
        </div>
      )}

      {/* Vulnerability details */}
      {selectedScanId && <VulnerabilityList scanId={selectedScanId} />}
    </div>
  );
}
