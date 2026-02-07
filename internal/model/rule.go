package model

type Rule struct {
	Type      string // e.g. "DOMAIN-SUFFIX", "IP-CIDR", "MATCH"
	Value     string // domain/suffix/keyword/cidr/cc
	Action    string // DIRECT/REJECT/group name
	NoResolve bool   // only meaningful for IP-CIDR/IP-CIDR6
}
