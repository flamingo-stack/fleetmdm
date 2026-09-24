// Package schema exposes Fleet's vendored osquery schema to server-side consumers.
package schema

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
)

const (
	DefaultOsquerySearchLimit  = 20
	MaxOsquerySearchLimit      = 50
	maxOsquerySearchQueryBytes = 512
	maxOsquerySearchTerms      = 32
)

// OsqueryTable is the searchable subset of a table in Fleet's canonical schema.
type OsqueryTable struct {
	Name        string          `json:"name"`
	Platforms   []string        `json:"platforms"`
	Description string          `json:"description"`
	Columns     []OsqueryColumn `json:"columns"`
	Examples    *string         `json:"examples"`
	Notes       *string         `json:"notes"`
	URL         string          `json:"url,omitempty"`
	Evented     bool            `json:"evented"`
	Cacheable   bool            `json:"cacheable"`
}

// OsqueryColumn describes a column, including constraints required to query it.
type OsqueryColumn struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Platforms   []string `json:"platforms,omitempty"`
	Notes       *string  `json:"notes"`
	Required    bool     `json:"required"`
	Hidden      *bool    `json:"hidden"`
	Index       *bool    `json:"index,omitempty"`
}

type rankedOsqueryTable struct {
	table OsqueryTable
	score int
}

type osquerySchemaSnapshot struct {
	tables []OsqueryTable
}

var (
	//go:embed osquery_fleet_schema.json
	osquerySchemaJSON []byte

	osquerySchemaState atomic.Pointer[osquerySchemaSnapshot]

	nonSearchCharacter = regexp.MustCompile(`[^a-z0-9]+`)
	searchStopWords    = map[string]struct{}{
		"a": {}, "an": {}, "and": {}, "are": {}, "for": {}, "from": {},
		"get": {}, "how": {}, "in": {}, "is": {}, "of": {}, "on": {},
		"show": {}, "the": {}, "to": {}, "with": {},
	}
)

func init() {
	tables, err := parseOsquerySchemaJSON(osquerySchemaJSON)
	if err != nil {
		panic(fmt.Sprintf("parse embedded osquery schema: %v", err))
	}
	storeOsqueryTables(tables)
}

// SearchOsqueryTables returns the canonical table definitions most relevant to a phrase.
func SearchOsqueryTables(query, platform string, limit int) ([]OsqueryTable, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query must not be empty")
	}
	if len(query) > maxOsquerySearchQueryBytes {
		return nil, fmt.Errorf("query must not exceed %d bytes", maxOsquerySearchQueryBytes)
	}

	platform, err := NormalizeOsqueryPlatform(platform)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > MaxOsquerySearchLimit {
		return nil, fmt.Errorf("limit must be between 1 and %d", MaxOsquerySearchLimit)
	}

	tables, err := currentOsqueryTables()
	if err != nil {
		return nil, err
	}

	normalizedQuery := normalizeSearchText(query)
	terms := searchTerms(normalizedQuery, platform)
	if len(terms) == 0 {
		return nil, errors.New("query must contain a searchable term")
	}
	if len(terms) > maxOsquerySearchTerms {
		return nil, fmt.Errorf("query must not contain more than %d searchable terms", maxOsquerySearchTerms)
	}

	platformTables := slices.DeleteFunc(slices.Clone(tables), func(table OsqueryTable) bool {
		return !osqueryTableSupportsPlatform(table, platform)
	})
	return rankOsqueryTables(platformTables, terms, normalizedQuery, limit), nil
}

func parseOsquerySchemaJSON(data []byte) ([]OsqueryTable, error) {
	var tables []OsqueryTable
	if err := json.Unmarshal(data, &tables); err != nil {
		return nil, fmt.Errorf("parse osquery schema: %w", err)
	}
	if len(tables) == 0 {
		return nil, errors.New("osquery schema contains no tables")
	}

	tableNames := make(map[string]struct{}, len(tables))
	for tableIndex, table := range tables {
		tableName := strings.TrimSpace(table.Name)
		if tableName == "" {
			return nil, fmt.Errorf("osquery schema table %d has no name", tableIndex)
		}
		normalizedTableName := strings.ToLower(tableName)
		if _, ok := tableNames[normalizedTableName]; ok {
			return nil, fmt.Errorf("osquery schema contains duplicate table %q", tableName)
		}
		tableNames[normalizedTableName] = struct{}{}
		if len(table.Columns) == 0 {
			return nil, fmt.Errorf("osquery schema table %q has no columns", tableName)
		}

		columnNames := make(map[string]struct{}, len(table.Columns))
		for columnIndex, column := range table.Columns {
			columnName := strings.TrimSpace(column.Name)
			if columnName == "" {
				return nil, fmt.Errorf("osquery schema table %q column %d has no name", tableName, columnIndex)
			}
			if strings.TrimSpace(column.Type) == "" {
				return nil, fmt.Errorf("osquery schema table %q column %q has no type", tableName, columnName)
			}
			normalizedColumnName := strings.ToLower(columnName)
			if _, ok := columnNames[normalizedColumnName]; ok {
				return nil, fmt.Errorf("osquery schema table %q contains duplicate column %q", tableName, columnName)
			}
			columnNames[normalizedColumnName] = struct{}{}
		}
	}
	return tables, nil
}

