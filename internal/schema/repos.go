package schema

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Repo represents a real Git repo, which may be either a remote repo from which chores or config are read, or a target repo cloned to disk.
type Repo struct {
	// present for all repos
	Domain    string
	OwnerName string
	Name      string

	// present for target repos only
	CloneUrl      string
	Auth          RepoAuth
	DefaultBranch string
	Archived      bool
}

type RepoAuth struct {
	Username string
	Password string
}

func (r *Repo) FullName() string {
	return fmt.Sprintf("%s/%s", r.OwnerName, r.Name)
}

func (ra *RepoAuth) ToTransportAuth() transport.AuthMethod {
	return &http.BasicAuth{
		Username: ra.Username,
		Password: ra.Password,
	}
}

func RepoFromUrl(repoUrl string) (Repo, error) {
	urlParsed, err := url.Parse(repoUrl)
	if err != nil {
		return Repo{}, fmt.Errorf("error parsing repo URL: %w", err)
	}

	domain := urlParsed.Host
	path := strings.Trim(urlParsed.Path, "/")
	pathSegments := strings.Split(path, "/")
	if len(pathSegments) != 2 {
		return Repo{}, fmt.Errorf("error parsing repo URL: path does not have two segments")
	}

	return Repo{
		Domain:    domain,
		OwnerName: pathSegments[0],
		Name:      strings.TrimSuffix(pathSegments[1], ".git"),
	}, nil
}
