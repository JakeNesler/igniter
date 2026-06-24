package power

import (
	"bytes"
	"net"
	"testing"
)

func TestMagicPacket(t *testing.T) {
	mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	packet := magicPacket(mac)

	if got, want := len(packet), 102; got != want {
		t.Fatalf("len(magicPacket) = %d, want %d", got, want)
	}
	if got, want := packet[:6], bytes.Repeat([]byte{0xff}, 6); !bytes.Equal(got, want) {
		t.Fatalf("header = %x, want %x", got, want)
	}
	for i := 0; i < 16; i++ {
		start := 6 + i*len(mac)
		end := start + len(mac)
		if got := packet[start:end]; !bytes.Equal(got, mac) {
			t.Fatalf("mac repeat %d = %x, want %x", i, got, mac)
		}
	}
}

func TestNewWOLValidationAndNormalization(t *testing.T) {
	tests := []struct {
		name    string
		mac     string
		sshAddr string
		wantErr bool
		wantSSH string
	}{
		{
			name:    "missing mac",
			mac:     "",
			sshAddr: "host:22",
			wantErr: true,
		},
		{
			name:    "missing ssh addr",
			mac:     "00:11:22:33:44:55",
			sshAddr: "",
			wantErr: true,
		},
		{
			name:    "invalid mac",
			mac:     "not-a-mac",
			sshAddr: "host:22",
			wantErr: true,
		},
		{
			name:    "appends default ssh port",
			mac:     "00:11:22:33:44:55",
			sshAddr: "host",
			wantSSH: "host:22",
		},
		{
			name:    "keeps explicit ssh port",
			mac:     "00:11:22:33:44:55",
			sshAddr: "host:2222",
			wantSSH: "host:2222",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wol, err := NewWOL(tt.mac, "255.255.255.255:9", tt.sshAddr, "root", "", "")
			if tt.wantErr {
				if err == nil {
					t.Fatal("NewWOL returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewWOL returned error: %v", err)
			}
			if wol.sshAddr != tt.wantSSH {
				t.Fatalf("sshAddr = %q, want %q", wol.sshAddr, tt.wantSSH)
			}
		})
	}
}
