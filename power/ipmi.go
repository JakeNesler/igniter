package power

import (
	"context"
	"fmt"
	"time"

	"github.com/bougou/go-ipmi"
)

// Default timings for the soft -> verify -> hard power-off sequence. Override
// with IPMI_SOFT_GRACE / IPMI_HARD_GRACE / IPMI_POLL_INTERVAL.
const (
	defaultSoftGrace = 3 * time.Minute
	defaultHardGrace = 90 * time.Second
	defaultPoll      = 15 * time.Second
)

// IPMI drives a BMC (e.g. Dell iDRAC) over lanplus. Off tries a graceful ACPI
// soft-off first (so a Proxmox host stops its VMs cleanly), then VERIFIES the
// host actually powered down and escalates to a hard power-down if it did not.
// A soft-off alone is fire-and-forget: the BMC ACKs the request and the call
// "succeeds" even when the host OS ignores the ACPI event (no acpid /
// power-button=ignore / a stuck VM), silently leaving the box running.
type IPMI struct {
	addr, user, pass string
	softGrace        time.Duration // wait for the ACPI soft-off to take effect
	hardGrace        time.Duration // wait for the hard power-down to take effect
	poll             time.Duration // power-state poll interval
}

func NewIPMI(addr, user, pass string, softGrace, hardGrace, poll time.Duration) (*IPMI, error) {
	if addr == "" || pass == "" {
		return nil, fmt.Errorf("IPMI_ADDR and IPMI_PASSWORD are required")
	}
	if softGrace <= 0 {
		softGrace = defaultSoftGrace
	}
	if hardGrace <= 0 {
		hardGrace = defaultHardGrace
	}
	if poll <= 0 {
		poll = defaultPoll
	}
	return &IPMI{
		addr: addr, user: user, pass: pass,
		softGrace: softGrace, hardGrace: hardGrace, poll: poll,
	}, nil
}

func (p *IPMI) client(ctx context.Context) (*ipmi.Client, error) {
	c, err := ipmi.NewClient(p.addr, 623, p.user, p.pass)
	if err != nil {
		return nil, err
	}
	c.WithInterface(ipmi.InterfaceLanplus)
	if err := c.Connect(ctx); err != nil {
		return nil, fmt.Errorf("ipmi connect %s: %w", p.addr, err)
	}
	return c, nil
}

func chassisOn(ctx context.Context, c *ipmi.Client) (bool, error) {
	st, err := c.GetChassisStatus(ctx)
	if err != nil {
		return false, err
	}
	return st.PowerIsOn, nil
}

func (p *IPMI) IsOn(ctx context.Context) (bool, error) {
	c, err := p.client(ctx)
	if err != nil {
		return false, err
	}
	defer c.Close(ctx)
	return chassisOn(ctx, c)
}

func (p *IPMI) On(ctx context.Context) error {
	c, err := p.client(ctx)
	if err != nil {
		return err
	}
	defer c.Close(ctx)
	_, err = c.ChassisControl(ctx, ipmi.ChassisControlPowerUp)
	return err
}

// Off shuts the host down and CONFIRMS it: ACPI soft-off -> wait softGrace ->
// if still on, hard power-down -> wait hardGrace -> error if it never went off
// (so the caller stops reporting a clean shutdown that did not happen). The BMC
// stays powered after a host power-off, so the verification polls keep working.
func (p *IPMI) Off(ctx context.Context) error {
	c, err := p.client(ctx)
	if err != nil {
		return err
	}
	defer c.Close(ctx)

	if on, err := chassisOn(ctx, c); err == nil && !on {
		return nil // already off — nothing to do
	}

	// Graceful first: lets a Proxmox host stop its VMs cleanly.
	if _, err := c.ChassisControl(ctx, ipmi.ChassisControlSoftShutdown); err != nil {
		return fmt.Errorf("ipmi soft-shutdown %s: %w", p.addr, err)
	}
	if p.waitOff(ctx, c, p.softGrace) {
		return nil
	}

	// Soft-off ignored. The power-down window is the operator's explicit intent,
	// so escalate to a hard power-down rather than leave the host running.
	if _, err := c.ChassisControl(ctx, ipmi.ChassisControlPowerDown); err != nil {
		return fmt.Errorf("ipmi hard power-down %s (after soft-off ignored): %w", p.addr, err)
	}
	if p.waitOff(ctx, c, p.hardGrace) {
		return nil
	}
	return fmt.Errorf("host %s still powered on after soft-off (%s) and hard power-down (%s)",
		p.addr, p.softGrace, p.hardGrace)
}

// waitOff polls the chassis power state until it reads off, the grace elapses,
// or ctx is cancelled. Transient status-read errors are tolerated (keep polling
// until the grace runs out) so a single BMC hiccup does not abort the wait.
func (p *IPMI) waitOff(ctx context.Context, c *ipmi.Client, grace time.Duration) bool {
	wctx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()
	t := time.NewTicker(p.poll)
	defer t.Stop()
	for {
		if on, err := chassisOn(wctx, c); err == nil && !on {
			return true
		}
		select {
		case <-wctx.Done():
			return false
		case <-t.C:
		}
	}
}
