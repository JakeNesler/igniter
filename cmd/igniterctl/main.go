// igniterctl — manual power ops with the same drivers the controller uses.
// Usage: IGNITER_POWER=ipmi IPMI_ADDR=... igniterctl status|on|off
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jakenesler/igniter/power"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: igniterctl status|on|off")
	}
	d, err := power.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch os.Args[1] {
	case "status":
		on, err := d.IsOn(ctx)
		if err != nil {
			log.Fatal(err)
		}
		if on {
			fmt.Println("ON")
		} else {
			fmt.Println("OFF")
		}
	case "on":
		if err := d.On(ctx); err != nil {
			log.Fatal(err)
		}
		fmt.Println("power on sent")
	case "off":
		if err := d.Off(ctx); err != nil {
			log.Fatal(err)
		}
		fmt.Println("graceful off sent")
	default:
		log.Fatal("usage: igniterctl status|on|off")
	}
}
