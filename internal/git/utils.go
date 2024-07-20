package git

import (
	"fmt"
	"strings"

	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
)

func repoStoragePath(conf *schema.TediumConfig, r *schema.Repo) string {
	// "domain/repo" and "domain/repo.git" clone the same repo, so remove the extension to avoid storing them at different paths
	url := strings.TrimSuffix(r.CloneUrl, ".git")
	return fmt.Sprintf(
		"%s/repos/%s",
		conf.RepoStoragePath,
		utils.Sha256String(url),
	)
}
