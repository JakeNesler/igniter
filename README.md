# igniter

Homelab host/VM power controller, designed to be **scaled by KEDA** — the
replica count is the power intent:

- **0 → 1**: power the host ON (iDRAC IPMI / Wake-on-LAN / Proxmox API), wait
  for its k8s nodes Ready, uncordon, then hold a "host is wanted" lease
  (re-asserting power if someone turns the box off while wanted).
- **1 → 0** (SIGTERM): cordon + drain the host's nodes, graceful power off, exit.

One Deployment per host (or Proxmox VM group), pinned to a node that never
sleeps. KEDA composes the intent upstream: a `cron` trigger for the daily
window, a `prometheus` trigger (pending pods) for demand-wake. Example
manifests live in [`deploy/`](deploy/).

Power drivers — each does a graceful shutdown, then **verifies the machine
actually went down and escalates if it didn't** (so igniter never reports a
power-off that didn't happen): `ipmi` (BMC; ACPI soft-off → hard power-down),
`wol` (magic packet on; SSH `shutdown` off → best-effort forced power-off, no
BMC to fall back on), `proxmox` (API token; per-VM `shutdown` → hard `stop`).
`igniterctl status|on|off` runs the same drivers by hand.

## Configure

All config is environment variables — copy [`.env.example`](.env.example) and
fill in your host + chosen driver:

```sh
cp .env.example .env       # then edit
set -a; . ./.env; set +a   # export them into your shell
igniterctl status          # ipmi|wol|proxmox, driven by IGNITER_POWER
```

Required: `IGNITER_HOST`, `IGNITER_POWER`, and the selected driver's vars
(`.env.example` documents each driver block). Everything else has a default.
Secrets (`*_PASSWORD`, `PVE_TOKEN_SECRET`) come from a real `.env` or a k8s
Secret — never committed (`.env` is gitignored).

For Kubernetes, copy the examples in [`deploy/`](deploy/) per host, edit the
placeholders, build + push the image to your registry, and apply the manifests.

## Run

```sh
# one-shot, by hand
docker run --rm --env-file .env --network host ghcr.io/<you>/igniter igniterctl status

# in-cluster: a Deployment per host, scaled by KEDA (replica count = power intent).
# Inject IGNITER_DEPLOYMENT/IGNITER_NAMESPACE from the downward API and the
# secret vars from a Secret. Example manifests: deploy/.
```

Adding a power driver = implement `power.Driver` (On/Off/IsOn) and wire a case in
`power.FromEnv`; the node lifecycle + KEDA lease logic are driver-agnostic.
