package externalrefs

import (
	"log"

	maintained_apps "github.com/fleetdm/fleet/v4/ee/maintained-apps"
)

// Funcs is a registry of enrichment functions keyed by app slug.
// Each slug can have multiple enricher functions that run sequentially.
var Funcs = map[string][]func(*maintained_apps.FMAManifestApp) (*maintained_apps.FMAManifestApp, error){
	"1password/windows": {OnePasswordVersionShortener},
}

// EnrichManifest applies all registered enrichment functions for the given app.
// Enrichers are looked up by app.Slug and run sequentially.
// Errors are logged and stop the enrichment pipeline for that app, preserving
// the last known-good app value.
func EnrichManifest(app *maintained_apps.FMAManifestApp) {
	if enrichers, ok := Funcs[app.Slug]; ok {
		for _, enricher := range enrichers {
			enriched, err := enricher(app)
			if err != nil {
				log.Printf("Error enriching app %s: %v\n", app.UniqueIdentifier, err)
				break
			}
			app = enriched
		}
	}
}

