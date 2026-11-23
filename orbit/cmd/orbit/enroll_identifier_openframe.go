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

func flagEnrollIdentity(mode, instanceID, hardwareUUID string, previous openframeEnrollIdentity) openframeEnrollIdentity {
	if mode != "instance" {
		return uuidEnrollIdentity(hardwareUUID)
	}
	if instanceID == "" {
		return previous
	}
	return openframeEnrollIdentity{identifier: instanceID, osqueryIdentifier: instanceID, osqueryHostIdentifier: "instance"}
}

func wantedEnrollIdentity(machineIdentity bool, mode, machineID, instanceID, hardwareUUID string, previous openframeEnrollIdentity) openframeEnrollIdentity {
	if !machineIdentity {
		return flagEnrollIdentity(mode, instanceID, hardwareUUID, previous)
	}
	if machineID == "" {
		log.Warn().Msg("openframe machine id is not available, keeping the current enrollment identifier")
		return previous
	}
	return specifiedEnrollIdentity(machineID)
}

type identityServer interface {
	IdentityCapabilities() (moves bool, machineIdentity bool, err error)
	MoveToIdentifier(osqueryIdentifier string) error
}

func chooseEnrollIdentity(mode, machineID, instanceID, hardwareUUID string, previous openframeEnrollIdentity, enrolled bool, server identityServer) openframeEnrollIdentity {
	moves, machineIdentity, err := server.IdentityCapabilities()
	if err != nil || !moves {
		log.Info().Err(err).Bool("moves", moves).Msg("fleet server does not move hosts between identifiers, keeping the current one")
		if enrolled {
			return previous
		}
		return flagEnrollIdentity(mode, instanceID, hardwareUUID, previous)
	}
	wanted := wantedEnrollIdentity(machineIdentity, mode, machineID, instanceID, hardwareUUID, previous)
	if wanted.identifier == previous.identifier || !enrolled {
		return wanted
	}
	if err := server.MoveToIdentifier(wanted.osqueryIdentifier); err != nil {
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

func openframeEnrollIdentityFor(rootDir, mode, machineID, instanceID, hardwareUUID string, server identityServer) openframeEnrollIdentity {
	previous := previousEnrollIdentity(readEnrolledIdentifier(rootDir), hardwareUUID, instanceID)
	chosen := chooseEnrollIdentity(mode, machineID, instanceID, hardwareUUID, previous, hasOrbitNodeKey(rootDir), server)
	if err := writeOsqueryIdentifierFile(rootDir, chosen.identifier); err != nil {
		log.Error().Err(err).Msg("write osquery identifier file")
	}
	return chosen
}
