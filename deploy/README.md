# igniter deploy examples

These are example Kubernetes and KEDA manifests for running igniter in-cluster.
Copy them per host, edit every placeholder, and apply them in the namespace where
you want igniter to run. Put real credentials in a real Kubernetes Secret; never
commit live secret values.

The model is one Deployment per physical host or Proxmox VM group. KEDA owns the
Deployment replica count, and that count is the power intent:

- `0` means the host is not wanted; on scale-down igniter drains the configured
  nodes and powers the host off.
- `1` means the host is wanted; igniter powers it on, waits for its nodes, then
  holds the lease.

Pin the igniter pod to a node that never sleeps so it can wake and wind down the
hosts it controls.
