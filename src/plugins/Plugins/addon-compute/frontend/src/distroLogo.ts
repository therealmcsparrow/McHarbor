// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

const DASHBOARD_ICONS = 'https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons/svg';

const DASHBOARD: Record<string, string> = {
  almalinux: 'alma-linux',
  androidx86: 'android',
  archlinuxarm: 'arch-linux',
  alpine: 'alpine-linux',
  archlinux: 'arch-linux',
  blissos: 'android',
  centos: 'centos',
  debian: 'debian-linux',
  endeavouros: 'endeavouros-linux',
  fedora: 'fedora',
  flatcar: 'flatcar',
  homeassistant: 'home-assistant',
  linuxmint: 'linux-mint',
  manjaro: 'manjaro-linux',
  nixos: 'nixos',
  opensuse: 'opensuse',
  oraclelinux: 'oracle',
  puppylinux: 'puppy-linux',
  rocky: 'rocky-linux',
  solus: 'solus',
  ubuntu: 'ubuntu-linux',
  windows: 'microsoft-windows',
  zorin: 'zorin-linux',
};

const SIMPLE: Record<string, string> = {
  elementary: 'elementary',
  mxlinux: 'mxlinux/1f2937/f9fafb',
  popos: 'popos',
};

const ALIASES: Record<string, string> = {
  alma: 'almalinux',
  alpinelinux: 'alpine',
  arch: 'archlinux',
  'arch-linux': 'archlinux',
  'android-x86': 'androidx86',
  android: 'androidx86',
  'arch-linux-arm': 'archlinuxarm',
  archarm: 'archlinuxarm',
  bliss: 'blissos',
  centosstream: 'centos',
  elementaryos: 'elementary',
  'linux-mint': 'linuxmint',
  mint: 'linuxmint',
  mx: 'mxlinux',
  'mx-linux': 'mxlinux',
  'oracle-linux': 'oraclelinux',
  oracle: 'oraclelinux',
  pop: 'popos',
  'pop-os': 'popos',
  pop_os: 'popos',
  puppy: 'puppylinux',
  rockylinux: 'rocky',
  suse: 'opensuse',
  zorinos: 'zorin',
};

function normalize(distro: string): string {
  const key = distro.trim().toLowerCase().replace(/\s+/g, '');
  return ALIASES[key] ?? key;
}

export function distroLogo(distro: string | undefined): string | null {
  if (!distro) return null;
  const key = normalize(distro);
  if (DASHBOARD[key]) return `${DASHBOARD_ICONS}/${DASHBOARD[key]}.svg`;
  if (SIMPLE[key]) return `https://cdn.simpleicons.org/${SIMPLE[key]}`;
  return null;
}

const LABELS: Record<string, string> = {
  almalinux: 'AlmaLinux',
  alpine: 'Alpine Linux',
  androidx86: 'Android-x86',
  archlinux: 'Arch Linux',
  archlinuxarm: 'Arch Linux ARM',
  blissos: 'Bliss OS',
  centos: 'CentOS',
  debian: 'Debian',
  elementary: 'elementary OS',
  endeavouros: 'EndeavourOS',
  fedora: 'Fedora',
  flatcar: 'Flatcar Container Linux',
  homeassistant: 'Home Assistant OS',
  linuxmint: 'Linux Mint',
  manjaro: 'Manjaro',
  mxlinux: 'MX Linux',
  nixos: 'NixOS',
  opensuse: 'openSUSE',
  oraclelinux: 'Oracle Linux',
  popos: 'Pop!_OS',
  puppylinux: 'Puppy Linux',
  rocky: 'Rocky Linux',
  solus: 'Solus',
  ubuntu: 'Ubuntu',
  windows: 'Windows',
  zorin: 'Zorin OS',
};

export function distroLabel(distro: string): string {
  return LABELS[normalize(distro)] ?? distro;
}
