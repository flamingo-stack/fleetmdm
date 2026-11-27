// OPENFRAME(windows-mdm-tenant)
package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mdm"
	microsoft_mdm "github.com/fleetdm/fleet/v4/server/mdm/microsoft"
	"github.com/fleetdm/fleet/v4/server/mdm/microsoft/syncml"
	"github.com/fleetdm/fleet/v4/server/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
)

const (
	openframeTestTeamA uint = 7
	openframeTestTeamB uint = 8
)

var openframeTestWSTEPPairs = sync.OnceValue(func() map[uint][2][]byte {
	pairs := map[uint][2][]byte{}
	for _, teamID := range []uint{openframeTestTeamA, openframeTestTeamB} {
		certPEM, keyPEM, err := microsoft_mdm.OpenframeNewWSTEPPair()
		if err != nil {
			panic(err)
		}
		pairs[teamID] = [2][]byte{certPEM, keyPEM}
	}
	return pairs
})

func openframeTenantURL(teamID uint) string {
	return fmt.Sprintf("https://tenant-%d.example.com/mdm", teamID)
}

func openframeWindowsTenantStore() *mock.DataStore {
	ds := new(mock.DataStore)
	ds.AppConfigFunc = func(ctx context.Context) (*fleet.AppConfig, error) {
		cfg := &fleet.AppConfig{}
		cfg.ServerSettings.ServerURL = "http://fleet-service.internal:8080"
		if teamID, ok := fleet.OpenframeTeamID(ctx); ok {
			cfg.MDM.WindowsServerURL = openframeTenantURL(teamID) + "/"
		}
		return cfg, nil
	}
	ds.OpenframeLoadWindowsWSTEPFunc = func(_ context.Context, teamID uint, _ func() ([]byte, []byte, error)) ([]byte, []byte, error) {
		pair := openframeTestWSTEPPairs()[teamID]
		return pair[0], pair[1], nil
	}
	ds.OpenframeGetWindowsWSTEPFunc = func(_ context.Context, teamID uint) ([]byte, []byte, error) {
		pair, ok := openframeTestWSTEPPairs()[teamID]
		if !ok {
			return nil, nil, newNotFoundError()
		}
		return pair[0], pair[1], nil
	}
	ds.WSTEPNewSerialFunc = func(context.Context) (*big.Int, error) { return big.NewInt(42), nil }
	ds.WSTEPStoreCertificateFunc = func(context.Context, string, *x509.Certificate) error { return nil }
	return ds
}

func TestOpenframeWindowsMDMServerURL(t *testing.T) {
	cfg := &fleet.AppConfig{}
	cfg.ServerSettings.ServerURL = "http://fleet-service.internal:8080"

	got, err := openframeWindowsMDMServerURL(cfg, false)
	require.NoError(t, err)
	require.Equal(t, "http://fleet-service.internal:8080", got, "outside shared mode the server URL is used, as upstream")

	_, err = openframeWindowsMDMServerURL(cfg, true)
	require.ErrorIs(t, err, errOpenframeWindowsMDMURLNotSet, "shared mode never falls back to the server URL")

	cfg.MDM.WindowsServerURL = "https://Acme.Example.com/mdm/"
	got, err = openframeWindowsMDMServerURL(cfg, true)
	require.NoError(t, err)
	require.Equal(t, "https://acme.example.com/mdm", got)
}

func TestOpenframeWindowsMDMBaseURLPerTenant(t *testing.T) {
	ds := openframeWindowsTenantStore()
	svc := &Service{ds: ds}

	_, err := svc.openframeWindowsMDMBaseURL(t.Context(), true)
	require.Error(t, err, "a request without a tenant gets no URL")

	got, err := svc.openframeWindowsMDMBaseURL(fleet.NewOpenframeTeamContext(t.Context(), openframeTestTeamA), true)
	require.NoError(t, err)
	require.Equal(t, openframeTenantURL(openframeTestTeamA), got, "a pinned request (orbit, discovery) uses its tenant")

	ctx := openframeWithWindowsMDMTeam(t.Context(), openframeTestTeamB)
	got, err = svc.openframeWindowsMDMBaseURL(ctx, true)
	require.NoError(t, err)
	require.Equal(t, openframeTenantURL(openframeTestTeamB), got, "an enroll or management request uses its device's tenant")
	_, pinned := fleet.OpenframeTeamID(ctx)
	require.False(t, pinned, "naming the device's tenant does not pin the request")

	discoURL, err := microsoft_mdm.ResolveWindowsMDMDiscovery(got)
	require.NoError(t, err)
	require.Equal(t, openframeTenantURL(openframeTestTeamB)+"/api/mdm/microsoft/discovery", discoURL)
}

