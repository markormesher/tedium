package platforms

import (
	"encoding/base64"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
)

type GiteaPlatform struct {
	schema.PlatformConfig

	// supplied via config
	domain string
	auth   *schema.AuthConfig

	// generated locally
	apiBaseUrl string
	profile    schema.PlatformProfile
}

func giteaPlatformFromConfig(conf schema.TediumConfig, platformConfig schema.PlatformConfig) (*GiteaPlatform, error) {
	if platformConfig.Auth != nil && platformConfig.Auth.Type != schema.AuthConfigTypeUserToken {
		return nil, fmt.Errorf("cannot construct Gitea platform with auth type other than user token (domain: %s)", platformConfig.Domain)
	}

	return &GiteaPlatform{
		PlatformConfig: platformConfig,

		domain:     platformConfig.Domain,
		auth:       platformConfig.Auth,
		apiBaseUrl: fmt.Sprintf("https://%s/api/v1", platformConfig.Domain),
	}, nil
}

// interface methods

func (p *GiteaPlatform) Init(conf schema.TediumConfig) error {
	err := p.loadProfile(conf)
	if err != nil {
		return err
	}

	return nil
}

func (p *GiteaPlatform) Deinit() error {
	return nil
}

func (p *GiteaPlatform) Config() schema.PlatformConfig {
	return p.PlatformConfig
}

func (p *GiteaPlatform) ApiBaseUrl() string {
	return p.apiBaseUrl
}

func (p *GiteaPlatform) AcceptsDomain(domain string) bool {
	return domain == p.domain
}

func (p *GiteaPlatform) Profile() schema.PlatformProfile {
	return p.profile
}

func (p *GiteaPlatform) AuthToken() string {
	if p.auth == nil {
		return ""
	}

	return p.auth.Token
}

func (p *GiteaPlatform) DiscoverRepos() ([]schema.Repo, error) {
	if p.auth == nil {
		l.Warn("No auth configured for paltform; skipping repo discovery", "domain", p.domain)
		return []schema.Repo{}, nil
	}

	var repoData struct {
		Data []struct {
			Name          string `json:"name"`
			CloneUrl      string `json:"clone_url"`
			DefaultBranch string `json:"default_branch"`
			Archived      bool   `json:"archived"`
			Owner         struct {
				Username string `json:"username"`
			} `json:"owner"`
		} `json:"data"`
	}

	_, req := p.authedRequest()
	req.SetResult(&repoData)
	req.SetQueryParams(map[string]string{
		"limit": "100",
	})

	response, err := req.Get(fmt.Sprintf("%s/repos/search", p.apiBaseUrl))

	if err != nil {
		return nil, fmt.Errorf("error making Gitea API request: %v", err)
	}

	if response.IsError() {
		return nil, fmt.Errorf("error making Gitea API request, status: %v", response.Status())
	}

	var output []schema.Repo
	for _, repo := range repoData.Data {
		output = append(output, schema.Repo{
			Domain:    p.domain,
			OwnerName: repo.Owner.Username,
			Name:      repo.Name,

			CloneUrl: repo.CloneUrl,
			Auth: schema.RepoAuth{
				// TODO: don't forget to set this properly when app auth is supported
				Username: "x-access-token",
				Password: p.auth.Token,
			},
			DefaultBranch: repo.DefaultBranch,
			Archived:      repo.Archived,
		})
	}

	return output, nil
}

func (p *GiteaPlatform) RepoHasTediumConfig(repo schema.Repo) (bool, error) {
	file, err := p.ReadRepoFile(repo, "", utils.AddYamlJsonExtensions(".tedium"))

	if err != nil {
		return false, fmt.Errorf("failed to read Tedium file via Gitea API: %w", err)
	}

	return file != nil, nil
}

