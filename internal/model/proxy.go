package model

type KV struct {
	Key   string
	Value string
}

// Proxy is the minimal node representation used by the compiler pipeline.
// Subscription input still only produces Type=="ss", while profile-defined
// custom proxies may use additional outbound types.
type Proxy struct {
	ID     string
	Source string

	// MatchName is used for regex-based selection before final display-name
	// disambiguation. It is not required to be globally unique.
	MatchName string

	Type string

	// Name comes from subscription fragment (#name). It may be empty and is not
	// guaranteed to be globally unique; the compiler phase will turn it into the
	// final display name.
	Name string

	Server   string
	Port     int
	Username string
	Cipher   string
	Password string

	// PluginName/PluginOpts come from the "plugin" query parameter in ss://.
	// PluginOpts must preserve order (no map) to keep behavior deterministic.
	PluginName string
	PluginOpts []KV

	// ViaProxyID points to the chain exit proxy for subscription proxies.
	ViaProxyID string
}
