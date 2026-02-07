package render

// AllowedRuleTypes returns the rule TYPE allow-list for the target.
//
// NOTE: This is used at parse/compile time to fail fast with source URL/line
// information, instead of producing a config that the client cannot import.
func AllowedRuleTypes(target Target) map[string]struct{} {
	switch target {
	case TargetClash:
		return map[string]struct{}{
			"DOMAIN":         {},
			"DOMAIN-SUFFIX":  {},
			"DOMAIN-KEYWORD": {},
			"IP-CIDR":        {},
			"IP-CIDR6":       {},
			"GEOIP":          {},
			"PROCESS-NAME":   {},
			// Widely used in popular Clash rulesets (e.g. ACL4SSR).
			"URL-REGEX": {},
			"MATCH":          {},
		}
	case TargetSurge, TargetShadowrocket:
		// v1: Surge-like targets support a superset (e.g. URL-REGEX).
		return map[string]struct{}{
			"DOMAIN":         {},
			"DOMAIN-SUFFIX":  {},
			"DOMAIN-KEYWORD": {},
			"IP-CIDR":        {},
			"IP-CIDR6":       {},
			"GEOIP":          {},
			"PROCESS-NAME":   {},
			"URL-REGEX":      {},
			"MATCH":          {},
		}
	case TargetQuanx:
		return map[string]struct{}{
			"DOMAIN":         {},
			"DOMAIN-SUFFIX":  {},
			"DOMAIN-KEYWORD": {},
			"IP-CIDR":        {},
			"IP-CIDR6":       {},
			"GEOIP":          {},
			"PROCESS-NAME":   {},
			"URL-REGEX":      {},
			"MATCH":          {},
		}
	default:
		// Unknown target: let later validation handle it. Returning nil means "no extra restriction".
		return nil
	}
}
