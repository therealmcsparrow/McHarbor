#!/usr/bin/env node
// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { copyFileSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..");
const out = resolve(process.env.AGENT_OUT || join(root, "agents"));
const catalog = join(out, "catalog");
const versionPattern = /^[0-9]+\.[0-9]+\.[0-9]+$/;
const buildPattern = /^[0-9]{12}$/;

const read = (path) => (existsSync(path) ? readFileSync(path, "utf8").trim() : "");
const fail = (message) => {
  console.error(`publish-agent-catalog: ${message}`);
  process.exit(1);
};

const version = read(join(out, "VERSION")) || read(join(root, "VERSION"));
if (!versionPattern.test(version)) fail(`no valid VERSION in ${out} (got "${version}")`);
const build = read(join(out, "BUILD"));
if (!buildPattern.test(build)) fail(`no valid BUILD in ${out} (got "${build}")`);
const sumsPath = join(out, "SHA256SUMS");
if (!existsSync(sumsPath)) fail(`${sumsPath} is missing; run scripts/development/build-agent.sh first`);

const listed = readFileSync(sumsPath, "utf8")
  .split(/\r?\n/)
  .map((line) => line.trim().match(/^[0-9a-f]{64} [ *]?(.+)$/))
  .filter(Boolean)
  .map((m) => m[1]);
if (!listed.some((name) => name.startsWith("mcharbor-agent-"))) fail("SHA256SUMS lists no agent binaries");

const files = [...new Set(["SHA256SUMS", "SHA256SUMS.sig", "image.txt", ...listed])].filter((name) =>
  existsSync(join(out, name)),
);
const missing = listed.filter((name) => !existsSync(join(out, name)));
if (missing.length) fail(`SHA256SUMS names files that are not in ${out}: ${missing.join(", ")}`);

const target = join(catalog, version, build);
rmSync(target, { recursive: true, force: true });
mkdirSync(target, { recursive: true });
for (const name of files) copyFileSync(join(out, name), join(target, name));
if (!files.includes("SHA256SUMS.sig")) {
  console.warn(`publish-agent-catalog: ${version}/${build} is unsigned; McHarbor refuses it from a source until SHA256SUMS.sig exists`);
}

const compare = (a, b) => {
  const pa = a.version.split(".").map(Number);
  const pb = b.version.split(".").map(Number);
  for (let i = 0; i < 3; i++) if (pa[i] !== pb[i]) return pb[i] - pa[i];
  return b.build.localeCompare(a.build);
};

const dirs = (path, pattern) =>
  readdirSync(path, { withFileTypes: true })
    .filter((entry) => entry.isDirectory() && pattern.test(entry.name))
    .map((entry) => entry.name);

const versions = dirs(catalog, versionPattern)
  .flatMap((v) => dirs(join(catalog, v), buildPattern).map((b) => ({ version: v, build: b })))
  .filter((entry) => existsSync(join(catalog, entry.version, entry.build, "SHA256SUMS")))
  .sort(compare);

const index = { version: versions[0].version, build: versions[0].build, versions };
writeFileSync(join(catalog, "index.json"), `${JSON.stringify(index, null, 2)}\n`);

console.log(`publish-agent-catalog: ${version}/${build} -> ${target} (${files.length} files)`);
console.log(
  `publish-agent-catalog: index.json lists ${versions.map((v) => `${v.version}/${v.build}`).join(", ")}; newest ${index.version}/${index.build}`,
);
console.log("Upload the contents of catalog/ to https://www.mcharbor.eu/catalog/agents/");
