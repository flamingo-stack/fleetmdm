package schema

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearchOsqueryTablesRanksExactTableAndReturnsColumnTypes(t *testing.T) {
	tables, err := SearchOsqueryTables("windows update result code", "windows", 5)
	require.NoError(t, err)
	require.NotEmpty(t, tables)
	require.Equal(t, "windows_update_history", tables[0].Name)

	var resultCode *OsqueryColumn
	for i := range tables[0].Columns {
		if tables[0].Columns[i].Name == "result_code" {
			resultCode = &tables[0].Columns[i]
			break
		}
	}
	require.NotNil(t, resultCode)
	require.Equal(t, "text", resultCode.Type)
}

func TestSearchOsqueryTablesFiltersByPlatform(t *testing.T) {
	tables, err := SearchOsqueryTables("windows update history", "darwin", 20)
	require.NoError(t, err)
	for _, table := range tables {
		require.NotEqual(t, "windows_update_history", table.Name)
	}
}

func TestSearchOsqueryTablesPreservesNullableCanonicalFields(t *testing.T) {
	tables, err := SearchOsqueryTables("acpi tables", "darwin", 1)
	require.NoError(t, err)
	require.Len(t, tables, 1)
	require.Equal(t, "acpi_tables", tables[0].Name)
	require.Nil(t, tables[0].Examples)

	tables, err = SearchOsqueryTables("adobe plugins", "darwin", 1)
	require.NoError(t, err)
	require.Len(t, tables, 1)
	require.Equal(t, "adobe_plugins", tables[0].Name)
	require.NotEmpty(t, tables[0].Columns)
	require.Nil(t, tables[0].Columns[0].Hidden)
}

func TestSearchOsqueryTablesPreservesColumnPlatforms(t *testing.T) {
	tables, err := SearchOsqueryTables("battery", "darwin", 1)
	require.NoError(t, err)
	require.Len(t, tables, 1)
	require.Equal(t, "battery", tables[0].Name)

	var healthColumn *OsqueryColumn
	for i := range tables[0].Columns {
		if tables[0].Columns[i].Name == "health" {
			healthColumn = &tables[0].Columns[i]
			break
		}
	}
	require.NotNil(t, healthColumn)
	require.Equal(t, []string{"macOS"}, healthColumn.Platforms)
}

func TestRankOsqueryTablesUsesNameOrderForEqualScores(t *testing.T) {
	tables := []OsqueryTable{
		{Name: "z_table", Description: "firmware details"},
		{Name: "a_table", Description: "firmware details"},
	}

	ranked := rankOsqueryTables(tables, []string{"firmware"}, "firmware", 2)
	require.Equal(t, []string{"a_table", "z_table"}, []string{ranked[0].Name, ranked[1].Name})
}

func TestSearchOsqueryTablesValidatesInput(t *testing.T) {
	_, err := SearchOsqueryTables("   ", "all", 20)
	require.ErrorContains(t, err, "query must not be empty")

	_, err = SearchOsqueryTables("updates", "freebsd", 20)
	require.ErrorContains(t, err, "unsupported platform")

	_, err = SearchOsqueryTables("updates", "windows", 51)
	require.ErrorContains(t, err, "limit must be between 1 and 50")

	_, err = SearchOsqueryTables(strings.Repeat("x", maxOsquerySearchQueryBytes+1), "all", 20)
	require.ErrorContains(t, err, "query must not exceed 512 bytes")

	terms := make([]string, maxOsquerySearchTerms+1)
	for i := range terms {
		terms[i] = fmt.Sprintf("term%d", i)
	}
	_, err = SearchOsqueryTables(strings.Join(terms, " "), "all", 20)
	require.ErrorContains(t, err, "query must not contain more than 32 searchable terms")
}

func TestNormalizeOsqueryPlatformAliases(t *testing.T) {
	tests := []struct {
		platform string
		expected string
	}{
		{platform: "macos", expected: "darwin"},
		{platform: "chromeos", expected: "chrome"},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			platform, err := NormalizeOsqueryPlatform(tt.platform)
			require.NoError(t, err)
			require.Equal(t, tt.expected, platform)
		})
	}
}

func TestSearchTermsDeduplicatesTerms(t *testing.T) {
	require.Equal(t, []string{"processes", "ports"}, searchTerms("processes processes ports processes", "all"))
}
