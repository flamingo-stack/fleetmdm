package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fleetdm/fleet/v4/server/vulnerabilities/macoffice"
	"github.com/fleetdm/fleet/v4/server/vulnerabilities/msrc/parsed"
	"github.com/fleetdm/fleet/v4/server/vulnerabilities/nvd"
	"github.com/jmoiron/sqlx"
)

func main() {
	dbDir := flag.String("db_dir", "/tmp/vulndbs", "Path to the vulnerability database")
	flag.Parse()

	vulnPath := *dbDir
	if err := checkCPETranslations(vulnPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := checkMacOfficeNotes(vulnPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := checkMSRCVulnerabilities(vulnPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := checkSqliteDb(vulnPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func checkCPETranslations(vulnPath string) error {
	// Check that the CPE translations file is parseable into an array of CPETranslationItem
	_, err := nvd.LoadCPETranslations(filepath.Join(vulnPath, "cpe_translations.json"))
	if err != nil {
		return fmt.Errorf("failed to load CPE translations: %w", err)
	}
	return nil
}

func checkMacOfficeNotes(vulnPath string) error {
	// Iterate over each file in the vulnPath directory that starts with `fleet_macoffice`
	files, err := os.ReadDir(vulnPath)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "fleet_macoffice") && strings.HasSuffix(file.Name(), ".json") {
			filePath := filepath.Join(vulnPath, file.Name())

			payload, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read MacOffice release notes file %s: %w", file.Name(), err)
			}
			// Attempt to parse the file as a MacOffice release notes.
			relNotes := macoffice.ReleaseNotes{}
			err = json.Unmarshal(payload, &relNotes)
			if err != nil {
				return fmt.Errorf("failed to parse MacOffice release notes %s: %w", file.Name(), err)
			}
		}
	}
	return nil
}

func checkMSRCVulnerabilities(vulnPath string) error {
	// Iterate over each file in the vulnPath directory that starts with `fleet_msrc`
	files, err := os.ReadDir(vulnPath)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "fleet_msrc") && strings.HasSuffix(file.Name(), ".json") {
			filePath := filepath.Join(vulnPath, file.Name())
			// Attempt to parse the file as a MSRC feed.
			_, err := parsed.UnmarshalBulletin(filePath)
			if err != nil {
				return fmt.Errorf("failed to parse MSRC feed %s: %w", file.Name(), err)
			}
		}
	}
	return nil
}

func checkSqliteDb(vulnPath string) error {
	// Iterate over each file in the vulnPath directory to find the sqlite.gz file
	files, err := os.ReadDir(vulnPath)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}
	var sqliteFilename string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".sqlite.gz") {
			sqliteFilename = file.Name()
			break
		}
	}
	if sqliteFilename == "" {
		return fmt.Errorf("no sqlite.gz file found: %w", err)
	}
	// Unzip the sqlite.gz file and create a new sqlite.db file
	gzFile, err := os.Open(filepath.Join(vulnPath, sqliteFilename))
	if err != nil {
		return fmt.Errorf("error opening sqlite.gz file: %w", err)
	}
	defer gzFile.Close()
	sqliteFile, err := os.Create(filepath.Join(vulnPath, "sqlite.db"))
	if err != nil {
		return fmt.Errorf("error creating test sqlite.db file: %w", err)
	}
	defer sqliteFile.Close()
	gzReader, err := gzip.NewReader(gzFile)
	if err != nil {
		return fmt.Errorf("error creating new gzip reader: %w", err)
	}
	defer gzReader.Close()
	for {
		_, err := io.CopyN(sqliteFile, gzReader, 100*1024*1024)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error unzipping sqlite file: %w", err)
		}
	}
	db, err := sqlx.Open("sqlite3", filepath.Join(vulnPath, "sqlite.db"))
	if err != nil {
		return fmt.Errorf("error opening sqlite db: %w", err)
	}
	// Check that the database is valid
	_, err = db.Exec(`SELECT * FROM cpe_2 LIMIT 1`)
	if err != nil {
		return fmt.Errorf("error executing query on sqlite db: %w", err)
	}
	return nil
}
