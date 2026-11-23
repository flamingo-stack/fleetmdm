// OPENFRAME(enroll-machine-identity): orbit changes the identifier it enrolls with and keeps its host — openframe/docs/enroll-machine-identity.md
package client

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/fleetdm/fleet/v4/server/fleet"
)

func (oc *OrbitClient) IdentityCapabilities() (moves bool, machineIdentity bool, err error) {
	if err := oc.Ping(); err != nil {
		return false, false, fmt.Errorf("ping fleet server: %w", err)
	}
	capabilities := oc.GetServerCapabilities()
	return capabilities.Has(fleet.CapabilityOpenframeIdentifierMove), capabilities.Has(fleet.CapabilityOpenframeMachineIdentity), nil
}

func (oc *OrbitClient) SetOsqueryIdentifier(osqueryIdentifier string) {
	enrollLock.Lock()
	defer enrollLock.Unlock()
	oc.hostInfo.OsqueryIdentifier = osqueryIdentifier
}

func (oc *OrbitClient) MoveToIdentifier(osqueryIdentifier string) error {
	enrollLock.Lock()
	defer enrollLock.Unlock()

	nodeKey, err := os.ReadFile(oc.nodeKeyFilePath)
	if err != nil {
		return fmt.Errorf("read orbit node key file: %w", err)
	}
	nodeKey = bytes.TrimSpace(nodeKey)
	if len(nodeKey) == 0 {
		return errors.New("orbit node key file is empty")
	}

	previousIdentifier := oc.hostInfo.OsqueryIdentifier
	oc.hostInfo.OsqueryIdentifier = osqueryIdentifier
	oc.hostInfo.PreviousOrbitNodeKey = string(nodeKey)
	defer func() { oc.hostInfo.PreviousOrbitNodeKey = "" }()

	if _, err := oc.enrollAndWriteNodeKeyFile(); err != nil {
		oc.hostInfo.OsqueryIdentifier = previousIdentifier
		return err
	}
	oc.clearReenrollState()
	return nil
}
