package power

import (
	"context"
	"fmt"

	"github.com/bougou/go-ipmi"
)

// IPMI drives a BMC (e.g. Dell iDRAC) over lanplus. Off sends ACPI soft so the
// host OS shuts its VMs down cleanly before powering off.
type IPMI struct {
	addr, user, pass string
}

func NewIPMI(addr, user, pass string) (*IPMI, error) {
	if addr == "" || pass == "" {
		return nil, fmt.Errorf("IPMI_ADDR and IPMI_PASSWORD are required")
	}
	return &IPMI{addr: addr, user: user, pass: pass}, nil
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

func (p *IPMI) IsOn(ctx context.Context) (bool, error) {
	c, err := p.client(ctx)
	if err != nil {
		return false, err
	}
	defer c.Close(ctx)
	st, err := c.GetChassisStatus(ctx)
	if err != nil {
		return false, err
	}
	return st.PowerIsOn, nil
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

func (p *IPMI) Off(ctx context.Context) error {
	c, err := p.client(ctx)
	if err != nil {
		return err
	}
	defer c.Close(ctx)
	// ACPI soft-off: Proxmox catches it and stops VMs cleanly.
	_, err = c.ChassisControl(ctx, ipmi.ChassisControlSoftShutdown)
	return err
}
