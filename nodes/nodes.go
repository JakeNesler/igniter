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

// nodeReady reports whether one node currently reports Ready=True.
func nodeReady(ctx context.Context, name string) bool {
	out, err := kubectl(ctx, "get", "node", name,
		"-o", `jsonpath={.status.conditions[?(@.type=="Ready")].status}`)
	return err == nil && strings.TrimSpace(out) == "True"
}

// classify partitions names into ready/notReady using check. The check is
// injectable so the partition logic can be tested without a live cluster.
func classify(ctx context.Context, names []string, check func(context.Context, string) bool) (ready, notReady []string) {
	for _, n := range names {
		if check(ctx, n) {
			ready = append(ready, n)
		} else {
			notReady = append(notReady, n)
		}
	}
	return ready, notReady
}

// WaitReady blocks until every node reports Ready (or ctx expires). Strict
// all-or-nothing; prefer WaitReadyBestEffort for bring-up so one wedged node
// can't fail the whole fleet.
func WaitReady(ctx context.Context, names []string) error {
	for {
		if _, notReady := classify(ctx, names, nodeReady); len(notReady) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

// WaitReadyBestEffort waits until every node is Ready or ctx expires, then
// reports which nodes reached Ready and which did not. Unlike WaitReady it
// never fails outright on a partial result — the caller decides whether a
// partial bring-up is acceptable, so a single wedged node can't hold the whole
// fleet hostage (cordoned + unschedulable) until it recovers.
func WaitReadyBestEffort(ctx context.Context, names []string) (ready, notReady []string) {
	for {
		ready, notReady = classify(ctx, names, nodeReady)
		if len(notReady) == 0 {
			return ready, notReady
		}
		select {
		case <-ctx.Done():
			return ready, notReady
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
