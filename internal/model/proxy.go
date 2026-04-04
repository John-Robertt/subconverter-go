package model

type KV struct {
	Key   string
	Value string
}

// Proxy is the minimal node representation used by the compiler pipeline.
// v1 only produces Type=="ss".
type Proxy struct {
	Type string

	// Name comes from subscription fragment (#name). It may be empty and is not
	// guaranteed to be globally unique; the compiler phase will normalize and
	// deduplicate names.
	Name string

	Server   string
	Port     int
	Cipher   string
	Password string

	// PluginName/PluginOpts come from the "plugin" query parameter in ss://.
	// PluginOpts must preserve order (no map) to keep behavior deterministic.
	PluginName string
	PluginOpts []KV
}
