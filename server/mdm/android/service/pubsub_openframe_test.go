package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/fleetdm/fleet/v4/server/config"
	"github.com/fleetdm/fleet/v4/server/contexts/viewer"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mdm/android"
	android_mock "github.com/fleetdm/fleet/v4/server/mdm/android/mock"
	common_mysql "github.com/fleetdm/fleet/v4/server/platform/mysql"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/androidmanagement/v1"
)

func TestOpenframeAndroidEnrollTeam(t *testing.T) {
	tenantA, tenantB := new(uint(5)), new(uint(6))

	cases := []struct {
		name       string
		secret     *uint
		prior      *uint
		priorFound bool
		shared     bool
		want       *uint
	}{
		{"shared mode keeps the secret's tenant when the device was another tenant's", tenantB, tenantA, true, true, tenantB},
		{"shared mode keeps the secret's tenant for a device seen in the same tenant", tenantB, tenantB, true, true, tenantB},
		{"shared mode keeps the secret's tenant for a new device", tenantB, nil, false, true, tenantB},
		{"upstream restores the device's last team", tenantB, tenantA, true, false, tenantA},
		{"upstream restores a last team of no team", tenantB, nil, true, false, nil},
		{"upstream uses the secret's team for a new device", tenantB, nil, false, false, tenantB},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, openframeAndroidEnrollTeam(c.secret, c.prior, c.priorFound, c.shared))
		})
	}
}

func TestOpenframeAndroidReenrollAsNewHost(t *testing.T) {
	tenantA, tenantB := new(uint(5)), new(uint(6))

	cases := []struct {
		name         string
		host, secret *uint
		shared       bool
		want         bool
	}{
		{"shared mode, another tenant's host", tenantA, tenantB, true, true},
		{"shared mode, same tenant by value", tenantB, new(uint(6)), true, false},
		{"shared mode, host without a team", nil, tenantB, true, true},
		{"shared mode, secret without a team", tenantA, nil, true, true},
		{"shared mode, neither has a team", nil, nil, true, false},
		{"upstream never re-creates the host", tenantA, tenantB, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, openframeAndroidReenrollAsNewHost(c.host, c.secret, c.shared))
		})
	}
}

func TestOpenframeAndroidReenrollIntoAnotherTenant(t *testing.T) {
	const (
		existingHostID   = uint(20)
		existingHostUUID = "EXISTING-HOST-OTHER-TENANT"
	)
	tenantA, tenantB := uint(5), uint(6)

	type outcome struct {
		deletedHostID    *uint
		newHostTeamID    *uint
		movedHostTeamID  *uint
		newHostInvoked   bool
		updateInvoked    bool
		addToTeamInvoked bool
		tenantChange     bool
	}

	run := func(t *testing.T, shared bool, hostTeam, secretTeam uint) outcome {
		svc, mockDS := createAndroidService(t)
		var got outcome

		mockDS.AppConfigFunc = func(ctx context.Context) (*fleet.AppConfig, error) {
			return &fleet.AppConfig{MDM: fleet.MDM{AndroidEnabledAndConfigured: true}}, nil
		}
		mockDS.AndroidHostLiteFunc = func(ctx context.Context, esID string) (*fleet.AndroidHost, error) {
			return &fleet.AndroidHost{
				Host: &fleet.Host{ID: existingHostID, UUID: existingHostUUID, TeamID: new(hostTeam)},
				Device: &android.Device{
					HostID:               existingHostID,
					DeviceID:             "other-tenant-device",
					EnterpriseSpecificID: new(existingHostUUID),
				},
			}, nil
		}
		mockDS.VerifyEnrollSecretFunc = func(ctx context.Context, secret string) (*fleet.EnrollSecret, error) {
			return &fleet.EnrollSecret{Secret: secret, TeamID: new(secretTeam)}, nil
		}
		mockDS.GetAndroidDeviceLastTeamIDFunc = func(ctx context.Context, esID string) (*uint, bool, error) {
			return new(hostTeam), true, nil
		}
		mockDS.DeleteHostFunc = func(ctx context.Context, hid uint) error {
			got.deletedHostID = new(hid)
			got.tenantChange = fleet.OpenframeHostTenantChangeFromContext(ctx)
			return nil
		}
		mockDS.NewAndroidHostFunc = func(ctx context.Context, host *fleet.AndroidHost, companyOwned bool) (*fleet.AndroidHost, error) {
			got.newHostInvoked = true
			got.newHostTeamID = host.TeamID
			return &fleet.AndroidHost{Host: &fleet.Host{ID: 21, TeamID: host.TeamID}}, nil
		}
		mockDS.MaybeAssociateHostWithScimUserFunc = func(ctx context.Context, hostID uint) error {
			return nil
		}
		mockDS.UpdateAndroidHostFunc = func(ctx context.Context, host *fleet.AndroidHost, fromEnroll, companyOwned bool) error {
			got.updateInvoked = true
			return nil
		}
		mockDS.AddHostsToTeamFunc = func(ctx context.Context, params *fleet.AddHostsToTeamParams) error {
			got.addToTeamInvoked = true
			got.movedHostTeamID = params.TeamID
			return nil
		}
		mockDS.DeleteAllHostCertificateTemplatesFunc = func(ctx context.Context, hostUUID string) error {
			return nil
		}
		mockDS.ClearHostMDMActionsFunc = func(ctx context.Context, hostID uint) error {
			return nil
		}
		mockDS.ScimUserByHostIDFunc = func(ctx context.Context, hostID uint) (*fleet.ScimUser, error) {
			return nil, common_mysql.NotFound("scim user")
		}
		mockDS.ListHostDeviceMappingFunc = func(ctx context.Context, id uint) ([]*fleet.HostDeviceMapping, error) {
			return nil, nil
		}

		enrollTokenData, err := json.Marshal(enrollmentTokenRequest{EnrollSecret: "tenant-secret"})
		require.NoError(t, err)
		message := createEnrollmentMessage(t, androidmanagement.Device{
			Name:                createAndroidDeviceId("other-tenant-device"),
			EnrollmentTokenData: string(enrollTokenData),
		})

		ctx := context.WithValue(t.Context(), openframeSharedModeCtxKey{}, shared)
		require.NoError(t, svc.ProcessPubSubPush(ctx, "value", message))
		return got
	}

	t.Run("shared mode deletes the previous tenant's host and enrolls a new one", func(t *testing.T) {
		got := run(t, true, tenantA, tenantB)
		require.NotNil(t, got.deletedHostID)
		require.Equal(t, existingHostID, *got.deletedHostID)
		require.True(t, got.tenantChange, "the deletion drops the previous tenant's IdP link")
		require.True(t, got.newHostInvoked)
		require.Equal(t, new(tenantB), got.newHostTeamID)
		require.False(t, got.updateInvoked, "the previous tenant's host row is not reused")
		require.False(t, got.addToTeamInvoked, "the previous tenant's host row is not moved")
	})

	t.Run("shared mode keeps the host when the device re-enrolls into the same tenant", func(t *testing.T) {
		got := run(t, true, tenantB, tenantB)
		require.Nil(t, got.deletedHostID)
		require.False(t, got.newHostInvoked)
		require.True(t, got.updateInvoked)
		require.Equal(t, new(tenantB), got.movedHostTeamID)
	})

	t.Run("upstream keeps the host and restores its last team", func(t *testing.T) {
		got := run(t, false, tenantA, tenantB)
		require.Nil(t, got.deletedHostID)
		require.False(t, got.newHostInvoked)
		require.True(t, got.updateInvoked)
		require.Equal(t, new(tenantA), got.movedHostTeamID)
	})
}

