// Package power abstracts "turn this machine on/off" across the three homelab
// mechanisms: a BMC (iDRAC/IPMI), Wake-on-LAN + SSH, and the Proxmox API for
// VM-granular control.
package power

import "context"

// Driver is one host's power mechanism. Off SHOULD prefer a graceful shutdown
// (ACPI soft / clean) so hypervisors stop their VMs cleanly, but MUST ensure the
// machine actually powers off — returning an error if it cannot confirm the host
// went down (e.g. the IPMI driver verifies and escalates to a hard power-down).
type Driver interface {
	IsOn(ctx context.Context) (bool, error)
	On(ctx context.Context) error
	Off(ctx context.Context) error
}
