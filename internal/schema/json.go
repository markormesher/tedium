package schema

type JsonPatchOperation struct {
	Operation string `json:"op" yaml:"op"`
	Path      string `json:"path" yaml:"path"`
	Value     string `json:"value" yaml:"value"`
}

type JsonPatch []JsonPatchOperation
