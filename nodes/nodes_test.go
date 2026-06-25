package nodes

import (
	"context"
	"reflect"
	"testing"
)

func TestClassify(t *testing.T) {
	ctx := context.Background()

	t.Run("partitions ready and not-ready preserving order", func(t *testing.T) {
		down := map[string]bool{"node0": true} // node0 wedged
		check := func(_ context.Context, n string) bool { return !down[n] }
		ready, notReady := classify(ctx, []string{"node0", "node1", "node2", "node3"}, check)
		if want := []string{"node1", "node2", "node3"}; !reflect.DeepEqual(ready, want) {
			t.Fatalf("ready = %v, want %v", ready, want)
		}
		if want := []string{"node0"}; !reflect.DeepEqual(notReady, want) {
			t.Fatalf("notReady = %v, want %v", notReady, want)
		}
	})

	t.Run("all ready", func(t *testing.T) {
		ready, notReady := classify(ctx, []string{"a", "b"},
			func(context.Context, string) bool { return true })
		if len(ready) != 2 || len(notReady) != 0 {
			t.Fatalf("ready=%v notReady=%v, want all ready", ready, notReady)
		}
	})

	t.Run("none ready", func(t *testing.T) {
		ready, notReady := classify(ctx, []string{"a", "b"},
			func(context.Context, string) bool { return false })
		if len(ready) != 0 || len(notReady) != 2 {
			t.Fatalf("ready=%v notReady=%v, want none ready", ready, notReady)
		}
	})
}