func (p *GiteaPlatform) ReadRepoFile(repo schema.Repo, branch string, pathCandidates []string) ([]byte, error) {
	var repoFile struct {
		Content string `json:"content"`
	}

	for _, path := range pathCandidates {
		_, req := p.authedRequest()

		if branch != "" {
			req.SetQueryParam("ref", branch)
		}

		req.SetResult(&repoFile)
		response, err := req.Get(fmt.Sprintf("%s/repos/%s/%s/contents/%s", p.apiBaseUrl, repo.OwnerName, repo.Name, path))
		if err != nil {
			return nil, fmt.Errorf("failed to read file via Gitea API: %w", err)
		}

		if response.StatusCode() == 404 {
			// no match for this candidate, but there may be others
			continue
		}

		fileStr, err := base64.StdEncoding.DecodeString(repoFile.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 string: %w", err)
		}

		return fileStr, nil
	}

	// no result for any path candidate
	return nil, nil
}

func (p *GiteaPlatform) OpenOrUpdatePullRequest(job schema.Job) error {
	l.Info("Opening or updating PR", "chore", job.Chore.Name)

	var existingPrs []struct {
		Num   int    `json:"number"`
		State string `json:"state"`

		Base struct {
			// TODO: for GitHub these labels are "owner:branch" not just "branch" - are they the same here sometimes?
			Label string `json:"label"`
		} `json:"base"`
		Head struct {
			Label string `json:"label"`
		} `json:"head"`
	}

	_, req := p.authedRequest()
	req.SetResult(&existingPrs)
	response, err := req.Get(fmt.Sprintf("%s/repos/%s/%s/pulls", p.apiBaseUrl, job.Repo.OwnerName, job.Repo.Name))
	if err != nil {
		return fmt.Errorf("error fetching existing PRs: %w", err)
	}

	if !response.IsSuccess() {
		return fmt.Errorf("error fetching existing PRs: %v", string(response.Body()))
	}

	var existingPrNum int
	for _, pr := range existingPrs {
		if pr.Base.Label == job.Repo.DefaultBranch && pr.Head.Label == job.FinalBranchName && pr.State == "open" {
			existingPrNum = pr.Num
			break
		}
	}

	prBody := map[string]interface{}{
		"base":  job.Repo.DefaultBranch,
		"head":  job.FinalBranchName,
		"title": job.Chore.PrTitle(),
		"body":  job.Chore.PrBody(),
	}

	_, req = p.authedRequest()
	req.SetHeader("Content-type", "application/json")
	req.SetBody(prBody)

	if existingPrNum == 0 {
		l.Debug("Opening PR")
		response, err = req.Post(fmt.Sprintf("%s/repos/%s/%s/pulls", p.apiBaseUrl, job.Repo.OwnerName, job.Repo.Name))
	} else {
		l.Debug("Updating PR")
		response, err = req.Patch(fmt.Sprintf("%s/repos/%s/%s/pulls/%d", p.apiBaseUrl, job.Repo.OwnerName, job.Repo.Name, existingPrNum))
	}

	if err != nil {
		return fmt.Errorf("error opening or updating PR: %w", err)
	}

	if !response.IsSuccess() {
		return fmt.Errorf("error opening or updating PR: %v", string(response.Body()))
	}

	return nil
}

// internal methods

func (p *GiteaPlatform) loadProfile(conf schema.TediumConfig) error {
	if p.auth == nil || p.SkipDiscovery {
		return nil
	}

	var user struct {
		Email string `json:"email"`
	}

	_, req := p.authedRequest()
	req.SetResult(&user)
	response, err := req.Get(fmt.Sprintf("%s/user", p.apiBaseUrl))

	if err != nil {
		return fmt.Errorf("failed to load user profile: %v", err)
	}

	if response.IsError() {
		return fmt.Errorf("failed to load user profile, status: %v", response.Status())
	}

	p.profile = schema.PlatformProfile{
		Email: user.Email,
	}

	return nil
}

func (p *GiteaPlatform) authedRequest() (*resty.Client, *resty.Request) {
	client := resty.New()
	request := client.NewRequest()

	if p.auth == nil {
		return client, request
	}

	if p.auth.Type == schema.AuthConfigTypeUserToken {
		request.SetHeader("Authorization", fmt.Sprintf("token %s", p.auth.Token))
	}

	if p.auth.Type == schema.AuthConfigTypeApp {
		// TODO: support app auth for Gitea
		panic("Not supported yet")
	}

	return client, request
}