func TestOpenframeWindowsMDMDeviceTenant(t *testing.T) {
	ds := openframeWindowsTenantStore()
	teamA, teamB := openframeTestTeamA, openframeTestTeamB
	ds.OpenframeMDMWindowsEnrollmentTeamIDFunc = func(_ context.Context, enrollmentID uint) (*uint, error) {
		switch enrollmentID {
		case 1:
			return &teamA, nil
		case 2:
			return nil, nil
		}
		return nil, newNotFoundError()
	}
	ds.LoadHostByOrbitNodeKeyFunc = func(_ context.Context, nodeKey string) (*fleet.Host, error) {
		if nodeKey == "node-b" {
			return &fleet.Host{UUID: "uuid-b", TeamID: &teamB}, nil
		}
		return nil, newNotFoundError()
	}
	svc := &Service{ds: ds}

	ctx, err := svc.openframeWindowsMDMDeviceContext(t.Context(), 1, true)
	require.NoError(t, err)
	got, ok := openframeWindowsMDMTeam(ctx)
	require.True(t, ok)
	require.Equal(t, teamA, got, "the management session uses the tenant recorded on the enrollment")

	ctx, err = svc.openframeWindowsMDMDeviceContext(t.Context(), 2, true)
	require.ErrorContains(t, err, "must re-enroll", "an enrollment without a recorded tenant is refused")
	_, ok = openframeWindowsMDMTeam(ctx)
	require.False(t, ok)
	_, err = svc.openframeWindowsMDMDeviceContext(t.Context(), 3, true)
	require.Error(t, err, "a deleted enrollment has no tenant")

	ctx, err = svc.openframeWindowsMDMEnrollContext(t.Context(), "node-b", fleet.WindowsMDMEnrollTypeProgrammatic, true)
	require.NoError(t, err)
	got, ok = openframeWindowsMDMTeam(ctx)
	require.True(t, ok)
	require.Equal(t, teamB, got)
	require.Equal(t, &teamB, openframeWindowsMDMEnrollTeamID(ctx), "the enrollment records the enrolling host's tenant")

	_, err = svc.openframeWindowsMDMEnrollContext(t.Context(), "", fleet.WindowsMDMEnrollTypeAutomatic, true)
	require.Error(t, err, "shared mode enrolls only through the agent")

	ds.OpenframeMDMWindowsEnrollmentTeamIDFuncInvoked, ds.LoadHostByOrbitNodeKeyFuncInvoked = false, false
	ctx, err = svc.openframeWindowsMDMDeviceContext(t.Context(), 1, false)
	require.NoError(t, err)
	_, err = svc.openframeWindowsMDMEnrollContext(ctx, "x", fleet.WindowsMDMEnrollTypeAutomatic, false)
	require.NoError(t, err)
	_, ok = openframeWindowsMDMTeam(ctx)
	require.False(t, ok, "outside shared mode nothing is looked up")
	require.Nil(t, openframeWindowsMDMEnrollTeamID(ctx), "outside shared mode no tenant is recorded")
	require.Nil(t, openframeWindowsMDMEnrollTeamID(fleet.NewOpenframeTeamContext(t.Context(), teamA)),
		"a pinned request alone records no tenant on the enrollment")
	require.False(t, ds.OpenframeMDMWindowsEnrollmentTeamIDFuncInvoked)
	require.False(t, ds.LoadHostByOrbitNodeKeyFuncInvoked)
}

func TestOpenframeWindowsReenrollmentDeletesOnlyOwnTenant(t *testing.T) {
	ds := openframeWindowsTenantStore()
	var deletedFor []uint
	ds.OpenframeMDMWindowsDeleteEnrolledDeviceOnReenrollmentFunc = func(_ context.Context, hwID string, teamID uint) error {
		require.Equal(t, "hw-1", hwID)
		deletedFor = append(deletedFor, teamID)
		return nil
	}
	ds.MDMWindowsDeleteEnrolledDeviceOnReenrollmentFunc = func(context.Context, string) error { return nil }
	svc := &Service{ds: ds}

	require.NoError(t, svc.openframeDeleteWindowsDeviceOnReenrollment(openframeWithWindowsMDMTeam(t.Context(), openframeTestTeamB), "hw-1"))
	require.Equal(t, []uint{openframeTestTeamB}, deletedFor, "shared mode deletes within the enrolling tenant")
	require.False(t, ds.MDMWindowsDeleteEnrolledDeviceOnReenrollmentFuncInvoked)

	require.NoError(t, svc.openframeDeleteWindowsDeviceOnReenrollment(t.Context(), "hw-1"))
	require.True(t, ds.MDMWindowsDeleteEnrolledDeviceOnReenrollmentFuncInvoked, "outside shared mode as upstream")
	require.Len(t, deletedFor, 1)
}