func TestOpenframeEnterpriseSignupUsesInstanceConfig(t *testing.T) {
	run := func(t *testing.T, shared bool) (signupServerURL string, instanceRead bool) {
		androidAPIClient := android_mock.Client{}
		androidAPIClient.InitCommonMocks()
		androidAPIClient.SignupURLsCreateFunc = func(ctx context.Context, serverURL, callbackURL string) (*android.SignupDetails, error) {
			signupServerURL = serverURL
			require.True(t, strings.HasPrefix(callbackURL, serverURL+"/api/v1/fleet/android_enterprise/connect/"))
			return &android.SignupDetails{Url: "https://signup.example.com", Name: "signup"}, nil
		}
		mockDS := InitCommonDSMocks()
		mockDS.AppConfigFunc = func(ctx context.Context) (*fleet.AppConfig, error) {
			cfg := &fleet.AppConfig{}
			if fleet.OpenframeInstanceConfigFromContext(ctx) {
				instanceRead = true
				cfg.ServerSettings.ServerURL = "https://mdm.example.com"
			} else {
				cfg.ServerSettings.ServerURL = "https://tenant.example.com"
			}
			return cfg, nil
		}
		svc, err := NewServiceWithClient(slog.New(slog.DiscardHandler), mockDS, &androidAPIClient, "test-private-key", &mockDS.DataStore, noopNewActivity, config.AndroidAgentConfig{})
		require.NoError(t, err)

		ctx := viewer.NewContext(context.Background(), viewer.Viewer{User: &fleet.User{ID: 1, GlobalRole: new(fleet.RoleAdmin)}})
		ctx = context.WithValue(ctx, openframeSharedModeCtxKey{}, shared)
		_, err = svc.EnterpriseSignup(ctx)
		require.NoError(t, err)
		return signupServerURL, instanceRead
	}

	t.Run("shared mode signs up with the instance row's URL", func(t *testing.T) {
		serverURL, instanceRead := run(t, true)
		require.True(t, instanceRead)
		require.Equal(t, "https://mdm.example.com", serverURL)
	})

	t.Run("upstream signs up with the request's config", func(t *testing.T) {
		serverURL, instanceRead := run(t, false)
		require.False(t, instanceRead)
		require.Equal(t, "https://tenant.example.com", serverURL)
	})
}

func TestOpenframeEnterpriseSignupRequiresInstanceServerURL(t *testing.T) {
	androidAPIClient := android_mock.Client{}
	androidAPIClient.InitCommonMocks()
	mockDS := InitCommonDSMocks()
	mockDS.AppConfigFunc = func(ctx context.Context) (*fleet.AppConfig, error) {
		return &fleet.AppConfig{}, nil
	}
	svc, err := NewServiceWithClient(slog.New(slog.DiscardHandler), mockDS, &androidAPIClient, "test-private-key", &mockDS.DataStore, noopNewActivity, config.AndroidAgentConfig{})
	require.NoError(t, err)

	ctx := viewer.NewContext(context.Background(), viewer.Viewer{User: &fleet.User{ID: 1, GlobalRole: new(fleet.RoleAdmin)}})
	ctx = context.WithValue(ctx, openframeSharedModeCtxKey{}, true)
	_, err = svc.EnterpriseSignup(ctx)

	var badRequest *fleet.BadRequestError
	require.ErrorAs(t, err, &badRequest)
	require.Contains(t, badRequest.Message, "FLEET_OPENFRAME_INSTANCE_SERVER_URL")
	require.False(t, androidAPIClient.SignupURLsCreateFuncInvoked, "nothing is sent to Google")
	require.False(t, mockDS.CreateEnterpriseFuncInvoked, "no enterprise row is created")
}
