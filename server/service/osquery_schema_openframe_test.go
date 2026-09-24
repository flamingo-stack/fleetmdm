// OPENFRAME(osquery-schema-search): tests for the fork-only schema search endpoint.
package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/fleetdm/fleet/v4/schema"
	apiendpoints "github.com/fleetdm/fleet/v4/server/api_endpoints"
	"github.com/fleetdm/fleet/v4/server/config"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mock"
	mockservice "github.com/fleetdm/fleet/v4/server/mock/service"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"github.com/throttled/throttled/v2/store/memstore"
)

func TestSearchOsquerySchemaEndpointReturnsAuthenticationError(t *testing.T) {
	authenticationErr := errors.New("authenticate user")
	svc := &mockservice.Service{
		AuthenticatedUserFunc: func(context.Context) (*fleet.User, error) {
			return nil, authenticationErr
		},
	}

	response, err := searchOsquerySchemaEndpoint(context.Background(), &searchOsquerySchemaRequest{
		Query: "windows update",
	}, svc)
	require.NoError(t, err)
	require.ErrorIs(t, response.Error(), authenticationErr)
}

func authenticatedOsquerySchemaService() fleet.Service {
	return &mockservice.Service{
		AuthenticatedUserFunc: func(context.Context) (*fleet.User, error) {
			return &fleet.User{}, nil
		},
	}
}

func TestSearchOsquerySchemaRouteIsRegistered(t *testing.T) {
	ds := new(mock.Store)
	svc, _ := newTestService(t, ds, nil, nil)
	limitStore, err := memstore.New(0)
	require.NoError(t, err)
	router := MakeHandler(svc, config.TestConfig(), slog.New(slog.DiscardHandler), limitStore, nil, nil, nil).(*mux.Router)

	found := false
	err = router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		path, pathErr := route.GetPathTemplate()
		if pathErr != nil || path != "/api/{fleetversion:(?:v1|2022-04|latest)}/fleet/osquery/schema/search" {
			return nil
		}
		methods, methodsErr := route.GetMethods()
		require.NoError(t, methodsErr)
		found = len(methods) == 1 && methods[0] == "GET"
		return nil
	})
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, apiendpoints.IsInCatalog(fleet.NewAPIEndpointFromTpl("GET", "/api/v1/fleet/osquery/schema/search").Fingerprint()))
}

func TestSearchOsquerySchemaEndpointReturnsRankedSchema(t *testing.T) {
	limit := 5
	response, err := searchOsquerySchemaEndpoint(context.Background(), &searchOsquerySchemaRequest{
		Query:    "windows update result code",
		Platform: "windows",
		Limit:    &limit,
	}, authenticatedOsquerySchemaService())
	require.NoError(t, err)

	searchResponse := response.(searchOsquerySchemaResponse)
	require.NoError(t, searchResponse.Error())
	require.Equal(t, "windows update result code", searchResponse.Query)
	require.Equal(t, "windows", searchResponse.Platform)
	require.Equal(t, len(searchResponse.Tables), searchResponse.Count)
	require.Equal(t, "windows_update_history", searchResponse.Tables[0].Name)
}

func TestSearchOsquerySchemaEndpointAppliesDefaults(t *testing.T) {
	response, err := searchOsquerySchemaEndpoint(context.Background(), &searchOsquerySchemaRequest{
		Query: "os version",
	}, authenticatedOsquerySchemaService())
	require.NoError(t, err)

	searchResponse := response.(searchOsquerySchemaResponse)
	require.NoError(t, searchResponse.Error())
	require.Equal(t, "all", searchResponse.Platform)
	require.LessOrEqual(t, searchResponse.Count, schema.DefaultOsquerySearchLimit)
}

func TestSearchOsquerySchemaEndpointReturnsBadRequestForInvalidInput(t *testing.T) {
	response, err := searchOsquerySchemaEndpoint(context.Background(), &searchOsquerySchemaRequest{
		Query:    "updates",
		Platform: "freebsd",
	}, authenticatedOsquerySchemaService())
	require.NoError(t, err)

	searchResponse := response.(searchOsquerySchemaResponse)
	require.ErrorContains(t, searchResponse.Error(), "unsupported platform")

	zero := 0
	response, err = searchOsquerySchemaEndpoint(context.Background(), &searchOsquerySchemaRequest{
		Query: "updates",
		Limit: &zero,
	}, authenticatedOsquerySchemaService())
	require.NoError(t, err)
	searchResponse = response.(searchOsquerySchemaResponse)
	require.ErrorContains(t, searchResponse.Error(), "limit must be between 1 and 50")
}
