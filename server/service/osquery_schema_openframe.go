// OPENFRAME(osquery-schema-search): searchable canonical osquery schema for OpenFrame's AI query generation.
package service

import (
	"context"
	"strings"

	"github.com/fleetdm/fleet/v4/schema"
	"github.com/fleetdm/fleet/v4/server/fleet"
)

type searchOsquerySchemaRequest struct {
	Query    string `query:"query"`
	Platform string `query:"platform,optional"`
	Limit    *int   `query:"limit,optional"`
}

type searchOsquerySchemaResponse struct {
	Query    string                `json:"query"`
	Platform string                `json:"platform"`
	Count    int                   `json:"count"`
	Tables   []schema.OsqueryTable `json:"tables"`
	Err      error                 `json:"error,omitempty"`
}

func (r searchOsquerySchemaResponse) Error() error { return r.Err }

func searchOsquerySchemaEndpoint(ctx context.Context, request interface{}, svc fleet.Service) (fleet.Errorer, error) {
	if _, err := svc.AuthenticatedUser(ctx); err != nil {
		return searchOsquerySchemaResponse{Err: err}, nil
	}

	req := request.(*searchOsquerySchemaRequest)
	platform, err := schema.NormalizeOsqueryPlatform(req.Platform)
	if err != nil {
		return searchOsquerySchemaResponse{Err: badRequest(err.Error())}, nil
	}

	limit := schema.DefaultOsquerySearchLimit
	if req.Limit != nil {
		limit = *req.Limit
	}
	tables, err := schema.SearchOsqueryTables(req.Query, platform, limit)
	if err != nil {
		return searchOsquerySchemaResponse{Err: badRequest(err.Error())}, nil
	}

	query := strings.TrimSpace(req.Query)
	return searchOsquerySchemaResponse{
		Query:    query,
		Platform: platform,
		Count:    len(tables),
		Tables:   tables,
	}, nil
}
