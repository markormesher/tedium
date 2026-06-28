package platforms

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	urllib "net/url"

	"github.com/go-resty/resty/v2"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
)

type GiteaPlatform struct {
	schema.PlatformConfig

	// supplied via config
	baseURLs []*urllib.URL
	auth     *schema.AuthConfig

	// generated locally
	apiBaseUrl *urllib.URL
	profile    schema.PlatformProfile
}

func giteaPlatformFromConfig(conf schema.TediumConfig, platformConfig schema.PlatformConfig) (*GiteaPlatform, error) {
	if platformConfig.Auth != nil && platformConfig.Auth.Type != schema.AuthConfigTypeUserToken {
		return nil, fmt.Errorf("cannot construct Gitea platform with auth type other than user token (platform: %s)", platformConfig.BaseURL)
	}

	p := GiteaPlatform{
		PlatformConfig: platformConfig,
		auth:           platformConfig.Auth,
	}

	// normalise primary base URL
	urlParsed, err := urllib.Parse(platformConfig.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	p.baseURLs = []*urllib.URL{urlParsed}

	// generate API URL
	p.apiBaseUrl = urlParsed.JoinPath("/api/v1")

	// normalise alternate base URLs
	for _, u := range platformConfig.AlternateBaseURLs {
		urlParsed, err := urllib.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("invalid alternate base URL: %w", err)
		}
		p.baseURLs = append(p.baseURLs, urlParsed)
	}

	return &p, nil
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

func (p *GiteaPlatform) ApiBaseUrl() *urllib.URL {
	return p.apiBaseUrl
}

func (p *GiteaPlatform) AcceptsURL(url string) (string, bool) {
	urlParsed, err := urllib.Parse(url)
	if err != nil {
		return "", false
	}

	for _, baseURL := range p.baseURLs {
		if urlParsed.Scheme == baseURL.Scheme && urlParsed.Host == baseURL.Host {
			urlParsed.Scheme = p.baseURLs[0].Scheme
			urlParsed.Host = p.baseURLs[0].Host
			return urlParsed.String(), true
		}
	}

	return "", false
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
		slog.Warn("no auth configured for platform; skipping repo discovery", "baseURL", p.baseURLs[0])
		return []schema.Repo{}, nil
	}

	var repoData struct {
		Data []struct {
			Name          string `json:"name"`
			CloneUrl      string `json:"clone_url"`
			DefaultBranch string `json:"default_branch"`
			Archived      bool   `json:"archived"`
			Mirror        bool   `json:"mirror"`
			Owner         struct {
				Username string `json:"username"`
			} `json:"owner"`
		} `json:"data"`
	}

	var output []schema.Repo
	url := fmt.Sprintf("%s/repos/search?page=1&limit=50", p.apiBaseUrl)

	for {
		_, req := p.authedRequest()
		req.SetResult(&repoData)

		response, err := req.Get(url)
		if err != nil {
			return nil, fmt.Errorf("error making Gitea API request: %v", err)
		}

		if response.IsError() {
			return nil, fmt.Errorf("error making Gitea API request, status: %v", response.Status())
		}

		for _, repo := range repoData.Data {
			cloneURL, ok := p.AcceptsURL(repo.CloneUrl)
			if !ok {
				return nil, fmt.Errorf("platform returned a repo with an unaccepted clone URL: %s", repo.CloneUrl)
			}

			output = append(output, schema.Repo{
				OwnerName: repo.Owner.Username,
				Name:      repo.Name,

				CloneUrl: cloneURL,
				Auth: schema.RepoAuth{
					// TODO: don't forget to set this properly when app auth is supported
					Username: "x-access-token",
					Password: p.auth.Token,
				},
				DefaultBranch: repo.DefaultBranch,
				Archived:      repo.Archived,
				Mirror:        repo.Mirror,
			})
		}

		linkHeaders := utils.ParseLinkHeader(response.Header().Get("link"))
		if nextLink, ok := linkHeaders["next"]; ok {
			url = nextLink
		} else {
			break
		}
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
		url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", p.apiBaseUrl, repo.OwnerName, repo.Name, path)
		response, err := req.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to read file via Gitea API: %w", err)
		}

		if response.StatusCode() == 404 {
			// no match for this candidate, but there may be others
			continue
		}

		// TODO: handle non-200 statuses

		fileBytes, err := base64.StdEncoding.DecodeString(repoFile.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 string: %w", err)
		}

		return fileBytes, nil
	}

	// no result for any path candidate
	return nil, nil
}

func (p *GiteaPlatform) OpenOrUpdatePullRequest(job schema.Job) error {
	slog.Info("Opening or updating PR", "chore", job.Chore.Name)

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
		slog.Debug("Opening PR")
		response, err = req.Post(fmt.Sprintf("%s/repos/%s/%s/pulls", p.apiBaseUrl, job.Repo.OwnerName, job.Repo.Name))
	} else {
		slog.Debug("Updating PR")
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