func currentOsqueryTables() ([]OsqueryTable, error) {
	snapshot := osquerySchemaState.Load()
	if snapshot == nil || len(snapshot.tables) == 0 {
		return nil, errors.New("osquery schema is not loaded")
	}
	return snapshot.tables, nil
}

func storeOsqueryTables(tables []OsqueryTable) {
	osquerySchemaState.Store(&osquerySchemaSnapshot{tables: tables})
}

// NormalizeOsqueryPlatform validates Fleet's public platform aliases.
func NormalizeOsqueryPlatform(platform string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "", "all":
		return "all", nil
	case "darwin", "macos":
		return "darwin", nil
	case "windows", "linux":
		return strings.ToLower(strings.TrimSpace(platform)), nil
	case "chrome", "chromeos":
		return "chrome", nil
	default:
		return "", fmt.Errorf("unsupported platform %q", platform)
	}
}

func searchTerms(normalizedQuery, platform string) []string {
	terms := strings.Fields(normalizedQuery)
	terms = slices.DeleteFunc(terms, func(term string) bool {
		_, stopWord := searchStopWords[term]
		return len(term) < 2 || stopWord || term == platform || (platform == "darwin" && term == "macos") || (platform == "chrome" && term == "chromeos")
	})

	seen := make(map[string]struct{}, len(terms))
	return slices.DeleteFunc(terms, func(term string) bool {
		if _, ok := seen[term]; ok {
			return true
		}
		seen[term] = struct{}{}
		return false
	})
}

func osqueryTableSupportsPlatform(table OsqueryTable, platform string) bool {
	if platform == "all" || len(table.Platforms) == 0 {
		return true
	}
	return slices.ContainsFunc(table.Platforms, func(tablePlatform string) bool {
		normalized, err := NormalizeOsqueryPlatform(tablePlatform)
		return err == nil && normalized == platform
	})
}

func rankOsqueryTables(tables []OsqueryTable, terms []string, normalizedQuery string, limit int) []OsqueryTable {
	ranked := make([]rankedOsqueryTable, 0, len(tables))
	for _, table := range tables {
		score := osqueryTableSearchScore(table, terms, normalizedQuery)
		if score > 0 {
			ranked = append(ranked, rankedOsqueryTable{table: table, score: score})
		}
	}

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].table.Name < ranked[j].table.Name
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	results := make([]OsqueryTable, 0, len(ranked))
	for _, result := range ranked {
		results = append(results, result.table)
	}
	return results
}

func osqueryTableSearchScore(table OsqueryTable, terms []string, normalizedQuery string) int {
	tableName := normalizeSearchText(table.Name)
	score := phraseScore(tableName, normalizedQuery, 1_000, 500)
	score += phraseScore(normalizeSearchText(table.Description), normalizedQuery, 0, 100)

	for _, term := range terms {
		score += termScore(tableName, term, 120, 70)
		score += containsScore(normalizeSearchText(table.Description), term, 20)
		score += containsScore(normalizeNullableSearchText(table.Examples), term, 8)
		score += containsScore(normalizeNullableSearchText(table.Notes), term, 5)
		for _, column := range table.Columns {
			score += termScore(normalizeSearchText(column.Name), term, 100, 50)
			score += containsScore(normalizeSearchText(column.Description), term, 15)
			score += containsScore(normalizeNullableSearchText(column.Notes), term, 5)
		}
	}
	return score
}

func phraseScore(text, phrase string, exactScore, containsScore int) int {
	if phrase == "" {
		return 0
	}
	if text == phrase {
		return exactScore
	}
	if strings.Contains(text, phrase) {
		return containsScore
	}
	return 0
}

func termScore(text, term string, exactScore, containsWeight int) int {
	if text == term {
		return exactScore
	}
	for _, word := range strings.Fields(text) {
		if word == term {
			return exactScore
		}
	}
	return containsScore(text, term, containsWeight)
}

func containsScore(text, term string, score int) int {
	if strings.Contains(text, term) {
		return score
	}
	return 0
}

func normalizeNullableSearchText(value *string) string {
	if value == nil {
		return ""
	}
	return normalizeSearchText(*value)
}

func normalizeSearchText(value string) string {
	return strings.TrimSpace(nonSearchCharacter.ReplaceAllString(strings.ToLower(value), " "))
}