func TestOpenframeWindowsEnrollConflictAndPin(t *testing.T) {
	ds := openframeWindowsTenantStore()
	var checked []string
	ds.OpenframeWindowsMDMEnrollConflictFunc = func(_ context.Context, hostUUID, deviceID string, teamID uint) error {
		checked = append(checked, fmt.Sprintf("%s/%s/%d", hostUUID, deviceID, teamID))
		return &fleet.ConflictError{Message: "conflict"}
	}
	svc := &Service{ds: ds}
	msg := &fleet.RequestSecurityToken{AdditionalContext: fleet.AdditionalContext{ContextItems: []fleet.ContextItem{
		{Name: syncml.ReqSecTokenContextItemDeviceID, Value: "dev-1"},
	}}}

	ctxB := openframeWithWindowsMDMTeam(t.Context(), openframeTestTeamB)
	var conflict *fleet.ConflictError
	require.ErrorAs(t, svc.openframeRefuseWindowsEnrollConflict(ctxB, "uuid-1", msg), &conflict)
	require.Equal(t, []string{fmt.Sprintf("uuid-1/dev-1/%d", openframeTestTeamB)}, checked, "shared mode checks the device's identifiers in its tenant")

	require.ErrorContains(t, svc.openframeRefuseWindowsEnrollConflict(ctxB, "", msg), "no host")
	require.Len(t, checked, 1, "an enrollment without a host is refused before any lookup")

	require.NoError(t, svc.openframeRefuseWindowsEnrollConflict(t.Context(), "uuid-1", msg))
	require.Len(t, checked, 1, "outside shared mode nothing is checked")

	pinned, ok := fleet.OpenframeTeamID(openframeWindowsMDMPinnedContext(ctxB))
	require.True(t, ok)
	require.Equal(t, openframeTestTeamB, pinned, "lookups by host uuid are pinned to the device's tenant")
	_, ok = fleet.OpenframeTeamID(openframeWindowsMDMPinnedContext(t.Context()))
	require.False(t, ok, "outside shared mode nothing is pinned")
}

func TestOpenframeWindowsWSTEPPerTenant(t *testing.T) {
	ds := openframeWindowsTenantStore()
	var loads []uint
	load := ds.OpenframeLoadWindowsWSTEPFunc
	ds.OpenframeLoadWindowsWSTEPFunc = func(ctx context.Context, teamID uint, newPair func() ([]byte, []byte, error)) ([]byte, []byte, error) {
		loads = append(loads, teamID)
		return load(ctx, teamID, newPair)
	}
	svc := &Service{ds: ds, openframeWSTEPManagers: new(sync.Map), openframeWSTEPGroup: new(singleflight.Group)}

	_, err := svc.openframeWindowsWSTEPManager(t.Context(), true)
	require.Error(t, err, "a request without a tenant has no WSTEP pair")

	ctxA := openframeWithWindowsMDMTeam(t.Context(), openframeTestTeamA)
	ctxB := openframeWithWindowsMDMTeam(t.Context(), openframeTestTeamB)
	mgrA, err := svc.openframeWindowsWSTEPManager(ctxA, true)
	require.NoError(t, err)
	mgrB, err := svc.openframeWindowsWSTEPManager(ctxB, true)
	require.NoError(t, err)
	require.NotEqual(t, mgrA.IdentityFingerprint(), mgrB.IdentityFingerprint(), "each tenant has its own CA")

	again, err := svc.openframeWindowsWSTEPManager(ctxA, true)
	require.NoError(t, err)
	require.Same(t, mgrA, again)
	require.Equal(t, []uint{openframeTestTeamA, openframeTestTeamB}, loads, "a tenant's pair is read once")

	deviceKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "device"}}, deviceKey)
	require.NoError(t, err)
	csr, err := x509.ParseCertificateRequest(csrDER)
	require.NoError(t, err)

	signed, _, err := mgrA.SignClientCSR(ctxA, "device", csr)
	require.NoError(t, err)
	deviceCert, err := x509.ParseCertificate(signed)
	require.NoError(t, err)
	caA, caB := mgrA.IdentityCert(), mgrB.IdentityCert()
	require.NoError(t, deviceCert.CheckSignatureFrom(&caA), "the device's tenant signs its certificate")
	require.Error(t, deviceCert.CheckSignatureFrom(&caB))

	svc.wstepCertManager = nil
	_, err = svc.openframeWindowsWSTEPManager(ctxA, false)
	require.Error(t, err, "outside shared mode only the configured pair is used")
}

