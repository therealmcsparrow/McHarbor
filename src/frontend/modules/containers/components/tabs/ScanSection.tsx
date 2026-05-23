// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconScan,
  IconLoader2,
  IconTrash,
  IconExternalLink,
} from "@tabler/icons-react";
import { Button } from "@resources/components/ui/Button";
import { Badge } from "@resources/components/ui/Badge";
import { useEnvironmentStore } from "@resources/stores/environment";
import { formatDate } from "@resources/utils/format";
import {
  useImageScans,
  useScanVulnerabilities,
  useStartScan,
  useDeleteScan,
  type Scan,
  type ScanVulnerability,
} from "../../hooks/useScans";
import {
  useAvailableScanners,
  type ScannerInfo,
} from "@resources/hooks/useScannerSettings";

type ScanSectionProps = {
  imageRef: string;
};

const SEVERITY_VARIANT: Record<
  string,
  "destructive" | "warning" | "default" | "secondary"
> = {
  critical: "destructive",
  high: "destructive",
  medium: "warning",
  low: "secondary",
};

const STATUS_VARIANT: Record<
  string,
  "success" | "default" | "warning" | "destructive"
> = {
  completed: "success",
  running: "default",
  pending: "warning",
  failed: "destructive",
};

export function ScanSection({ imageRef }: ScanSectionProps) {
  const { t } = useTranslation("containers");
  const envId = useEnvironmentStore((s) => s.currentId);
  const { data: scansData } = useImageScans(imageRef);
  const { data: scannersData } = useAvailableScanners();
  const startScan = useStartScan();
  const deleteScan = useDeleteScan();

  const availableScanners: ScannerInfo[] = (
    scannersData?.scanners ?? []
  ).filter((s) => s.available);
  const defaultScanner = scannersData?.defaultScanner ?? "trivy";

  const [selectedScanner, setSelectedScanner] = useState<string | null>(null);
  const [selectedScanId, setSelectedScanId] = useState<string | null>(null);

  const activeScanner = selectedScanner ?? defaultScanner;

  const scans = scansData?.items ?? [];
  const isRunning = scans.some(
    (s) => s.status === "running" || s.status === "pending",
  );

  function handleStartScan() {
    startScan.mutate({
      imageRef,
      scanner: activeScanner,
      environmentId: envId,
    });
  }

  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("scan.vulnerabilities")}
      </h3>

      {/* Scan controls */}
      <div className="mb-4 flex items-center gap-3">
        <select
          value={activeScanner}
          onChange={(e) => setSelectedScanner(e.target.value)}
          className="rounded-md border border-border bg-muted px-3 py-1.5 text-sm text-foreground focus:border-primary focus:outline-none"
        >
          {availableScanners.map((s) => (
            <option key={s.name} value={s.name}>
              {s.name.charAt(0).toUpperCase() + s.name.slice(1)}
            </option>
          ))}
        </select>
        <Button
          size="sm"
          onClick={handleStartScan}
          disabled={isRunning || startScan.isPending || !imageRef}
        >
          {isRunning || startScan.isPending ? (
            <IconLoader2 className="mr-1.5 size-3.5 animate-spin" />
          ) : (
            <IconScan className="mr-1.5 size-3.5" />
          )}
          {isRunning ? t("scan.scanning") : t("scan.scanImage")}
        </Button>
      </div>

      {/* Scan history */}
      <h4 className="mb-2 text-xs font-medium text-muted-foreground">
        {t("scan.scanHistory")}
      </h4>

      {scans.length === 0 ? (
        <p className="py-4 text-center text-xs text-muted-foreground">
          {t("scan.noScans")}
        </p>
      ) : (
        <div className="space-y-2">
          {scans.map((scan) => (
            <ScanRow
              key={scan.id}
              scan={scan}
              selected={selectedScanId === scan.id}
              onSelect={() =>
                setSelectedScanId(selectedScanId === scan.id ? null : scan.id)
              }
              onDelete={() => deleteScan.mutate(scan.id)}
              t={t}
            />
          ))}
        </div>
      )}

      {/* Vulnerability details */}
      {selectedScanId && <VulnerabilityList scanId={selectedScanId} t={t} />}
    </div>
  );
}

