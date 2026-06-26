// Package power abstracts "turn this machine on/off" across the three homelab
// mechanisms: a BMC (iDRAC/IPMI), Wake-on-LAN + SSH, and the Proxmox API for
// VM-granular control.
package power

import (
	"context"
	"time"
)

// Default timings for the "soft -> verify -> hard" power-off every driver runs:
// try a graceful shutdown, wait softGrace for it to take effect, escalate to a
// forceful stop, wait hardGrace, then error if it never went down. Each driver
// exposes its own *_SOFT_GRACE / *_HARD_GRACE / *_POLL_INTERVAL overrides.
const (
	defaultSoftGrace = 3 * time.Minute
	defaultHardGrace = 90 * time.Second
	defaultPoll      = 15 * time.Second
)

// Driver is one host's power mechanism. Off SHOULD prefer a graceful shutdown
// (ACPI soft / clean) so hypervisors stop their VMs cleanly, but MUST ensure the
// machine actually powers off — verifying the host/VMs went down, escalating to
// a forceful stop if needed, and returning an error if it cannot confirm they
// did (so the caller never reports a shutdown that did not happen).
type Driver interface {
	IsOn(ctx context.Context) (bool, error)
	On(ctx context.Context) error
	Off(ctx context.Context) error
}
