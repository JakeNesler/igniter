package power

import (
	"testing"
	"time"
)

func TestNewIPMIValidation(t *testing.T) {
	tests := []struct {
		name, addr, pass string
	}{
		{name: "missing addr", pass: "secret"},
		{name: "missing pass", addr: "10.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewIPMI(tt.addr, "root", tt.pass, 0, 0, 0); err == nil {
				t.Fatal("NewIPMI returned nil error")
			}
		})
	}
}

func TestNewIPMIAppliesDefaults(t *testing.T) {
	p, err := NewIPMI("10.0.0.1", "root", "secret", 0, 0, 0)
	if err != nil {
		t.Fatalf("NewIPMI returned error: %v", err)
	}
	if p.softGrace != defaultSoftGrace || p.hardGrace != defaultHardGrace || p.poll != defaultPoll {
		t.Fatalf("defaults not applied: got soft=%v hard=%v poll=%v", p.softGrace, p.hardGrace, p.poll)
	}
}

func TestNewIPMIKeepsExplicitTimings(t *testing.T) {
	soft, hard, poll := 90*time.Second, 30*time.Second, 5*time.Second
	p, err := NewIPMI("10.0.0.1", "root", "secret", soft, hard, poll)
	if err != nil {
		t.Fatalf("NewIPMI returned error: %v", err)
	}
	if p.softGrace != soft || p.hardGrace != hard || p.poll != poll {
		t.Fatalf("explicit timings overwritten: got soft=%v hard=%v poll=%v", p.softGrace, p.hardGrace, p.poll)
	}
}
