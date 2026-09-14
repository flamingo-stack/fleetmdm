package oval

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	oval_parsed "github.com/fleetdm/fleet/v4/server/vulnerabilities/oval/parsed"
	utils "github.com/fleetdm/fleet/v4/server/vulnerabilities/utils"
)

const (
	hostsBatchSize = 500
	vulnBatchSize  = 500
)

var ErrUnsupportedPlatform = errors.New("unsupported platform")

// Analyze scans all hosts for vulnerabilities based on the OVAL definitions for their platform,
// inserting any new vulnerabilities and deleting anything patched. Returns nil, nil when
// the platform isn't supported.
func Analyze(
	ctx context.Context,
	ds fleet.Datastore,
	ver fleet.OSVersion,
	vulnPath string,
	collectVulns bool,
) ([]fleet.SoftwareVulnerability, error) {
	platform := NewPlatform(ver.Platform, ver.Name)

	source := fleet.UbuntuOVALSource
	if platform.IsRedHat() {
		source = fleet.RHELOVALSource
	}

	if !platform.IsSupported() {
		return nil, ErrUnsupportedPlatform
	}

	defs, err := loadDef(platform, vulnPath)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "load oval definitions")
	}

	rules, err := GetKnownOVALBugRules()
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "get known oval bug rules")
	}

	// Since hosts and software have a M:N relationship, the following sets are used to
	// avoid doing duplicated inserts/delete operations (a vulnerable software might be
	// present in many hosts).
	toInsertSet := make(map[string]fleet.SoftwareVulnerability)
	toDeleteSet := make(map[string]fleet.SoftwareVulnerability)

	var offset int
	for {
		hostIDs, err := ds.HostIDsByOSVersion(ctx, ver, offset, hostsBatchSize)
		if err != nil {
			return nil, ctxerr.Wrap(ctx, err, "get host IDs by os version")
		}

		if len(hostIDs) == 0 {
			break
		}
		offset += hostsBatchSize

		foundInBatch := make(map[uint][]fleet.SoftwareVulnerability)

		for _, hostID := range hostIDs {
			hostID := hostID
			software, err := ds.ListSoftwareForVulnDetection(ctx, fleet.VulnSoftwareFilter{HostID: &hostID})
			if err != nil {
				return nil, ctxerr.Wrap(ctx, err, "list software for vuln detection")
			}

			evalR, err := defs.Eval(ver, software)
			if err != nil {
				return nil, ctxerr.Wrap(ctx, err, "eval oval definitions")
			}
			foundInBatch[hostID] = evalR

			evalU, err := defs.EvalKernel(software)
			if err != nil {
				return nil, ctxerr.Wrap(ctx, err, "eval kernel oval definitions")
			}
			foundInBatch[hostID] = append(foundInBatch[hostID], evalU...)

			// Create a map of id: software for each
			// pair (id, cve) in foundInBatch for this host
			softwareIDs := make(map[uint]fleet.Software)
			for _, s := range software {
				softwareIDs[s.ID] = s
			}

			filteredBatch := make([]fleet.SoftwareVulnerability, 0, len(foundInBatch[hostID]))
			for _, v := range foundInBatch[hostID] {
				software := softwareIDs[v.SoftwareID]
				skip := rules.MatchesAny(software, v.CVE)
				if !skip {
					filteredBatch = append(filteredBatch, v)
				}
			}

			foundInBatch[hostID] = filteredBatch
		}

		existingInBatch, err := ds.ListSoftwareVulnerabilitiesByHostIDsSource(ctx, hostIDs, source)
		if err != nil {
			return nil, ctxerr.Wrap(ctx, err, "list software vulnerabilities by host ids source")
		}

		for _, hostID := range hostIDs {
			insrt, del := utils.VulnsDelta(foundInBatch[hostID], existingInBatch[hostID])
			for _, i := range insrt {
				toInsertSet[i.Key()] = i
			}
			for _, d := range del {
				toDeleteSet[d.Key()] = d
			}
		}
	}

	err = utils.BatchProcess(toDeleteSet, func(v []fleet.SoftwareVulnerability) error {
		return ds.DeleteSoftwareVulnerabilities(ctx, v)
	}, vulnBatchSize)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "delete software vulnerabilities")
	}

	allVulns := make([]fleet.SoftwareVulnerability, 0, len(toInsertSet))
	for _, v := range toInsertSet {
		allVulns = append(allVulns, v)
	}

	newVulns, err := ds.InsertSoftwareVulnerabilities(ctx, allVulns, source)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "insert software vulnerabilities")
	}
	if !collectVulns {
		return newVulns, nil
	}

	return newVulns, nil
}

// loadDef returns the latest oval Definition for the given platform.
// Returns an error if the loaded definition file contains no rules, since an empty
// definition would cause every existing vulnerability for the platform to be deleted
// (it would look like every host was suddenly patched). An empty file usually means
// the artifact download from GitHub was corrupted or partially failed.
func loadDef(platform Platform, vulnPath string) (oval_parsed.Result, error) {
	if !platform.IsSupported() {
		return nil, ctxerr.Errorf(context.Background(), "platform %q not supported", platform)
	}

	fileName := platform.ToFilename(time.Now(), "json")
	latest, err := utils.LatestFile(fileName, vulnPath)
	if err != nil {
		return nil, ctxerr.Wrap(context.Background(), err, "get latest oval file")
	}
	payload, err := os.ReadFile(latest)
	if err != nil {
		return nil, ctxerr.Wrap(context.Background(), err, "read oval file")
	}

	if platform.IsUbuntu() {
		result := oval_parsed.UbuntuResult{}
		if err := json.Unmarshal(payload, &result); err != nil {
			return nil, ctxerr.Wrap(context.Background(), err, "unmarshal ubuntu oval result")
		}
		if len(result.Definitions) == 0 {
			return nil, ctxerr.Errorf(context.Background(), "OVAL definition file %q contains no rules (possible corrupted feed)", latest)
		}
		return result, nil
	}

	if platform.IsRedHat() {
		result := oval_parsed.RhelResult{}
		if err := json.Unmarshal(payload, &result); err != nil {
			return nil, ctxerr.Wrap(context.Background(), err, "unmarshal rhel oval result")
		}
		if len(result.Definitions) == 0 {
			return nil, ctxerr.Errorf(context.Background(), "OVAL definition file %q contains no rules (possible corrupted feed)", latest)
		}
		return result, nil
	}

	return nil, ctxerr.Errorf(context.Background(), "don't know how to parse file %q for %q platform", latest, platform)
}
