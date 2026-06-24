// igniter — the homelab host/VM power controller, designed to be SCALED BY KEDA.
//
// One igniter Deployment per host (or Proxmox VM group). The replica count IS
// the power intent:
//
//	scale 0 -> 1   the pod powers the host ON (iDRAC IPMI / Wake-on-LAN /
//	               Proxmox API), waits for its k8s nodes to go Ready, uncordons
//	               them, then idles as a "this host is wanted" lease.
//	scale 1 -> 0   SIGTERM: the pod cordons + drains the host's nodes, then
//	               gracefully powers the host OFF, and exits.
//
// KEDA triggers (cron window for the daily floor, prometheus for demand-wake)
// compose upstream; igniter itself is deliberately dumb: ensure-on while
// running, drain-and-off when told to stop. Pin it to a node that never sleeps.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jakenesler/igniter/nodes"
	"github.com/jakenesler/igniter/power"
)

type config struct {
	HostName     string        // label for logs
	Nodes        []string      // k8s node names this host carries
	DrainTimeout time.Duration // per-node drain budget
	BootTimeout  time.Duration // power-on -> all nodes Ready budget
	Power        power.Driver
}

func loadConfig() (*config, error) {
	c := &config{
		HostName:     envOr("IGNITER_HOST", ""),
		DrainTimeout: envDur("IGNITER_DRAIN_TIMEOUT", 5*time.Minute),
		BootTimeout:  envDur("IGNITER_BOOT_TIMEOUT", 12*time.Minute),
	}
	if c.HostName == "" {
		return nil, fmt.Errorf("IGNITER_HOST is required")
	}
	if n := envOr("IGNITER_NODES", ""); n != "" {
		c.Nodes = strings.Split(n, ",")
	}

	var err error
	c.Power, err = power.FromEnv()
	return c, err
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	c, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	log.SetPrefix("[" + c.HostName + "] ")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Phase 1: ensure the host is ON and its nodes are serving.
	if err := ensureUp(ctx, c); err != nil {
		// Fail the pod: the Deployment restarts us; KEDA keeps wanting 1.
		log.Fatalf("bring-up failed: %v", err)
	}
	log.Printf("host up; holding lease (SIGTERM + scaled-to-0 = drain + power off)")

	// Phase 2: hold the lease; re-assert power if the host disappears (e.g.
	// someone hits the physical button) while we are still wanted.
	tick := time.NewTicker(2 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			// Phase 3: SIGTERM. CRITICAL distinction: a SIGTERM means EITHER the
			// host is genuinely no longer wanted (KEDA scaled the Deployment to
			// 0) OR this is just a pod rolling-replacement (image update,
			// eviction, KEDA flap) with the Deployment still wanting 1 — in which
			// case a replacement pod is already coming and powering the host off
			// would be catastrophic. Only wind down when the Deployment desires 0.
			if wanted := deploymentWantsReplica(c); wanted {
				log.Printf("SIGTERM but Deployment still wants >=1 replica — pod replacement, NOT a scale-down; exiting WITHOUT touching power")
				return
			}
			// Fresh context: the signal context is already cancelled.
			down, cancel := context.WithTimeout(context.Background(), c.DrainTimeout+5*time.Minute)
			defer cancel()
			if err := windDown(down, c); err != nil {
				log.Printf("wind-down error: %v", err)
				os.Exit(1)
			}
			log.Printf("host powered down cleanly")
			return
		case <-tick.C:
			on, err := c.Power.IsOn(ctx)
			if err != nil {
				log.Printf("power status check failed: %v", err)
				continue
			}
			if !on {
				log.Printf("host found OFF while lease held — re-asserting power on")
				if err := ensureUp(ctx, c); err != nil {
					log.Printf("re-assert failed: %v", err)
				}
			}
		}
	}
}

func ensureUp(ctx context.Context, c *config) error {
	on, err := c.Power.IsOn(ctx)
	if err != nil {
		return fmt.Errorf("power status: %w", err)
	}
	if !on {
		log.Printf("powering on")
		if err := c.Power.On(ctx); err != nil {
			return fmt.Errorf("power on: %w", err)
		}
	}
	if len(c.Nodes) == 0 {
		return nil
	}
	log.Printf("waiting for nodes %v (budget %s)", c.Nodes, c.BootTimeout)
	wctx, cancel := context.WithTimeout(ctx, c.BootTimeout)
	defer cancel()
	if err := nodes.WaitReady(wctx, c.Nodes); err != nil {
		return fmt.Errorf("nodes not ready: %w", err)
	}
	for _, n := range c.Nodes {
		if err := nodes.Uncordon(ctx, n); err != nil {
			log.Printf("uncordon %s: %v", n, err)
		}
	}
	log.Printf("nodes ready + uncordoned")
	return nil
}

func windDown(ctx context.Context, c *config) error {
	for _, n := range c.Nodes {
		log.Printf("cordon+drain %s", n)
		if err := nodes.Drain(ctx, n, c.DrainTimeout); err != nil {
			// Drain best-effort: a stuck PDB must not leave the host on forever;
			// the nightly window is the operator's explicit intent.
			log.Printf("drain %s: %v (continuing)", n, err)
		}
	}
	log.Printf("powering off (graceful)")
	return c.Power.Off(ctx)
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envDur(k string, d time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if p, err := time.ParseDuration(v); err == nil {
			return p
		}
	}
	return d
}

// deploymentWantsReplica reports whether this pod's controlling Deployment still
// desires >=1 replica. On SIGTERM that means a replacement pod is incoming (a
// rolling restart / eviction), so the host must NOT be powered off. When the
// Deployment desires 0 (KEDA scaled it down), this returns false and the host
// winds down. Fail-safe: on any error it returns TRUE (assume wanted) so an API
// hiccup can never trigger a spurious power-off.
func deploymentWantsReplica(c *config) bool {
	name := os.Getenv("IGNITER_DEPLOYMENT")
	ns := os.Getenv("IGNITER_NAMESPACE")
	if name == "" || ns == "" {
		log.Printf("IGNITER_DEPLOYMENT/IGNITER_NAMESPACE unset — cannot confirm scale-down intent; assuming WANTED (no power-off)")
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "kubectl", "get", "deployment", name, "-n", ns,
		"-o", "jsonpath={.spec.replicas}").Output()
	if err != nil {
		log.Printf("could not read Deployment %s/%s replicas (%v) — assuming WANTED (no power-off)", ns, name, err)
		return true
	}
	desired := strings.TrimSpace(string(out))
	log.Printf("Deployment %s/%s desired replicas = %q", ns, name, desired)
	return desired != "0"
}
