package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

const (
	batchSize    = 1000
	totalRecords = 1000000
)

func getEnvOrDefault(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

func main() {
	// MySQL connection details, overridable via environment variables for local dev
	user := getEnvOrDefault("SEED_MYSQL_USER", "fleet")
	password := getEnvOrDefault("SEED_MYSQL_PASSWORD", "insecure")
	host := getEnvOrDefault("SEED_MYSQL_HOST", "localhost") // Assuming you are running this script on the same host as Docker
	port := getEnvOrDefault("SEED_MYSQL_PORT", "3306")
	database := getEnvOrDefault("SEED_MYSQL_DATABASE", "fleet")

	// Construct the MySQL DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, password, host, port, database)

	// Open MySQL connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Disable foreign key checks to improve performance
	_, err = db.Exec("SET FOREIGN_KEY_CHECKS=0")
	if err != nil {
		log.Fatal(err) //nolint:gocritic // ignore exitAfterDefer
	}

	// Prepare the insert statement
	stmtPrefix := "INSERT INTO `queries` (`saved`, `name`, `description`, `query`, `author_id`, `observer_can_run`, `team_id`, `team_id_char`, `platform`, `min_osquery_version`, `schedule_interval`, `automations_enabled`, `logging_type`, `discard_data`) VALUES "
	stmtSuffix := ";"

	// Insert records in batches
	for batch := 0; batch < totalRecords/batchSize; batch++ {
		var valueStrings []string
		var valueArgs []interface{}

		// Generate batch of 1000 records
		for i := 0; i < batchSize; i++ {
			queryID := batch*batchSize + i + 1
			valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
			valueArgs = append(valueArgs, 0, fmt.Sprintf("query_%d", queryID), "", "SELECT * FROM processes;", 1, 0, nil, "", "", "", 0, 0, "snapshot", 0)
		}

		// Construct and execute the batch insert
		stmt := stmtPrefix + strings.Join(valueStrings, ",") + stmtSuffix
		_, err := db.Exec(stmt, valueArgs...)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Inserted batch %d/%d\n", batch+1, totalRecords/batchSize)
	}

	// Re-enable foreign key checks
	_, err = db.Exec("SET FOREIGN_KEY_CHECKS=1")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Finished inserting 1 million records.")
}
