package power

import "testing"

func TestNewProxmoxCleansVMIDs(t *testing.T) {
	p, err := NewProxmox(
		"https://pve.example:8006/",
		"user@pam!igniter",
		"secret",
		"node1",
		[]string{" 100 ", "", " \t", "101"},
		0, 0, 0,
	)
	if err != nil {
		t.Fatalf("NewProxmox returned error: %v", err)
	}

	want := []string{"100", "101"}
	if len(p.vmids) != len(want) {
		t.Fatalf("vmids = %#v, want %#v", p.vmids, want)
	}
	for i := range want {
		if p.vmids[i] != want[i] {
			t.Fatalf("vmids = %#v, want %#v", p.vmids, want)
		}
	}
}

func TestNewProxmoxValidation(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		tokenID     string
		tokenSecret string
		node        string
		vmids       []string
	}{
		{
			name:        "missing url",
			tokenID:     "user@pam!igniter",
			tokenSecret: "secret",
			node:        "node1",
			vmids:       []string{"100"},
		},
		{
			name:        "missing token id",
			url:         "https://pve.example:8006",
			tokenSecret: "secret",
			node:        "node1",
			vmids:       []string{"100"},
		},
		{
			name:    "missing token secret",
			url:     "https://pve.example:8006",
			tokenID: "user@pam!igniter",
			node:    "node1",
			vmids:   []string{"100"},
		},
		{
			name:        "missing node",
			url:         "https://pve.example:8006",
			tokenID:     "user@pam!igniter",
			tokenSecret: "secret",
			vmids:       []string{"100"},
		},
		{
			name:        "empty vmids",
			url:         "https://pve.example:8006",
			tokenID:     "user@pam!igniter",
			tokenSecret: "secret",
			node:        "node1",
			vmids:       nil,
		},
		{
			name:        "all empty vmids",
			url:         "https://pve.example:8006",
			tokenID:     "user@pam!igniter",
			tokenSecret: "secret",
			node:        "node1",
			vmids:       []string{"", " ", "\t"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewProxmox(tt.url, tt.tokenID, tt.tokenSecret, tt.node, tt.vmids, 0, 0, 0)
			if err == nil {
				t.Fatal("NewProxmox returned nil error")
			}
		})
	}
}
