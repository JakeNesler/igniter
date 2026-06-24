package power

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Proxmox drives a SET of VMs on one PVE node via the API (token auth) — the
// VM-granular tier: scale a subset of a host's k3s VMs without touching the
// host. On/Off are graceful (qm start / qm shutdown semantics).
type Proxmox struct {
	base   string // https://pve:8006
	auth   string // PVEAPIToken=user@realm!name=secret
	node   string
	vmids  []string
	client *http.Client
}

func NewProxmox(url, tokenID, tokenSecret, node string, vmids []string) (*Proxmox, error) {
	if url == "" || tokenID == "" || tokenSecret == "" || node == "" {
		return nil, fmt.Errorf("PVE_URL, PVE_TOKEN_ID, PVE_TOKEN_SECRET, PVE_NODE are required")
	}
	clean := make([]string, 0, len(vmids))
	for _, v := range vmids {
		if v = strings.TrimSpace(v); v != "" {
			clean = append(clean, v)
		}
	}
	if len(clean) == 0 {
		return nil, fmt.Errorf("PVE_VMIDS is required")
	}
	return &Proxmox{
		base:  strings.TrimRight(url, "/"),
		auth:  "PVEAPIToken=" + tokenID + "=" + tokenSecret,
		node:  node,
		vmids: clean,
		client: &http.Client{
			Timeout: 15 * time.Second,
			// Homelab PVE serves a self-signed cert on the LAN.
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		},
	}, nil
}

func (p *Proxmox) call(ctx context.Context, method, path string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.base+"/api2/json"+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", p.auth)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pve %s %s: %s", method, path, resp.Status)
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (p *Proxmox) vmStatus(ctx context.Context, vmid string) (string, error) {
	d, err := p.call(ctx, "GET", fmt.Sprintf("/nodes/%s/qemu/%s/status/current", p.node, vmid))
	if err != nil {
		return "", err
	}
	s, _ := d["status"].(string)
	return s, nil
}

// IsOn = ALL managed VMs running (any stopped VM means the group is "off"
// enough that the lease should re-assert).
func (p *Proxmox) IsOn(ctx context.Context) (bool, error) {
	for _, id := range p.vmids {
		s, err := p.vmStatus(ctx, id)
		if err != nil {
			return false, err
		}
		if s != "running" {
			return false, nil
		}
	}
	return true, nil
}

func (p *Proxmox) On(ctx context.Context) error {
	for _, id := range p.vmids {
		if s, _ := p.vmStatus(ctx, id); s == "running" {
			continue
		}
		if _, err := p.call(ctx, "POST", fmt.Sprintf("/nodes/%s/qemu/%s/status/start", p.node, id)); err != nil {
			return fmt.Errorf("start vm %s: %w", id, err)
		}
	}
	return nil
}

func (p *Proxmox) Off(ctx context.Context) error {
	for _, id := range p.vmids {
		if s, _ := p.vmStatus(ctx, id); s != "running" {
			continue
		}
		// shutdown = guest-cooperative (ACPI/agent), not a hard stop.
		if _, err := p.call(ctx, "POST", fmt.Sprintf("/nodes/%s/qemu/%s/status/shutdown", p.node, id)); err != nil {
			return fmt.Errorf("shutdown vm %s: %w", id, err)
		}
	}
	return nil
}
