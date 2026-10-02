package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/fleetdm/fleet/v4/orbit/pkg/update"
)

func main() {
	var (
		discoveryURL = flag.String("discovery-url", "", "The Windows MDM discovery URL")
		hostUUID     = flag.String("host-uuid", "", "The Host UUID")
		unenroll     = flag.Bool("unenroll", false, "Unenroll from MDM instead of enrolling")
	)
	flag.Parse()

	if *unenroll {
		if err := update.RunWindowsMDMUnenrollment(update.WindowsMDMEnrollmentArgs{}); err != nil {
			fmt.Printf("unenrollment failed: %v\n", fmt.Errorf("windows mdm unenrollment: %w", err))
			os.Exit(1)
		}
		return
	}

	if err := update.RunWindowsMDMEnrollment(update.WindowsMDMEnrollmentArgs{
		DiscoveryURL: *discoveryURL,
		HostUUID:     *hostUUID,
	}); err != nil {
		fmt.Printf("enrollment failed: %v\n", fmt.Errorf("windows mdm enrollment: %w", err))
		os.Exit(1)
	}
}
