package model

const (
	MemberRefProxy   = "proxy"
	MemberRefGroup   = "group"
	MemberRefBuiltin = "builtin"
)

type MemberRef struct {
	Kind  string
	Value string
}

type Group struct {
	Name string
	Type string // "select" | "url-test"

	Members []MemberRef

	// url-test only
	TestURL     string
	IntervalSec int

	ToleranceMS  int
	HasTolerance bool
}
