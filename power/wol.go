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
// (consumer boards have no BMC). IsOn probes the SSH port. Off runs a graceful
// `shutdown`, VERIFIES the host goes unreachable, and — since there is no
// out-of-band power for a WoL host — makes a best-effort forceful power-off over
// SSH before giving up with an error (so the caller never reports a shutdown
// that did not happen).
type WOL struct {
	mac       net.HardwareAddr
	broadcast string // ip:port for the magic packet (e.g. 10.0.0.255:9)
	sshAddr   string // host:22
	sshUser   string
	sshPass   string
	sshKeyPEM string
	softGrace time.Duration // wait for `shutdown` to take the host offline
	hardGrace time.Duration // wait for the forced power-off to take effect
	poll      time.Duration // reachability poll interval
}

func NewWOL(mac, broadcast, sshAddr, sshUser, sshPass, sshKeyPEM string, softGrace, hardGrace, poll time.Duration) (*WOL, error) {
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
	if softGrace <= 0 {
		softGrace = defaultSoftGrace
	}
	if hardGrace <= 0 {
		hardGrace = defaultHardGrace
	}
	if poll <= 0 {
		poll = defaultPoll
	}
	return &WOL{
		mac: hw, broadcast: broadcast, sshAddr: sshAddr,
		sshUser: sshUser, sshPass: sshPass, sshKeyPEM: sshKeyPEM,
		softGrace: softGrace, hardGrace: hardGrace, poll: poll,
	}, nil
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

// Off shuts the host down and CONFIRMS it: `shutdown -h now` -> wait softGrace
// for the SSH port to go dark -> if still up, a best-effort forced power-off
// (SysRq, then `poweroff -f`) -> wait hardGrace -> error if still reachable.
func (p *WOL) Off(ctx context.Context) error {
	if err := p.runSSH(ctx, "shutdown -h now"); err != nil {
		return fmt.Errorf("ssh %s: %w", p.sshAddr, err)
	}
	if p.waitOff(ctx, p.softGrace) {
		return nil
	}

	// No BMC to fall back on — try to force it from inside over SSH. Best-effort:
	// the host may already be too far down to accept the session, which is fine
	// (the verify below is what actually decides success/failure).
	_ = p.runSSH(ctx, "sh -c 'echo o > /proc/sysrq-trigger 2>/dev/null || poweroff -f'")
	if p.waitOff(ctx, p.hardGrace) {
		return nil
	}
	return fmt.Errorf("host %s still reachable after shutdown (%s) and forced power-off (%s)",
		p.sshAddr, p.softGrace, p.hardGrace)
}

// waitOff polls SSH reachability until the host is unreachable (off), the grace
// elapses, or ctx is cancelled.
func (p *WOL) waitOff(ctx context.Context, grace time.Duration) bool {
	wctx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()
	t := time.NewTicker(p.poll)
	defer t.Stop()
	for {
		if on, err := p.IsOn(wctx); err == nil && !on {
			return true
		}
		select {
		case <-wctx.Done():
			return false
		case <-t.C:
		}
	}
}

func (p *WOL) sshConfig() (*ssh.ClientConfig, error) {
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
				return nil, fmt.Errorf("SSH_KEY: %w", err)
			}
			keyBytes = b
		}
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("SSH_KEY: %w", err)
		}
		cfg.Auth = append(cfg.Auth, ssh.PublicKeys(signer))
	}
	if p.sshPass != "" {
		cfg.Auth = append(cfg.Auth, ssh.Password(p.sshPass))
	}
	return cfg, nil
}

func (p *WOL) runSSH(ctx context.Context, cmd string) error {
	cfg, err := p.sshConfig()
	if err != nil {
		return err
	}
	d := net.Dialer{Timeout: cfg.Timeout}
	conn, err := d.DialContext(ctx, "tcp", p.sshAddr)
	if err != nil {
		return err
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, p.sshAddr, cfg)
	if err != nil {
		conn.Close()
		return err
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()
	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	return sess.Run(cmd)
}
