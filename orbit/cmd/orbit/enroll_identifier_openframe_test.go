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

type fakeMover struct {
	supported  bool
	pingErr    error
	moveErr    error
	movedTo    []string
	pingCalled bool
}

func (m *fakeMover) SupportsIdentifierMove() (bool, error) {
	m.pingCalled = true
	return m.supported, m.pingErr
}

func (m *fakeMover) MoveToIdentifier(osqueryIdentifier string) error {
	m.movedTo = append(m.movedTo, osqueryIdentifier)
	return m.moveErr
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

func TestOpenframeEnrollIdentityMovesEnrolledHostToMachineID(t *testing.T) {
	root := enrolledRoot(t, testUUID, true)
	mover := &fakeMover{supported: true}

	identity := openframeEnrollIdentityFor(root, openframeHostIdentifier, testMachine, testInstance, testUUID, mover)

	require.Equal(t, specifiedEnrollIdentity(testMachine), identity)
	require.Equal(t, []string{testMachine}, mover.movedTo)
	require.Equal(t, testMachine, recordedIdentifier(t, root))
}

func TestOpenframeEnrollIdentityKeepsCurrentWhenServerCannotMove(t *testing.T) {
	for name, mover := range map[string]*fakeMover{
		"old server":       {supported: false},
		"server not found": {pingErr: errors.New("unreachable")},
	} {
		t.Run(name, func(t *testing.T) {
			root := enrolledRoot(t, testUUID, true)

			identity := openframeEnrollIdentityFor(root, openframeHostIdentifier, testMachine, testInstance, testUUID, mover)

			require.Equal(t, uuidEnrollIdentity(testUUID), identity)
			require.Empty(t, mover.movedTo)
			require.Equal(t, testUUID, recordedIdentifier(t, root))
		})
	}
}

func TestOpenframeEnrollIdentityKeepsCurrentWhenMoveFails(t *testing.T) {
	root := enrolledRoot(t, testUUID, true)
	mover := &fakeMover{supported: true, moveErr: errors.New("enroll failed")}

	identity := openframeEnrollIdentityFor(root, openframeHostIdentifier, testMachine, testInstance, testUUID, mover)

	require.Equal(t, uuidEnrollIdentity(testUUID), identity)
	require.Equal(t, testUUID, recordedIdentifier(t, root))
}

func TestOpenframeEnrollIdentityNewInstallEnrollsWithMachineIDDirectly(t *testing.T) {
	root := enrolledRoot(t, "", false)
	mover := &fakeMover{supported: true}

	identity := openframeEnrollIdentityFor(root, openframeHostIdentifier, testMachine, testInstance, testUUID, mover)

	require.Equal(t, specifiedEnrollIdentity(testMachine), identity)
	require.Empty(t, mover.movedTo)
	require.Equal(t, testMachine, recordedIdentifier(t, root))
}

func TestOpenframeEnrollIdentityUnchangedNeedsNoServer(t *testing.T) {
	root := enrolledRoot(t, testMachine, true)
	mover := &fakeMover{}

	identity := openframeEnrollIdentityFor(root, openframeHostIdentifier, testMachine, testInstance, testUUID, mover)

	require.Equal(t, specifiedEnrollIdentity(testMachine), identity)
	require.False(t, mover.pingCalled)
}

func TestOpenframeEnrollIdentityMissingMachineIDKeepsCurrent(t *testing.T) {
	root := enrolledRoot(t, testMachine, true)
	mover := &fakeMover{supported: true}

	identity := openframeEnrollIdentityFor(root, openframeHostIdentifier, "", testInstance, testUUID, mover)

	require.Equal(t, specifiedEnrollIdentity(testMachine), identity)
	require.False(t, mover.pingCalled)
}

func TestOpenframeEnrollIdentityRollbackMovesBackToUUID(t *testing.T) {
	root := enrolledRoot(t, testMachine, true)
	mover := &fakeMover{supported: true}

	identity := openframeEnrollIdentityFor(root, "uuid", testMachine, testInstance, testUUID, mover)

	require.Equal(t, uuidEnrollIdentity(testUUID), identity)
	require.Equal(t, []string{""}, mover.movedTo)
	require.Equal(t, testUUID, recordedIdentifier(t, root))
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
