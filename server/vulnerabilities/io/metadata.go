package io

import (
	"fmt"
	"strings"
	"time"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
)

const (
	mSRCFilePrefix              = "fleet_msrc_"
	winOfficePrefix             = "fleet_winoffice_"
	macOfficeReleaseNotesPrefix = "fleet_macoffice_release_notes_"
	fileExt                     = "json"
	dateLayout                  = "2006_01_02"
)

// MSRC Bulletins and other metadata files are published as assets to GH and copies are downloaded to the local FS. The file name
// of those assets contain some useful information like the 'product name' and the date the asset was modified. This type
// provides an abstration around the asset 'file name' to allow us to easy extract/compare the encoded info.
type MetadataFileName struct {
	prefix   string
	filename string
}

func NewMSRCMetadata(filename string) (MetadataFileName, error) {
	mfn := MetadataFileName{prefix: mSRCFilePrefix, filename: filename}

	// Check that the filename contains a valid timestamp
	_, err := mfn.date()

	return mfn, err
}

func NewMacOfficeRelNotesMetadata(filename string) (MetadataFileName, error) {
	mfn := MetadataFileName{prefix: macOfficeReleaseNotesPrefix, filename: filename}

	// Check that the filename contains a valid timestamp
	_, err := mfn.date()

	return mfn, err
}

func NewWinOfficeMetadata(filename string) (MetadataFileName, error) {
	mfn := MetadataFileName{prefix: winOfficePrefix, filename: filename}

	// Check that the filename contains a valid timestamp
	_, err := mfn.date()

	return mfn, err
}

func (mfn MetadataFileName) date() (time.Time, error) {
	parts := strings.Split(mfn.filename, "-")

	if len(parts) != 2 {
		return time.Time{}, ctxerr.New(nil, "invalid file name")
	}
	timeRaw := strings.TrimSuffix(parts[1], "."+fileExt)
	return time.Parse(dateLayout, timeRaw)
}

func (mfn MetadataFileName) Before(other MetadataFileName) bool {
	// If mfn is empty ...
	if mfn.filename == "" {
		return true
	}

	// If other is empty ...
	if other.filename == "" {
		return false
	}

	// We check that the MetadataFileName contains a valid timestamp at construction time so both of
	// these calls shouldn't fail, only reason for having an error at this point is if we are
	// dealing with an 'empty' (MetadataFileName{}) struct.
	a, _ := mfn.date()
	b, _ := other.date()

	return a.Before(b)
}

func (mfn MetadataFileName) ProductName() string {
	pName := strings.TrimPrefix(mfn.filename, mfn.prefix)
	parts := strings.Split(pName, "-")

	if len(parts) != 2 {
		return ""
	}

	return strings.ReplaceAll(parts[0], "_", " ")
}

func (mfn MetadataFileName) String() string {
	return mfn.filename
}

func MSRCFileName(productName string, date time.Time) string {
	pName := strings.ReplaceAll(productName, " ", "_")
	return fmt.Sprintf("%s%s-%d_%02d_%02d.%s", mSRCFilePrefix, pName, date.Year(), date.Month(), date.Day(), fileExt)
}

func MacOfficeRelNotesFileName(date time.Time) string {
	return fmt.Sprintf("%s%s-%d_%02d_%02d.%s", macOfficeReleaseNotesPrefix, "macoffice", date.Year(), date.Month(), date.Day(), fileExt)
}

func WinOfficeFileName(date time.Time) string {
	return fmt.Sprintf("%s%s-%d_%02d_%02d.%s", winOfficePrefix, "bulletin", date.Year(), date.Month(), date.Day(), fileExt)
}
FILE>>>
<<<NOTES
1. CONFIDENCE: 55 - In `date()`, replaced `errors.New("invalid file name")` with `ctxerr.New(nil, "invalid file name")` and removed the `errors` import, adding the `github.com/fleetdm/fleet/v4/server/contexts/ctxerr` import. Risk: `date()` has no `context.Context` parameter, so `nil` is passed to `ctxerr.New`; this compiles and works with fleet's ctxerr implementation (which tolerates nil context) but a complete fix would ideally thread a real `context.Context` through the `MetadataFileName` constructors/`date()` signature for proper ctxerr routing — that would be a larger, riskier signature change across this file's public API (`NewMSRCMetadata`, `NewMacOfficeRelNotesMetadata`, `NewWinOfficeMetadata`) and callers outside this file, which I did not do to keep the change minimal.
2. CONFIDENCE: 85 - In `date()`, changed the error-path return value from `time.Now()` to `time.Time{}` (zero value) so that `Before()`, which ignores the error via `a, _ := mfn.date()`, will sort a malformed timestamp first rather than treating it as "now"/most-recent, as requested.
