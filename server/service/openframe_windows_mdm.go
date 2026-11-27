// >>> OPENFRAME(windows-mdm-tenant): Windows MDM URLs and the WSTEP pair come from the device's tenant — openframe/docs/mysql-multitenancy-feature.md

package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"strconv"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	microsoft_mdm "github.com/fleetdm/fleet/v4/server/mdm/microsoft"
)

var errOpenframeWindowsMDMURLNotSet = errors.New("the tenant has no Windows MDM address (mdm.windows_server_url)")

type openframeWindowsMDMTeamCtxKey struct{}

// openframeWithWindowsMDMTeam names the tenant of a Windows device request (enroll, management)
// without pinning it: the rest of the request keeps reading as before, only the Windows MDM URLs
// and the WSTEP pair are taken from this tenant.
func openframeWithWindowsMDMTeam(ctx context.Context, teamID uint) context.Context {
	return context.WithValue(ctx, openframeWindowsMDMTeamCtxKey{}, teamID)
}

func openframeWindowsMDMTeam(ctx context.Context) (uint, bool) {
	if teamID, ok := ctx.Value(openframeWindowsMDMTeamCtxKey{}).(uint); ok && teamID != 0 {
		return teamID, true
	}
	return fleet.OpenframeTeamID(ctx)
}

// openframeWindowsMDMServerURL returns the base URL the Windows MDM URLs are built from: the
// server URL as upstream, or in shared mode the tenant's mdm.windows_server_url.
func openframeWindowsMDMServerURL(appConfig *fleet.AppConfig, shared bool) (string, error) {
	if !shared {
		return appConfig.ServerSettings.ServerURL, nil
	}
	if appConfig.MDM.WindowsServerURL == "" {
		return "", errOpenframeWindowsMDMURLNotSet
	}
	return fleet.ParseOpenframeWindowsServerURL(appConfig.MDM.WindowsServerURL)
}

// openframeWindowsMDMAppConfig returns the config the Windows MDM URLs come from: in shared mode
// the row of the request's tenant, otherwise the config the context reads.
func (svc *Service) openframeWindowsMDMAppConfig(ctx context.Context, shared bool) (*fleet.AppConfig, error) {
	if !shared {
		return svc.ds.AppConfig(ctx)
	}
	teamID, ok := openframeWindowsMDMTeam(ctx)
	if !ok {
		return nil, ctxerr.New(ctx, "Windows MDM request has no tenant")
	}
	return svc.ds.AppConfig(fleet.NewOpenframeTeamContext(ctx, teamID))
}

// openframeWindowsMDMBaseURL is openframeWindowsMDMAppConfig and openframeWindowsMDMServerURL in one.
func (svc *Service) openframeWindowsMDMBaseURL(ctx context.Context, shared bool) (string, error) {
	appConfig, err := svc.openframeWindowsMDMAppConfig(ctx, shared)
	if err != nil {
		return "", ctxerr.Wrap(ctx, err, "reading the config for the Windows MDM address")
	}
	serverURL, err := openframeWindowsMDMServerURL(appConfig, shared)
	if err != nil {
		return "", ctxerr.Wrap(ctx, err, "Windows MDM address")
	}
	return serverURL, nil
}

// openframeWindowsMDMDeviceContext names the tenant the enrolled device enrolled into for the rest of a
// management request. The tenant is recorded on the enrollment, not derived from identifiers the device
// sends: an enrollment without one (enrolled before it was recorded) is refused until it re-enrolls.
// Outside shared mode it returns ctx unchanged.
func (svc *Service) openframeWindowsMDMDeviceContext(ctx context.Context, enrollmentID uint, shared bool) (context.Context, error) {
	if !shared {
		return ctx, nil
	}
	teamID, err := svc.ds.OpenframeMDMWindowsEnrollmentTeamID(ctx, enrollmentID)
	if err != nil {
		return ctx, ctxerr.Wrap(ctx, err, "loading the tenant of a Windows MDM enrollment")
	}
	if teamID == nil {
		return ctx, ctxerr.New(ctx, "Windows MDM enrollment has no tenant recorded; the device must re-enroll")
	}
	return openframeWithWindowsMDMTeam(ctx, *teamID), nil
}

