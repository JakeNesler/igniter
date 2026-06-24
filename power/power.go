// Package power abstracts "turn this machine on/off" across the three homelab
// mechanisms: a BMC (iDRAC/IPMI), Wake-on-LAN + SSH, and the Proxmox API for
// VM-granular control.
package power

import "context"

// Driver is one host's power mechanism. Off MUST be graceful (ACPI soft /
// clean shutdown) — hypervisors get to stop their VMs cleanly.
type Driver interface {
	IsOn(ctx context.Context) (bool, error)
	On(ctx context.Context) error
	Off(ctx context.Context) error
}
