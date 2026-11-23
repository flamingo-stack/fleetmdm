// OPENFRAME(enroll-machine-identity): the identifier orbit and osquery enroll with in OpenFrame mode — openframe/docs/enroll-machine-identity.md
package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fleetdm/fleet/v4/orbit/pkg/constant"
	"github.com/rs/zerolog/log"
)

const openframeHostIdentifier = "openframe"

type openframeEnrollIdentity struct {
	identifier            string
	osqueryIdentifier     string
	osqueryHostIdentifier string
	osqueryFlags          []string
}

func uuidEnrollIdentity(hardwareUUID string) openframeEnrollIdentity {
	return openframeEnrollIdentity{identifier: hardwareUUID, osqueryHostIdentifier: "uuid"}
}

func specifiedEnrollIdentity(identifier string) openframeEnrollIdentity {
	return openframeEnrollIdentity{
		identifier:            identifier,
		osqueryIdentifier:     identifier,
		osqueryHostIdentifier: "specified",
		osqueryFlags:          []string{"--specified_identifier", identifier},
	}
}

func previousEnrollIdentity(previous, hardwareUUID, instanceID string) openframeEnrollIdentity {
	switch {
	case previous == "" || strings.EqualFold(previous, hardwareUUID):
		return uuidEnrollIdentity(hardwareUUID)
	case previous == instanceID:
		return openframeEnrollIdentity{identifier: instanceID, osqueryIdentifier: instanceID, osqueryHostIdentifier: "instance"}
	default:
		return specifiedEnrollIdentity(previous)
	}
}

func wantedEnrollIdentity(mode, machineID, instanceID, hardwareUUID string, previous openframeEnrollIdentity) openframeEnrollIdentity {
	switch mode {
	case openframeHostIdentifier:
		if machineID == "" {
			log.Warn().Msg("openframe machine id is not available, keeping the current enrollment identifier")
			return previous
		}
		return specifiedEnrollIdentity(machineID)
	case "instance":
		if instanceID == "" {
			return previous
		}
		return openframeEnrollIdentity{identifier: instanceID, osqueryIdentifier: instanceID, osqueryHostIdentifier: "instance"}
	default:
		return uuidEnrollIdentity(hardwareUUID)
	}
}

type identifierMover interface {
	SupportsIdentifierMove() (bool, error)
	MoveToIdentifier(osqueryIdentifier string) error
}

func chooseEnrollIdentity(wanted, previous openframeEnrollIdentity, enrolled bool, mover identifierMover) openframeEnrollIdentity {
	if wanted.identifier == previous.identifier {
		return wanted
	}
	supported, err := mover.SupportsIdentifierMove()
	if err != nil || !supported {
		log.Info().Err(err).Bool("supported", supported).Msg("fleet server cannot move the host to a new identifier yet, keeping the current one")
		return previous
	}
	if !enrolled {
		return wanted
	}
	if err := mover.MoveToIdentifier(wanted.osqueryIdentifier); err != nil {
		log.Warn().Err(err).Msg("move host to the new enrollment identifier failed, keeping the current one")
		return previous
	}
	log.Info().Str("identifier", wanted.identifier).Msg("host moved to the new enrollment identifier")
	return wanted
}

func readEnrolledIdentifier(rootDir string) string {
	b, err := os.ReadFile(filepath.Join(rootDir, constant.OsqueryIdentifierFileName))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Warn().Err(err).Msg("read osquery identifier file")
		}
		return ""
	}
	return strings.TrimSpace(string(b))
}

func hasOrbitNodeKey(rootDir string) bool {
	b, err := os.ReadFile(filepath.Join(rootDir, constant.OrbitNodeKeyFileName))
	return err == nil && len(bytes.TrimSpace(b)) > 0
}

func openframeEnrollIdentityFor(rootDir, mode, machineID, instanceID, hardwareUUID string, mover identifierMover) openframeEnrollIdentity {
	previous := previousEnrollIdentity(readEnrolledIdentifier(rootDir), hardwareUUID, instanceID)
	wanted := wantedEnrollIdentity(mode, machineID, instanceID, hardwareUUID, previous)
	chosen := chooseEnrollIdentity(wanted, previous, hasOrbitNodeKey(rootDir), mover)
	if err := writeOsqueryIdentifierFile(rootDir, chosen.identifier); err != nil {
		log.Error().Err(err).Msg("write osquery identifier file")
	}
	return chosen
}
