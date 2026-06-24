// Package nodes wraps kubectl for node lifecycle (wait-ready, cordon, drain,
// uncordon). It shells out to kubectl rather than importing client-go to keep
// the binary lean; the in-cluster ServiceAccount kubeconfig makes kubectl work
// with zero configuration.
package nodes

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func kubectl(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "kubectl", args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("kubectl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// WaitReady blocks until every node reports Ready (or ctx expires).
func WaitReady(ctx context.Context, names []string) error {
	for {
		allReady := true
		for _, n := range names {
			out, err := kubectl(ctx, "get", "node", n,
				"-o", `jsonpath={.status.conditions[?(@.type=="Ready")].status}`)
			if err != nil || strings.TrimSpace(out) != "True" {
				allReady = false
				break
			}
		}
		if allReady {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

// Drain cordons then evicts everything evictable from the node. DaemonSets and
// emptyDir data are expected casualties of a host wind-down.
func Drain(ctx context.Context, name string, timeout time.Duration) error {
	if _, err := kubectl(ctx, "cordon", name); err != nil {
		return err
	}
	_, err := kubectl(ctx, "drain", name,
		"--ignore-daemonsets", "--delete-emptydir-data", "--force",
		"--grace-period=60", fmt.Sprintf("--timeout=%s", timeout))
	return err
}

// Uncordon marks the node schedulable again after a bring-up.
func Uncordon(ctx context.Context, name string) error {
	_, err := kubectl(ctx, "uncordon", name)
	return err
}
