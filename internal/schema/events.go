package schema

// Event is used to relay progress between different goroutines.
type Event int

const (
	RepoDiscovered Event = iota
	RepoFailed
	RepoSkipped
	DiscoveryFinished

	JobDiscovered
	JobSucceeded
	JobFailed
)
