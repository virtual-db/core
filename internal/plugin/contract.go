package plugin

// Manifest is the parsed content of a plugin's manifest.json / manifest.yaml file.
type Manifest struct {
	Name    string            `json:"name"    yaml:"name"`
	Version string            `json:"version" yaml:"version"`
	Command []string          `json:"command" yaml:"command"`
	Env     map[string]string `json:"env"     yaml:"env"`
}

// DeclareParams is the payload of the "declare" JSON-RPC 2.0 notification that a
// plugin sends immediately after connecting.
type DeclareParams struct {
	PluginID             string         `json:"plugin_id"`
	PipelineHandlers     []HandlerEntry `json:"pipeline_handlers"`
	EventSubscriptions   []string       `json:"event_subscriptions"`
	EventDeclarations    []string       `json:"event_declarations"`
	PipelineDeclarations []PipelineDecl `json:"pipeline_declarations"`
}

// HandlerEntry describes one pipeline point a plugin handles.
type HandlerEntry struct {
	Point    string `json:"point"`
	Priority int    `json:"priority"`
}

// PipelineDecl is a plugin-owned pipeline declaration.
type PipelineDecl struct {
	Name        string   `json:"name"        yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Points      []string `json:"points"      yaml:"points"`
}