func TestOpenframeWindowsWSTEPFirstEnrollmentsShareOneGeneration(t *testing.T) {
	ds := openframeWindowsTenantStore()
	release := make(chan struct{})
	var generated atomic.Int32
	load := ds.OpenframeLoadWindowsWSTEPFunc
	ds.OpenframeLoadWindowsWSTEPFunc = func(ctx context.Context, teamID uint, newPair func() ([]byte, []byte, error)) ([]byte, []byte, error) {
		generated.Add(1)
		<-release
		return load(ctx, teamID, newPair)
	}
	svc := &Service{ds: ds, openframeWSTEPManagers: new(sync.Map), openframeWSTEPGroup: new(singleflight.Group)}
	ctx := openframeWithWindowsMDMTeam(t.Context(), openframeTestTeamA)

	const enrollments = 8
	managers := make(chan microsoft_mdm.CertManager, enrollments)
	var started, done sync.WaitGroup
	for range enrollments {
		started.Add(1)
		done.Add(1)
		go func() {
			defer done.Done()
			started.Done()
			m, err := svc.openframeWindowsWSTEPManager(ctx, true)
			if err == nil {
				managers <- m
			}
		}()
	}
	started.Wait()
	require.Eventually(t, func() bool { return generated.Load() == 1 }, 5*time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	close(release)
	done.Wait()
	close(managers)

	require.EqualValues(t, 1, generated.Load(), "concurrent first enrollments of a tenant generate its pair once")
	var first microsoft_mdm.CertManager
	n := 0
	for m := range managers {
		if first == nil {
			first = m
		}
		require.Same(t, first, m)
		n++
	}
	require.Equal(t, enrollments, n)
}

func TestOpenframeWindowsBitLockerKeyPerTenant(t *testing.T) {
	ds := openframeWindowsTenantStore()
	svc := &Service{ds: ds}
	teamA, teamB, teamC := openframeTestTeamA, openframeTestTeamB, uint(9)

	certA, err := svc.openframeWindowsWSTEPCertificate(t.Context(), &teamA, true)
	require.NoError(t, err)
	certB, err := svc.openframeWindowsWSTEPCertificate(t.Context(), &teamB, true)
	require.NoError(t, err)

	enc, err := microsoft_mdm.Encrypt("123456-654321", certA.Leaf)
	require.NoError(t, err)
	plain, err := mdm.DecryptBase64CMS(enc, certA.Leaf, certA.PrivateKey)
	require.NoError(t, err)
	require.Equal(t, "123456-654321", string(plain))
	_, err = mdm.DecryptBase64CMS(enc, certB.Leaf, certB.PrivateKey)
	require.Error(t, err, "another tenant's pair can't decrypt the key")

	_, err = svc.openframeWindowsWSTEPCertificate(t.Context(), &teamC, true)
	require.True(t, fleet.IsNotFound(err), "a tenant without a pair gets none: escrow never generates one")
	require.False(t, ds.OpenframeLoadWindowsWSTEPFuncInvoked)
	_, err = svc.openframeWindowsWSTEPCertificate(t.Context(), nil, true)
	require.Error(t, err)
}

func TestOpenframeValidateWindowsMDM(t *testing.T) {
	cases := []struct {
		name    string
		old     fleet.MDM
		new     fleet.MDM
		shared  bool
		invalid bool
	}{
		{"turn on with address", fleet.MDM{}, fleet.MDM{WindowsEnabledAndConfigured: true, WindowsServerURL: "https://acme.example.com/mdm"}, true, false},
		{"turn on without address", fleet.MDM{}, fleet.MDM{WindowsEnabledAndConfigured: true}, true, true},
		{"turn on without address outside shared mode", fleet.MDM{}, fleet.MDM{WindowsEnabledAndConfigured: true}, false, false},
		{"clear address while on", fleet.MDM{WindowsEnabledAndConfigured: true, WindowsServerURL: "https://acme.example.com/mdm"}, fleet.MDM{WindowsEnabledAndConfigured: true}, true, true},
		{"unrelated change on a row without address", fleet.MDM{WindowsEnabledAndConfigured: true}, fleet.MDM{WindowsEnabledAndConfigured: true, AppleServerURL: "x"}, true, false},
		{"http address", fleet.MDM{}, fleet.MDM{WindowsServerURL: "http://acme.example.com/mdm"}, true, true},
		{"address with query", fleet.MDM{}, fleet.MDM{WindowsServerURL: "https://acme.example.com/mdm?x=1"}, false, true},
		{"set address with Windows off", fleet.MDM{}, fleet.MDM{WindowsServerURL: "https://acme.example.com/mdm"}, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			invalid := &fleet.InvalidArgumentError{}
			openframeValidateWindowsMDM(&c.old, &c.new, c.shared, invalid)
			require.Equal(t, c.invalid, invalid.HasErrors(), invalid.Error())
		})
	}
}
