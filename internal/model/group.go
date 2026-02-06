package model

type Group struct {
	Name string
	Type string // "select" | "url-test"

	Members []string // proxy names / group names / DIRECT / REJECT

	// url-test only
	TestURL     string
	IntervalSec int

	ToleranceMS  int
	HasTolerance bool
}
