// Package jobnames holds job-name string constants shared between the
// server/worker package and packages (such as server/fleet) that cannot
// import server/worker directly without risking an import cycle. This
// package has no dependencies, so it is safely importable from both sides.
package jobnames

const (
	ChartScrubDatasetGlobalJobName = "chart_scrub_dataset_global"
	ChartScrubDatasetFleetJobName  = "chart_scrub_dataset_fleet"
)