// openframeWindowsMDMEnrollTeamID is the tenant openframeWindowsMDMEnrollContext named for this enrollment,
// recorded on the enrollment row; nil outside shared mode.
func openframeWindowsMDMEnrollTeamID(ctx context.Context) *uint {
	if teamID, ok := ctx.Value(openframeWindowsMDMTeamCtxKey{}).(uint); ok && teamID != 0 {
		return &teamID
	}
	return nil
}

// openframeDeleteWindowsDeviceOnReenrollment removes the enrollment the re-enrolling device's hardware id
// names. In shared mode only an enrollment of the device's tenant is removed; one of another tenant
// refuses the enrollment.
func (svc *Service) openframeDeleteWindowsDeviceOnReenrollment(ctx context.Context, mdmDeviceHWID string) error {
	if teamID := openframeWindowsMDMEnrollTeamID(ctx); teamID != nil {
		return svc.ds.OpenframeMDMWindowsDeleteEnrolledDeviceOnReenrollment(ctx, mdmDeviceHWID, *teamID)
	}
	return svc.ds.MDMWindowsDeleteEnrolledDeviceOnReenrollment(ctx, mdmDeviceHWID)
}

// openframeWindowsMDMEnrollContext names the tenant of the host enrolling with this orbit node key.
// Shared mode enrolls only through the agent (programmatic enrollment): another enrollment type
// carries no host, so its tenant is unknown.
func (svc *Service) openframeWindowsMDMEnrollContext(ctx context.Context, orbitNodeKey string, enrollType fleet.WindowsMDMEnrollType, shared bool) (context.Context, error) {
	if !shared {
		return ctx, nil
	}
	if enrollType != fleet.WindowsMDMEnrollTypeProgrammatic {
		return ctx, ctxerr.New(ctx, "Windows MDM enrolls only through the agent in shared mode")
	}
	host, err := svc.ds.LoadHostByOrbitNodeKey(ctx, orbitNodeKey)
	if err != nil {
		return ctx, ctxerr.Wrap(ctx, err, "loading the enrolling host")
	}
	if host == nil {
		return ctx, ctxerr.New(ctx, "no host for the enrolling orbit node key")
	}
	return openframeWindowsMDMTeamOfHost(ctx, host.TeamID)
}

func openframeWindowsMDMTeamOfHost(ctx context.Context, teamID *uint) (context.Context, error) {
	if teamID == nil || *teamID == 0 {
		return ctx, ctxerr.New(ctx, "Windows MDM host has no tenant team")
	}
	return openframeWithWindowsMDMTeam(ctx, *teamID), nil
}

// openframeWindowsWSTEPManager returns the cert manager that signs the device certificate of the
// request: in shared mode its tenant's, generated with the tenant's first enrollment.
func (svc *Service) openframeWindowsWSTEPManager(ctx context.Context, shared bool) (microsoft_mdm.CertManager, error) {
	if !shared {
		if svc.wstepCertManager == nil {
			return nil, errors.New("windows mdm identity keypair was not configured")
		}
		return svc.wstepCertManager, nil
	}
	teamID, ok := openframeWindowsMDMTeam(ctx)
	if !ok {
		return nil, ctxerr.New(ctx, "Windows MDM request has no tenant")
	}
	return svc.openframeTenantWSTEPManager(ctx, teamID)
}