function ScanRow({
  scan,
  selected,
  onSelect,
  onDelete,
  t,
}: {
  scan: Scan;
  selected: boolean;
  onSelect: () => void;
  onDelete: () => void;
  t: (key: string) => string;
}) {
  return (
    <div
      className={`flex items-center justify-between rounded-md border px-3 py-2 text-xs transition-colors ${
        selected
          ? "border-primary/50 bg-primary/5"
          : "border-border bg-muted/30 hover:border-border/80"
      }`}
    >
      <Button
        type="button"
        variant="ghost"
        className="h-auto flex-1 justify-start gap-3 p-0 text-left hover:bg-transparent"
        onClick={onSelect}
      >
        <Badge
          variant={STATUS_VARIANT[scan.status] ?? "secondary"}
          className="text-[10px]"
        >
          {t(`scan.${scan.status}`)}
        </Badge>
        <span className="font-mono text-muted-foreground">{scan.scanner}</span>
        {scan.status === "completed" && (
          <div className="flex gap-1.5">
            {scan.criticalCount > 0 && (
              <Badge variant="destructive" className="text-[10px]">
                C:{scan.criticalCount}
              </Badge>
            )}
            {scan.highCount > 0 && (
              <Badge variant="destructive" className="text-[10px]">
                H:{scan.highCount}
              </Badge>
            )}
            {scan.mediumCount > 0 && (
              <Badge variant="warning" className="text-[10px]">
                M:{scan.mediumCount}
              </Badge>
            )}
            {scan.lowCount > 0 && (
              <Badge variant="secondary" className="text-[10px]">
                L:{scan.lowCount}
              </Badge>
            )}
          </div>
        )}
        <span className="ml-auto text-muted-foreground">
          {scan.completedAt
            ? formatDate(scan.completedAt)
            : formatDate(scan.startedAt)}
        </span>
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t("scan.deleteScan")}
        className="ml-2 size-6"
        onClick={(e) => {
          e.stopPropagation();
          onDelete();
        }}
      >
        <IconTrash className="size-3.5" />
      </Button>
    </div>
  );
}

function VulnerabilityList({
  scanId,
  t,
}: {
  scanId: string;
  t: (key: string) => string;
}) {
  const { data, isLoading } = useScanVulnerabilities(scanId);
  const vulns = data?.items ?? [];

  if (isLoading) {
    return (
      <div className="mt-4 flex items-center justify-center py-6">
        <IconLoader2 className="size-4 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (vulns.length === 0) {
    return (
      <p className="mt-4 py-4 text-center text-xs text-muted-foreground">
        {t("scan.noVulnerabilities")}
      </p>
    );
  }

  return (
    <div className="mt-4">
      <h4 className="mb-2 text-xs font-medium text-muted-foreground">
        {t("scan.vulnerabilities")} ({vulns.length})
      </h4>
      <div className="overflow-x-auto rounded-md border border-border">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-border bg-muted/50">
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                {t("scan.severity")}
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                {t("scan.cve")}
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                {t("scan.package")}
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                {t("scan.version")}
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                {t("scan.fixedIn")}
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                {t("scan.title")}
              </th>
            </tr>
          </thead>
          <tbody>
            {vulns.map((v) => (
              <VulnRow key={v.id} vuln={v} />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function VulnRow({ vuln }: { vuln: ScanVulnerability }) {
  return (
    <tr className="border-b border-border/50 last:border-0 hover:bg-muted/30">
      <td className="px-3 py-1.5">
        <Badge
          variant={SEVERITY_VARIANT[vuln.severity] ?? "secondary"}
          className="text-[10px]"
        >
          {vuln.severity}
        </Badge>
      </td>
      <td className="px-3 py-1.5 font-mono">
        {vuln.url ? (
          <a
            href={vuln.url}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 text-primary hover:underline"
          >
            {vuln.vulnId}
            <IconExternalLink className="size-3" />
          </a>
        ) : (
          vuln.vulnId
        )}
      </td>
      <td className="px-3 py-1.5 font-mono text-foreground">{vuln.pkgName}</td>
      <td className="px-3 py-1.5 font-mono text-muted-foreground">
        {vuln.pkgVersion}
      </td>
      <td className="px-3 py-1.5 font-mono text-muted-foreground">
        {vuln.fixedVersion || "-"}
      </td>
      <td className="max-w-xs truncate px-3 py-1.5 text-muted-foreground">
        {vuln.title || "-"}
      </td>
    </tr>
  );
}
