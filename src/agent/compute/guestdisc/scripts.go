// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package guestdisc

// linuxScript installs and enables the agent from the distribution's own
// repository, which is where every distribution that ships it wants it from.
const linuxScript = `#!/bin/sh
# McHarbor guest agent installer.
set -eu

if [ "$(id -u)" -ne 0 ]; then
	echo "This script must run as root. Try: sudo sh $0" >&2
	exit 1
fi

install_package() {
	if command -v apt-get >/dev/null 2>&1; then
		apt-get update
		DEBIAN_FRONTEND=noninteractive apt-get install -y qemu-guest-agent
	elif command -v dnf >/dev/null 2>&1; then
		dnf install -y qemu-guest-agent
	elif command -v yum >/dev/null 2>&1; then
		yum install -y qemu-guest-agent
	elif command -v zypper >/dev/null 2>&1; then
		zypper --non-interactive install qemu-guest-agent
	elif command -v pacman >/dev/null 2>&1; then
		pacman -Sy --noconfirm qemu-guest-agent
	elif command -v apk >/dev/null 2>&1; then
		apk add --no-cache qemu-guest-agent
	else
		echo "No supported package manager found. Install qemu-guest-agent by hand." >&2
		exit 1
	fi
}

enable_service() {
	if command -v systemctl >/dev/null 2>&1; then
		systemctl enable --now qemu-guest-agent
	elif command -v rc-update >/dev/null 2>&1; then
		rc-update add qemu-guest-agent default
		rc-service qemu-guest-agent start
	else
		echo "Installed, but no init system was recognised." >&2
		echo "Start qemu-guest-agent yourself for McHarbor to see this machine." >&2
		exit 1
	fi
}

install_package
enable_service
echo "Guest agent installed. McHarbor sees it within about a minute."
`

// bsdScript covers FreeBSD's packaged agent; other BSDs need it from ports and
// the script says so rather than failing obscurely.
const bsdScript = `#!/bin/sh
# McHarbor guest agent installer.
set -eu

if [ "$(id -u)" -ne 0 ]; then
	echo "This script must run as root. Try: sudo sh $0" >&2
	exit 1
fi

if ! command -v pkg >/dev/null 2>&1; then
	echo "No pkg command found. Install qemu-guest-agent from ports." >&2
	exit 1
fi

pkg install -y qemu-guest-agent

if command -v sysrc >/dev/null 2>&1; then
	sysrc qemu_guest_agent_enable=YES
fi
service qemu-guest-agent start

echo "Guest agent installed. McHarbor sees it within about a minute."
`