// openframeTenantWSTEPManager returns the tenant's cert manager. Concurrent first enrollments of a
// tenant share one load: the pair is generated (RSA 4096) once per process, not once per request.
func (svc *Service) openframeTenantWSTEPManager(ctx context.Context, teamID uint) (microsoft_mdm.CertManager, error) {
	if svc.openframeWSTEPManagers != nil {
		if m, ok := svc.openframeWSTEPManagers.Load(teamID); ok {
			return m.(microsoft_mdm.CertManager), nil
		}
	}
	if svc.openframeWSTEPGroup == nil {
		return svc.openframeLoadTenantWSTEPManager(ctx, teamID)
	}
	m, err, _ := svc.openframeWSTEPGroup.Do(strconv.FormatUint(uint64(teamID), 10), func() (any, error) {
		return svc.openframeLoadTenantWSTEPManager(context.WithoutCancel(ctx), teamID)
	})
	if err != nil {
		return nil, err
	}
	return m.(microsoft_mdm.CertManager), nil
}

func (svc *Service) openframeLoadTenantWSTEPManager(ctx context.Context, teamID uint) (microsoft_mdm.CertManager, error) {
	if svc.openframeWSTEPManagers != nil {
		if m, ok := svc.openframeWSTEPManagers.Load(teamID); ok {
			return m.(microsoft_mdm.CertManager), nil
		}
	}
	certPEM, keyPEM, err := svc.ds.OpenframeLoadWindowsWSTEP(ctx, teamID, microsoft_mdm.OpenframeNewWSTEPPair)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "loading the tenant's Windows MDM WSTEP pair")
	}
	m, err := microsoft_mdm.NewCertManager(svc.ds, certPEM, keyPEM)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "reading the tenant's Windows MDM WSTEP pair")
	}
	if svc.openframeWSTEPManagers == nil {
		return m, nil
	}
	actual, _ := svc.openframeWSTEPManagers.LoadOrStore(teamID, m)
	return actual.(microsoft_mdm.CertManager), nil
}

// openframeWindowsWSTEPCertificate returns the pair that encrypts and decrypts a Windows host's
// BitLocker key: in shared mode the host's tenant's. It is never generated here: a tenant has
// escrowed keys only after its first enrollment generated the pair.
func (svc *Service) openframeWindowsWSTEPCertificate(ctx context.Context, teamID *uint, shared bool) (*tls.Certificate, error) {
	if !shared {
		cert, _, _, err := svc.config.MDM.MicrosoftWSTEP()
		return cert, err
	}
	if teamID == nil || *teamID == 0 {
		return nil, ctxerr.New(ctx, "Windows host has no tenant team")
	}
	certPEM, keyPEM, err := svc.ds.OpenframeGetWindowsWSTEP(ctx, *teamID)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "loading the tenant's Windows MDM WSTEP pair")
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "reading the tenant's Windows MDM WSTEP pair")
	}
	if cert.Leaf == nil {
		if cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0]); err != nil {
			return nil, ctxerr.Wrap(ctx, err, "parsing the tenant's Windows MDM WSTEP certificate")
		}
	}
	return &cert, nil
}

// openframeValidateWindowsMDM checks mdm.windows_server_url on a config change. In shared mode
// a tenant can't turn Windows MDM on, or keep it on, without the address.
func openframeValidateWindowsMDM(oldMdm, mdm *fleet.MDM, shared bool, invalid *fleet.InvalidArgumentError) {
	changed := mdm.WindowsServerURL != oldMdm.WindowsServerURL
	if changed && mdm.WindowsServerURL != "" {
		if _, err := fleet.ParseOpenframeWindowsServerURL(mdm.WindowsServerURL); err != nil {
			invalid.Append("mdm.windows_server_url", err.Error())
		}
	}
	turnedOn := mdm.WindowsEnabledAndConfigured && !oldMdm.WindowsEnabledAndConfigured
	if shared && mdm.WindowsEnabledAndConfigured && mdm.WindowsServerURL == "" && (turnedOn || changed) {
		invalid.Append("mdm.windows_server_url", "Couldn't turn on Windows MDM. Set the tenant's Windows MDM address first.")
	}
}

// <<< OPENFRAME(windows-mdm-tenant)
