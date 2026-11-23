package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fleetdm/fleet/v4/orbit/pkg/constant"
	"github.com/stretchr/testify/require"
)

// OPENFRAME(enroll-machine-identity): openframe/docs/enroll-machine-identity.md

type fakeServer struct {
	moves           bool
	machineIdentity bool
	pingErr         error
	moveErr         error
	movedTo         []string
}

func (s *fakeServer) IdentityCapabilities() (bool, bool, error) {
	return s.moves, s.machineIdentity, s.pingErr
}

func (s *fakeServer) MoveToIdentifier(osqueryIdentifier string) error {
	s.movedTo = append(s.movedTo, osqueryIdentifier)
	return s.moveErr
}

const (
	testUUID     = "HW-UUID-1"
	testInstance = "instance-1"
	testMachine  = "machine-1"
)

func enrolledRoot(t *testing.T, identifier string, withNodeKey bool) string {
	root := t.TempDir()
	if identifier != "" {
		require.NoError(t, os.WriteFile(filepath.Join(root, constant.OsqueryIdentifierFileName), []byte(identifier), 0o600))
	}
	if withNodeKey {
		require.NoError(t, os.WriteFile(filepath.Join(root, constant.OrbitNodeKeyFileName), []byte("node-key"), 0o600))
	}
	return root
}

func recordedIdentifier(t *testing.T, root string) string {
	b, err := os.ReadFile(filepath.Join(root, constant.OsqueryIdentifierFileName))
	require.NoError(t, err)
	return string(b)
}

var machineIdentityServer = func() *fakeServer { return &fakeServer{moves: true, machineIdentity: true} }

func TestOpenframeEnrollIdentityServerSwitchedOnMovesTheEnrolledHost(t *testing.T) {
	root := enrolledRoot(t, testUUID, true)
	server := machineIdentityServer()

	identity := openframeEnrollIdentityFor(root, "uuid", testMachine, testInstance, testUUID, server)

	require.Equal(t, specifiedEnrollIdentity(testMachine), identity)
	require.Equal(t, []string{testMachine}, server.movedTo)
	require.Equal(t, testMachine, recordedIdentifier(t, root))
}

func TestOpenframeEnrollIdentityServerSwitchedOffKeepsTheHardwareUUID(t *testing.T) {
	root := enrolledRoot(t, testUUID, true)
	server := &fakeServer{moves: true}

	identity := openframeEnrollIdentityFor(root, "uuid", testMachine, testInstance, testUUID, server)

	require.Equal(t, uuidEnrollIdentity(testUUID), identity)
	require.Empty(t, server.movedTo)
}

func TestOpenframeEnrollIdentityServerSwitchedOffMovesBackToTheHardwareUUID(t *testing.T) {
	root := enrolledRoot(t, testMachine, true)
	server := &fakeServer{moves: true}

	identity := openframeEnrollIdentityFor(root, "uuid", testMachine, testInstance, testUUID, server)

	require.Equal(t, uuidEnrollIdentity(testUUID), identity)
	require.Equal(t, []string{""}, server.movedTo)
	require.Equal(t, testUUID, recordedIdentifier(t, root))
}

func TestOpenframeEnrollIdentityServerWithoutMovesKeepsTheCurrentIdentifier(t *testing.T) {
	for name, server := range map[string]*fakeServer{
		"old server":         {},
		"server unreachable": {pingErr: errors.New("unreachable")},
	} {
		t.Run(name, func(t *testing.T) {
			root := enrolledRoot(t, testMachine, true)

			identity := openframeEnrollIdentityFor(root, "uuid", testMachine, testInstance, testUUID, server)

			require.Equal(t, specifiedEnrollIdentity(testMachine), identity)
			require.Empty(t, server.movedTo)
			require.Equal(t, testMachine, recordedIdentifier(t, root))
		})
	}
}

func TestOpenframeEnrollIdentityNewInstallOnAnOldServerFollowsTheFlag(t *testing.T) {
	root := enrolledRoot(t, "", false)

	identity := openframeEnrollIdentityFor(root, "instance", testMachine, testInstance, testUUID, &fakeServer{})

	require.Equal(t, testInstance, identity.identifier)
	require.Equal(t, "instance", identity.osqueryHostIdentifier)
}

func TestOpenframeEnrollIdentityFailedMoveKeepsTheCurrentIdentifier(t *testing.T) {
	root := enrolledRoot(t, testUUID, true)
	server := &fakeServer{moves: true, machineIdentity: true, moveErr: errors.New("enroll failed")}

	identity := openframeEnrollIdentityFor(root, "uuid", testMachine, testInstance, testUUID, server)

	require.Equal(t, uuidEnrollIdentity(testUUID), identity)
	require.Equal(t, testUUID, recordedIdentifier(t, root))
}

func TestOpenframeEnrollIdentityNewInstallEnrollsWithTheMachineIDDirectly(t *testing.T) {
	root := enrolledRoot(t, "", false)
	server := machineIdentityServer()

	identity := openframeEnrollIdentityFor(root, "uuid", testMachine, testInstance, testUUID, server)

	require.Equal(t, specifiedEnrollIdentity(testMachine), identity)
	require.Empty(t, server.movedTo)
}

func TestOpenframeEnrollIdentityReinstalledAgentWithoutNodeKeyEnrollsWithTheMachineID(t *testing.T) {
	root := enrolledRoot(t, "", false)
	server := machineIdentityServer()

	identity := openframeEnrollIdentityFor(root, "uuid", testMachine, testInstance, testUUID, server)

	require.Equal(t, testMachine, identity.identifier)
	require.Empty(t, server.movedTo)
}

func TestOpenframeEnrollIdentityUnchangedNeedsNoMove(t *testing.T) {
	root := enrolledRoot(t, testMachine, true)
	server := machineIdentityServer()

	identity := openframeEnrollIdentityFor(root, "uuid", testMachine, testInstance, testUUID, server)

	require.Equal(t, specifiedEnrollIdentity(testMachine), identity)
	require.Empty(t, server.movedTo)
}

func TestOpenframeEnrollIdentityMissingMachineIDKeepsTheCurrentIdentifier(t *testing.T) {
	root := enrolledRoot(t, testUUID, true)
	server := machineIdentityServer()

	identity := openframeEnrollIdentityFor(root, "uuid", "", testInstance, testUUID, server)

	require.Equal(t, uuidEnrollIdentity(testUUID), identity)
	require.Empty(t, server.movedTo)
}

func TestPreviousEnrollIdentity(t *testing.T) {
	for name, tc := range map[string]struct {
		recorded string
		want     openframeEnrollIdentity
	}{
		"nothing recorded is the hardware uuid": {"", uuidEnrollIdentity(testUUID)},
		"hardware uuid in any case":             {"hw-uuid-1", uuidEnrollIdentity(testUUID)},
		"current instance id":                   {testInstance, openframeEnrollIdentity{identifier: testInstance, osqueryIdentifier: testInstance, osqueryHostIdentifier: "instance"}},
		"any other identifier stays reachable":  {"old-instance", specifiedEnrollIdentity("old-instance")},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, previousEnrollIdentity(tc.recorded, testUUID, testInstance))
		})
	}
}

func TestSpecifiedEnrollIdentityOsqueryFlags(t *testing.T) {
	identity := specifiedEnrollIdentity(testMachine)

	require.Equal(t, "specified", identity.osqueryHostIdentifier)
	require.Equal(t, []string{"--specified_identifier", testMachine}, identity.osqueryFlags)
}