// windowsScript runs the installer carried on this disc. It is silent because
// the operator is already watching McHarbor, not the guest's desktop.
//
// It also enables the VioGpu Resolution Service on a virtio-gpu guest: the
// viogpudo driver applies the console's resize requests only while that service
// runs, and the guest-tools installer ships it as a feature that is off by default.
// The same driver installs with HWCursor=0, which draws the pointer into the
// picture so every move waits for a frame round trip; the script turns it on.
const windowsScript = `# McHarbor guest agent installer.
param([switch]$SkipDisplayService, [switch]$SoftwareCursor)
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$installer = Join-Path $root 'virtio-win-guest-tools.exe'

if (-not (Test-Path $installer)) {
	Write-Error "virtio-win-guest-tools.exe is missing from this disc."
	exit 1
}

Write-Host 'Installing the virtio guest tools. This takes a minute.'
$process = Start-Process -FilePath $installer -ArgumentList '/install', '/quiet', '/norestart' -Wait -PassThru

if ($process.ExitCode -ne 0 -and $process.ExitCode -ne 3010) {
	Write-Error "The installer exited with code $($process.ExitCode)."
	exit $process.ExitCode
}

Write-Host 'Guest agent installed. McHarbor sees it within about a minute.'

function Get-ResizeService {
	Get-Service -Name 'vgpusrv', 'VioGpuSrv' -ErrorAction SilentlyContinue | Select-Object -First 1
}

function Add-ResizeServiceFromInstaller {
	$layout = Join-Path $env:TEMP 'mcharbor-virtio-layout'
	if (Test-Path $layout) { Remove-Item -Recurse -Force $layout }
	try {
		$extract = Start-Process -FilePath $installer -ArgumentList '/layout', ('"' + $layout + '"'), '/quiet' -Wait -PassThru
		if ($extract.ExitCode -ne 0 -or -not (Test-Path $layout)) { return }
		$arch = if ([Environment]::Is64BitOperatingSystem) { 'x64' } else { 'x86' }
		$msi = Get-ChildItem -Path $layout -Recurse -Filter "virtio-win-gt-$arch.msi" | Select-Object -First 1
		if (-not $msi) { return }
		$null = Start-Process -FilePath 'msiexec.exe' -ArgumentList '/i', ('"' + $msi.FullName + '"'), 'ADDLOCAL=FE_viogpudo_service', '/qn', '/norestart' -Wait -PassThru
	} finally {
		Remove-Item -Recurse -Force $layout -ErrorAction SilentlyContinue
	}
}

function Get-ViogpudoFolders {
	$os = Get-CimInstance Win32_OperatingSystem
	$build = [int]$os.BuildNumber
	$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'ARM64' } elseif ([Environment]::Is64BitOperatingSystem) { 'amd64' } else { 'x86' }
	if ($os.ProductType -ne 1) {
		if ($build -ge 26100) { $names = '2k25', '2k22' }
		elseif ($build -ge 20348) { $names = '2k22', '2k19' }
		elseif ($build -ge 17763) { $names = '2k19', '2k16' }
		else { $names = @('2k16') }
	} elseif ($build -ge 22000) {
		$names = 'w11', 'w10'
	} else {
		$names = @('w10')
	}
	foreach ($name in $names) { Join-Path (Join-Path 'viogpudo' $name) $arch }
}

function Add-ResizeServiceFromDisc {
	$system = [Environment]::SystemDirectory
	foreach ($drive in Get-CimInstance Win32_LogicalDisk -Filter 'DriveType = 5') {
		foreach ($folder in Get-ViogpudoFolders) {
			$source = Join-Path ($drive.DeviceID + '\') $folder
			$service = Join-Path $source 'vgpusrv.exe'
			$helper = Join-Path $source 'viogpuap.exe'
			if ((Test-Path $service) -and (Test-Path $helper)) {
				Copy-Item -Path $service, $helper -Destination $system -Force
				& (Join-Path $system 'vgpusrv.exe') -i | Out-Host
				return
			}
		}
	}
}

$gpu = Get-CimInstance Win32_PnPEntity | Where-Object { $_.DeviceID -like 'PCI\VEN_1AF4&DEV_1050*' }
if ($SkipDisplayService) {
	Write-Host 'Skipping the display resize service, as asked.'
} elseif (-not $gpu) {
	Write-Host 'No virtio-gpu adapter, so the display resize service is not needed.'
} else {
	Write-Host 'Enabling the VioGpu Resolution Service so the display follows the console window.'
	if (-not (Get-ResizeService)) {
		try { Add-ResizeServiceFromInstaller } catch { Write-Warning "The guest tools could not add the service: $_" }
	}
	if (-not (Get-ResizeService)) {
		try { Add-ResizeServiceFromDisc } catch { Write-Warning "The service could not be copied from the virtio-win disc: $_" }
	}
	$resize = Get-ResizeService
	if ($resize) {
		try {
			Set-Service -Name $resize.Name -StartupType Automatic
			if ($resize.Status -ne 'Running') { Start-Service -Name $resize.Name }
			Write-Host 'Display resize service running. The console now resizes this desktop.'
		} catch {
			Write-Warning "The display resize service is installed but did not start; it starts at the next boot. $_"
		}
	} else {
		Write-Warning 'These guest tools do not carry the display resize service. Attach the virtio-win ISO and run this script again, or copy vgpusrv.exe and viogpuap.exe from viogpudo\<version>\<arch> on that ISO into C:\Windows\System32 and run "vgpusrv.exe -i" as Administrator.'
	}
}

$cursorReboot = $false
if ($gpu -and -not $SoftwareCursor) {
	$class = 'HKLM:\SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}'
	$drivers = Get-ChildItem -Path $class -ErrorAction SilentlyContinue | Where-Object {
		(Get-ItemProperty -Path $_.PSPath -Name MatchingDeviceId -ErrorAction SilentlyContinue).MatchingDeviceId -like 'pci\ven_1af4&dev_1050*'
	}
	if ($drivers) {
		foreach ($driver in $drivers) {
			if ((Get-ItemProperty -Path $driver.PSPath -Name HWCursor -ErrorAction SilentlyContinue).HWCursor -ne 1) {
				Set-ItemProperty -Path $driver.PSPath -Name HWCursor -Value 1 -Type DWord
				$cursorReboot = $true
			}
		}
		if ($cursorReboot) {
			Write-Host 'Hardware cursor turned on: the console draws the pointer itself, so it follows the mouse without lag.'
		}
	} else {
		Write-Warning 'The VirtIO GPU driver is not bound yet, so the hardware cursor could not be turned on. Run this script again after the reboot.'
	}
}

if ($process.ExitCode -eq 3010 -or $cursorReboot) {
	Write-Host 'A reboot is needed to finish installing the drivers.'
}
`

// readmeText is what an operator who mounts the disc and looks at it first
// should read; every family's one command is on it.
const readmeText = `McHarbor guest agent disc
=========================

The QEMU guest agent lets McHarbor freeze this machine's filesystems before a
snapshot and report its addresses. The virtio-serial channel it talks over is
already wired by McHarbor, so only the in-guest half is missing.

Linux    sudo sh /mnt/cdrom/install-guest-agent.sh
BSD      sudo sh /mnt/cdrom/install-guest-agent.sh
Windows  powershell -ExecutionPolicy Bypass -File D:\install-guest-agent.ps1

Mount point and drive letter depend on the guest. On Linux the disc is usually
already mounted under /media or /run/media.

Linux and BSD install the agent from the distribution's own repository, so the
machine needs to reach it. Windows installs from virtio-win-guest-tools.exe on
this disc and needs no network at all.

On a Windows guest with a virtio-gpu display, the script also enables the VioGpu
Resolution Service, so the desktop follows the McHarbor console window when
"Resize guest to window" is selected. Pass -SkipDisplayService to leave it off.
It also turns on the driver's hardware cursor, so the pointer does not lag in
the VNC console; pass -SoftwareCursor to leave that off.
`
