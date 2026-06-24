package power

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// WOL powers on via Wake-on-LAN magic packet and off via SSH to the host
// (consumer boards have no BMC). IsOn probes the SSH port.
type WOL struct {
	mac       net.HardwareAddr
	broadcast string // ip:port for the magic packet (e.g. 10.0.0.255:9)
	sshAddr   string // host:22
	sshUser   string
	sshPass   string
	sshKeyPEM string
}

func NewWOL(mac, broadcast, sshAddr, sshUser, sshPass, sshKeyPEM string) (*WOL, error) {
	if mac == "" || sshAddr == "" {
		return nil, fmt.Errorf("WOL_MAC and SSH_ADDR are required")
	}
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return nil, fmt.Errorf("WOL_MAC: %w", err)
	}
	if !strings.Contains(sshAddr, ":") {
		sshAddr += ":22"
	}
	return &WOL{mac: hw, broadcast: broadcast, sshAddr: sshAddr, sshUser: sshUser, sshPass: sshPass, sshKeyPEM: sshKeyPEM}, nil
}

func (p *WOL) IsOn(ctx context.Context) (bool, error) {
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", p.sshAddr)
	if err != nil {
		return false, nil // unreachable = off (for our purposes)
	}
	conn.Close()
	return true, nil
}

func (p *WOL) On(ctx context.Context) error {
	payload := magicPacket(p.mac)
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", p.broadcast)
	if err != nil {
		return err
	}
	defer conn.Close()
	// WoL is fire-and-forget UDP; send a small burst, pausing between packets
	// (not after the last) and honoring cancellation.
	const sends = 3
	for i := 0; i < sends; i++ {
		if _, err := conn.Write(payload); err != nil {
			return err
		}
		if i < sends-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
	return nil
}

func magicPacket(mac net.HardwareAddr) []byte {
	payload := make([]byte, 0, 102)
	payload = append(payload, bytes.Repeat([]byte{0xff}, 6)...)
	for i := 0; i < 16; i++ {
		payload = append(payload, mac...)
	}
	return payload
}

func (p *WOL) Off(ctx context.Context) error {
	cfg := &ssh.ClientConfig{
		User:            p.sshUser,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // homelab LAN
		Timeout:         10 * time.Second,
	}
	if p.sshKeyPEM != "" {
		// SSH_KEY is either inline PEM (starts with the PEM header) or a path to a
		// key file. The header is an unambiguous discriminator, so a path that does
		// not exist surfaces a clear "no such file" error instead of a cryptic key
		// parse failure.
		keyBytes := []byte(p.sshKeyPEM)
		if !strings.HasPrefix(strings.TrimSpace(p.sshKeyPEM), "-----BEGIN") {
			b, err := os.ReadFile(p.sshKeyPEM)
			if err != nil {
				return fmt.Errorf("SSH_KEY: %w", err)
			}
			keyBytes = b
		}
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return fmt.Errorf("SSH_KEY: %w", err)
		}
		cfg.Auth = append(cfg.Auth, ssh.PublicKeys(signer))
	}
	if p.sshPass != "" {
		cfg.Auth = append(cfg.Auth, ssh.Password(p.sshPass))
	}
	client, err := ssh.Dial("tcp", p.sshAddr, cfg)
	if err != nil {
		return fmt.Errorf("ssh %s: %w", p.sshAddr, err)
	}
	defer client.Close()
	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	// Graceful: the hypervisor stops VMs per its shutdown policy.
	return sess.Run("shutdown -h now")
}
