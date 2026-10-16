# Installing Docker Engine

McHarbor manages Docker, and the recommended way to run McHarbor is itself a container,
so Docker Engine is the one prerequisite. This page covers installing it on every
distribution McHarbor supports.

Install **Docker Engine**, not Docker Desktop, on a server. Desktop is a developer
product that runs the daemon inside a VM and puts the socket somewhere McHarbor's
default bind mount does not expect.

## Which distributions

Two different lists, and confusing them is the most common mistake here.

**To run the McHarbor container:** any supported distribution below that runs a current
Docker Engine. There is no glibc floor for the container, because it carries its own
userland: Ubuntu 22.04, Debian 11 and RHEL/Rocky/Alma 9 all run it.

**Alpine is not supported.** Neither natively nor as a container host.

**To install McHarbor natively** (`.deb`, `.rpm`, tarball): glibc 2.36 or newer: Debian
12+, Ubuntu 24.04+, Fedora 38+ only. See
[Native Linux install](./native-linux.md) for why.

So if your host is below the glibc floor, the container is the answer, not a blocker.

You need Docker Engine 20.10 or newer for the Compose plugin (`docker compose`, not the
legacy `docker-compose` script). Every procedure below installs a current version.

## Debian 12 / 13

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker
```

## Ubuntu 22.04 / 24.04 / 25.04

Identical to Debian except the repository path and the codename variable. Ubuntu
derivatives such as Linux Mint set `UBUNTU_CODENAME`, which is why it is used in
preference to `VERSION_CODENAME`.

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}") stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker
```

Remove Ubuntu's own `docker.io` / `docker-compose` packages first if they are installed;
they conflict with Docker's repository and ship an older daemon:

```bash
sudo apt-get remove -y docker.io docker-doc docker-compose podman-docker containerd runc
```

## Fedora 38+

```bash
sudo dnf -y install dnf-plugins-core
sudo dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo
sudo dnf -y install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker
```

On Fedora 41 and newer, `dnf5` changed the subcommand:

```bash
sudo dnf config-manager addrepo --from-repofile=https://download.docker.com/linux/fedora/docker-ce.repo
```

## RHEL 9 / Rocky Linux 9 / AlmaLinux 9

Supported for running the container. The **native** McHarbor package does not work here:
glibc 2.34 is below the floor.

```bash
sudo dnf -y install dnf-plugins-core
sudo dnf config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
sudo dnf -y install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker
```

On RHEL proper, substitute `rhel` for `centos` in the repository URL. If `podman` and
`buildah` are installed they conflict with `docker-ce`; remove them first.

## openSUSE Leap / Tumbleweed

Docker publishes no openSUSE repository; the distribution's own packages are current.

```bash
sudo zypper install -y docker docker-compose docker-compose-switch
sudo systemctl enable --now docker
```

## Arch Linux

```bash
sudo pacman -S --needed docker docker-compose docker-buildx
sudo systemctl enable --now docker
```


## After installing, on every distribution

### 1. Let your user reach the socket

```bash
sudo usermod -aG docker "$USER"
newgrp docker
```

> **The `docker` group is root-equivalent.** Anyone in it can start a container that
> mounts the host filesystem and so become root. Add only accounts you would give root
> to. This is the same trust boundary McHarbor's own socket mount has; it is just
> expressed as group membership.

### 2. Verify

```bash
docker run --rm hello-world
docker compose version
```

`docker compose version` must succeed. If only `docker-compose` works, you have the
legacy standalone script and not the plugin; install `docker-compose-plugin` from the
procedures above.

### 3. Check the socket path

McHarbor's Compose file bind-mounts `/var/run/docker.sock`, which is correct for every
procedure on this page. It differs under rootless Docker and Docker Desktop:

```bash
docker context inspect --format '{{.Endpoints.docker.Host}}'
```

If that prints anything other than `unix:///var/run/docker.sock`, mount the path it
reports instead.

## Rootless Docker

Rootless works, with two consequences worth knowing before you choose it:

- The socket moves to `$XDG_RUNTIME_DIR/docker.sock`, so the bind mount in McHarbor's
  Compose file must be changed to match.
- Host metrics and some container operations are limited by the user namespace. McHarbor
  reports what it can see, which is less than under a rootful daemon.

Rootful is the tested configuration.

## Next

- [Installation](./README.md): the McHarbor Compose file
- [Native Linux install](./native-linux.md): running McHarbor without a container
- [Troubleshooting](./troubleshooting.md): when it does not come up
