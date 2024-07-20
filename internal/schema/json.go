package schema

type JsonPatchOperation struct {
	Operation string `json:"op"`
	Path      string `json:"path"`
	Value     string `json:"value"`
}

type JsonPatch []JsonPatchOperation
