package utils

// this horrible hack is needed because some of the Podman bindings expect *bool as the argument type

func BoolPtr(value bool) *bool {
	return &value
}
