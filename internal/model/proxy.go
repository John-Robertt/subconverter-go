package model

type KV struct {
	Key   string
	Value string
}

// Proxy is the minimal node representation used by the compiler pipeline.
// Compiled output may contain both subscription proxies and chain-derived proxies.
type Proxy struct {
	// ID is the stable internal identity of a compiled proxy. Parsers do not
	// need to fill it; the compiler derives it from the normalized proxy fields.
	ID string

	Type string

	// Name comes from subscription fragment (#name). It may be empty and is not
	// guaranteed to be globally unique; the compiler phase will normalize and
	// deduplicate names.
	Name string

	Server   string
	Port     int
	Username string
	Cipher   string
	Password string
	// ViaProxyID points to the subscription proxy used to access a derived proxy.
	// Empty means this proxy is a direct subscription proxy.
	ViaProxyID string

	// PluginName/PluginOpts come from the "plugin" query parameter in ss://.
	// PluginOpts must preserve order (no map) to keep behavior deterministic.
	PluginName string
	PluginOpts []KV
}
