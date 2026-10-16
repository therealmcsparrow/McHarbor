# Virtual Machines troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Creation fails on a host that looks fine | No KVM support, or the agent cannot reach `/dev/kvm` | Run the capability check on the host; it reports exactly which prerequisite is missing. |
| The console will not open | Missing `vm.console` | It is a separate key from `vm.view` on purpose. |
| The VM has no IP address | No DHCP answer reaches it; see the diagnosis in Compute Foundation | [A guest gets no address](../compute-foundation/guest-networking.md#a-guest-gets-no-address). |
| An agent setting like `MCHARBOR_VM_VHOST` made no difference | It is read when QEMU starts; a reboot from inside the guest keeps the same QEMU process | Stop and start the VM from McHarbor. |
| A cloud image boots, but the wizard's user and password are rejected | The guest never read its cloud-init seed: an agent older than this release wrote `seed.iso` without attaching it, or created the account with a `sudo` group RHEL-family images do not have | Update the agent and start the machine again; a machine that never booted with the seed gets it on that start. See [Checking the cloud-init seed](#checking-the-cloud-init-seed). |
| Start fails with "QEMU … never opened its monitor socket" | QEMU exited while starting; the reason is in the host journal | `journalctl -u mcharbor-vm-<guestId>.service -n 50` on the host. "There is no option group 'spice'" means the SPICE module is missing; see [SPICE console](./spice-console/README.md). |

A feature plugin has no connection to test. What to check after installing is that its
sidebar section appears. It stays hidden until a role holds at least the `view` key it
declares.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i addon-kvm
```

This plugin owns tables, so a failure at install time may be a migration rather than a
configuration problem. Its migrations are `0001_init.sql`.

## Checking the cloud-init seed

On the host, with the machine's id from its detail page:

```bash
ID=<guestId>
sudo grep -c seed.iso /var/lib/mcharbor/compute/vm/$ID/args.json      # attached on this start?
sudo mount -o loop,ro /var/lib/mcharbor/compute/vm/$ID/seed.iso /mnt   # what the guest is given
sudo cat /mnt/user-data /mnt/meta-data; sudo umount /mnt
```

`args.json` is QEMU's command line for the last start. It names `seed.iso` only while the
seed is still to be delivered; once `seed.delivered` exists beside it, later starts leave it
off on purpose. `user-data` carries the password only as a `$6# Virtual Machines troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Creation fails on a host that looks fine | No KVM support, or the agent cannot reach `/dev/kvm` | Run the capability check on the host; it reports exactly which prerequisite is missing. |
| The console will not open | Missing `vm.console` | It is a separate key from `vm.view` on purpose. |
| The VM has no IP address | No DHCP answer reaches it; see the diagnosis in Compute Foundation | [A guest gets no address](../compute-foundation/guest-networking.md#a-guest-gets-no-address). |
| An agent setting like `MCHARBOR_VM_VHOST` made no difference | It is read when QEMU starts; a reboot from inside the guest keeps the same QEMU process | Stop and start the VM from McHarbor. |
| A cloud image boots, but the wizard's user and password are rejected | The guest never read its cloud-init seed: an agent older than this release wrote `seed.iso` without attaching it, or created the account with a `sudo` group RHEL-family images do not have | Update the agent and start the machine again; a machine that never booted with the seed gets it on that start. See [Checking the cloud-init seed](#checking-the-cloud-init-seed). |
| Start fails with "QEMU … never opened its monitor socket" | QEMU exited while starting; the reason is in the host journal | `journalctl -u mcharbor-vm-<guestId>.service -n 50` on the host. "There is no option group 'spice'" means the SPICE module is missing; see [SPICE console](./spice-console/README.md). |

A feature plugin has no connection to test. What to check after installing is that its
sidebar section appears. It stays hidden until a role holds at least the `view` key it
declares.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i addon-kvm
```

This plugin owns tables, so a failure at install time may be a migration rather than a
configuration problem. Its migrations are `0001_init.sql`.

 hash.

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Virtual Machines](./README.md): what it is and how it works
