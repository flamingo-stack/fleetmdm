package storage

import (
	"errors"

	"github.com/fleetdm/fleet/v4/server/mdm/nanodep/client"
	"github.com/fleetdm/fleet/v4/server/mdm/nanodep/http/api"
	"github.com/fleetdm/fleet/v4/server/mdm/nanodep/sync"
)

// ErrNotFound is returned by AllStorage when a requested resource is not found.
var ErrNotFound = errors.New("resource not found")

// >>> OPENFRAME(nanodep-allstorage-rename): avoid name collision with nanomdm.AllStorage so internal mockimpl tooling can resolve both interfaces — openframe/docs/nanodep-storage-rename.md
// AllDEPStorage represents all possible required storage used by NanoDEP.
// Renamed from AllStorage to avoid ambiguity with the nanomdm AllStorage
// interface, which our mockimpl tool cannot resolve correctly.
type AllDEPStorage interface {
	client.AuthTokensRetriever
	client.ConfigRetriever
	sync.AssignerProfileRetriever
	sync.CursorStorage
	api.AuthTokensStorer
	api.ConfigStorer
	api.TokenPKIStorer
	api.TokenPKIRetriever
	api.AssignerProfileStorer
}
// <<< OPENFRAME(nanodep-allstorage-rename)

