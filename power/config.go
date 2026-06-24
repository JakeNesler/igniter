package power

import (
	"fmt"
	"os"
	"strings"
)

// FromEnv builds the configured power driver from IGNITER_POWER and the
// selected driver's environment variables.
func FromEnv() (Driver, error) {
	switch kind := envOr("IGNITER_POWER", ""); kind {
	case "ipmi":
		return NewIPMI(
			envOr("IPMI_ADDR", ""), envOr("IPMI_USER", "root"), envOr("IPMI_PASSWORD", ""))
	case "wol":
		return NewWOL(
			envOr("WOL_MAC", ""), envOr("WOL_BROADCAST", "255.255.255.255:9"),
			envOr("SSH_ADDR", ""), envOr("SSH_USER", "root"), envOr("SSH_PASSWORD", ""), envOr("SSH_KEY", ""))
	case "proxmox":
		return NewProxmox(
			envOr("PVE_URL", ""), envOr("PVE_TOKEN_ID", ""), envOr("PVE_TOKEN_SECRET", ""),
			envOr("PVE_NODE", ""), strings.Split(envOr("PVE_VMIDS", ""), ","))
	default:
		return nil, fmt.Errorf("IGNITER_POWER must be ipmi|wol|proxmox (got %q)", kind)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
