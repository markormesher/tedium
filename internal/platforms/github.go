package platforms

import (
	"encoding/base64"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
)

type GitHubPlatform struct {
	schema.PlatformConfig

	// supplied via config
	domain string
	auth   *schema.AuthConfig

	// generated locally
	apiBaseUrl string
	profile    schema.PlatformProfile
}

func githubPlatformFromConfig(conf schema.TediumConfig, platformConfig schema.PlatformConfig) (*GitHubPlatform, error) {
	return &GitHubPlatform{
		PlatformConfig: platformConfig,

		domain:     platformConfig.Domain,
		auth:       platformConfig.Auth,
		apiBaseUrl: fmt.Sprintf("https://api.%s", platformConfig.Domain),
	}, nil
}

// interface methods

func (p *GitHubPlatform) Init(conf schema.TediumConfig) error {
	err := p.loadProfile(conf)
	if err != nil {
		return err
	}

	return nil
}

func (p *GitHubPlatform) Deinit() error {
	return nil
}

func (p *GitHubPlatform) Config() schema.PlatformConfig {
	return p.PlatformConfig
}

func (p *GitHubPlatform) ApiBaseUrl() string {
	return p.apiBaseUrl
}

func (p *GitHubPlatform) AcceptsDomain(domain string) bool {
	return domain == p.domain
}

func (p *GitHubPlatform) Profile() schema.PlatformProfile {
	return p.profile
}

func (p *GitHubPlatform) AuthToken() string {
	if p.auth == nil {
		return ""
	}

	switch p.auth.Type {
	case schema.AuthConfigTypeUserToken:
		return p.auth.Token

	case schema.AuthConfigTypeApp:
		return p.auth.AppInstallationToken

	default:
		return ""
	}
}

func (p *GitHubPlatform) DiscoverRepos() ([]schema.Repo, error) {
	if p.auth == nil {
		l.Warn("No auth configured for paltform; skipping repo discovery", "domain", p.domain)
		return []schema.Repo{}, nil
	}

	switch p.auth.Type {
	case schema.AuthConfigTypeUserToken:
		var repoData []struct {
			Name          string `json:"name"`
			CloneUrl      string `json:"clone_url"`
			DefaultBranch string `json:"default_branch"`
			Archived      bool   `json:"archived"`
			Owner         struct {
				Username string `json:"login"`
			} `json:"owner"`
		}

		_, req, err := p.authedUserRequest()
		if err != nil {
			return nil, fmt.Errorf("error making GitHub API request: %w", err)
		}

		req.SetResult(&repoData)
		response, err := req.Get(fmt.Sprintf("%s/user/repos?per_page=100", p.apiBaseUrl))

		if err != nil {
			return nil, fmt.Errorf("error making GitHub API request: %w", err)
		}

		if response.IsError() {
			return nil, fmt.Errorf("error making GitHub API request, status: %v", response.Status())
		}

		var output []schema.Repo
		for _, repo := range repoData {
			output = append(output, schema.Repo{
				Domain:    p.domain,
				OwnerName: repo.Owner.Username,
				Name:      repo.Name,

				CloneUrl: repo.CloneUrl,
				Auth: schema.RepoAuth{
					Username: "x-access-token",
					Password: p.auth.Token,
				},
				DefaultBranch: repo.DefaultBranch,
				Archived:      repo.Archived,
			})
		}

		return output, nil

	case schema.AuthConfigTypeApp:
		var repoData struct {
			Repos []struct {
				Name          string `json:"name"`
				CloneUrl      string `json:"clone_url"`
				DefaultBranch string `json:"default_branch"`
				Archived      bool   `json:"archived"`
				Owner         struct {
					Username string `json:"login"`
				} `json:"owner"`
			} `json:"repositories"`
		}

		_, req, err := p.authedInstallationRequest()
		if err != nil {
			return nil, fmt.Errorf("error making GitHub API request: %w", err)
		}

		req.SetResult(&repoData)
		response, err := req.Get(fmt.Sprintf("%s/installation/repositories?per_page=100", p.apiBaseUrl))

		if err != nil {
			return nil, fmt.Errorf("error making GitHub API request: %w", err)
		}

		if response.IsError() {
			return nil, fmt.Errorf("error making GitHub API request, status: %v", response.Status())
		}

		var output []schema.Repo
		for _, repo := range repoData.Repos {
			output = append(output, schema.Repo{
				Domain:    p.domain,
				OwnerName: repo.Owner.Username,
				Name:      repo.Name,

				CloneUrl: repo.CloneUrl,
				Auth: schema.RepoAuth{
					Username: "x-access-token",
					Password: p.auth.AppInstallationToken,
				},
				DefaultBranch: repo.DefaultBranch,
				Archived:      repo.Archived,
			})
		}

		return output, nil

	default:
		return nil, fmt.Errorf("unrecognised auth type: %s", p.auth.Type)
	}
}

func (p *GitHubPlatform) RepoHasTediumConfig(repo schema.Repo) (bool, error) {
	file, err := p.ReadRepoFile(repo, "", utils.AddYamlJsonExtensions(".tedium"))

	if err != nil {
		return false, fmt.Errorf("failed to read Tedium file via GitHub API: %w", err)
	}

	return file != nil, nil
}

func (p *GitHubPlatform) ReadRepoFile(repo schema.Repo, branch string, pathCandidates []string) ([]byte, error) {
	var repoFile struct {
		Content string `json:"content"`
	}

	for _, path := range pathCandidates {
		_, req, err := p.authedUserOrInstallationRequest()
		if err != nil {
			return nil, fmt.Errorf("failed to read file via GitHub API: %w", err)
		}

		if branch != "" {
			req.SetQueryParam("ref", branch)
		}

		req.SetResult(&repoFile)
		url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", p.apiBaseUrl, repo.OwnerName, repo.Name, path)
		response, err := req.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to read file via GitHub API: %w", err)
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

func (p *GitHubPlatform) OpenOrUpdatePullRequest(job schema.Job) error {
	l.Info("Opening or updating PR", "chore", job.Chore.Name)

	var existingPrs []struct {
		Num   int    `json:"number"`
		State string `json:"state"`

		Base struct {
			Label string `json:"label"`
		} `json:"base"`
		Head struct {
			Label string `json:"label"`
		} `json:"head"`
	}

	_, req, err := p.authedUserOrInstallationRequest()
	if err != nil {
		return fmt.Errorf("error fetching existing PRs: %w", err)
	}

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
		if pr.Base.Label == fmt.Sprintf("%s:%s", job.Repo.OwnerName, job.Repo.DefaultBranch) && pr.Head.Label == fmt.Sprintf("%s:%s", job.Repo.OwnerName, job.FinalBranchName) && pr.State == "open" {
			existingPrNum = pr.Num
			break
		}
	}

	prBody := map[string]interface{}{
		"base":  job.Repo.DefaultBranch,
		"head":  fmt.Sprintf("%s:%s", job.Repo.OwnerName, job.FinalBranchName),
		"title": job.Chore.PrTitle(),
		"body":  job.Chore.PrBody(),
	}

	_, req, err = p.authedUserOrInstallationRequest()
	if err != nil {
		return fmt.Errorf("error fetching existing PRs: %w", err)
	}

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
		return fmt.Errorf("error opening or updating PR: status %d", response.StatusCode())
	}

	return nil
}

// internal methods

func (p *GitHubPlatform) loadProfile(conf schema.TediumConfig) error {
	if p.auth == nil || p.SkipDiscovery {
		return nil
	}

	switch p.auth.Type {
	case schema.AuthConfigTypeUserToken:
		var userEmails []struct {
			Email   string `json:"email"`
			Primary bool   `json:"primary"`
		}

		_, req, err := p.authedUserRequest()
		if err != nil {
			return fmt.Errorf("error loading user profile: %w", err)
		}
		req.SetResult(&userEmails)
		response, err := req.Get(fmt.Sprintf("%s/user/emails", p.apiBaseUrl))

		if err != nil {
			return fmt.Errorf("failed to load user profile: %w", err)
		}

		if response.IsError() {
			return fmt.Errorf("failed to load user profile: %v", response.Status())
		}

		primaryEmail := ""
		for _, email := range userEmails {
			if email.Primary {
				primaryEmail = email.Email
				break
			}
		}

		if primaryEmail == "" {
			return fmt.Errorf("failed to load user profile: no primary email addresses")
		}

		p.profile = schema.PlatformProfile{
			Email: primaryEmail,
		}

		return nil

	case schema.AuthConfigTypeApp:
		var appProfile struct {
			Slug string `json:"slug"`
		}

		_, req, err := p.authedAppRequest()
		if err != nil {
			return fmt.Errorf("error loading app profile: %w", err)
		}
		req.SetResult(&appProfile)
		response, err := req.Get(fmt.Sprintf("%s/app", p.apiBaseUrl))

		if err != nil {
			return fmt.Errorf("failed to load app profile: %w", err)
		}

		if response.IsError() {
			return fmt.Errorf("failed to load app profile: %v", response.Status())
		}

		p.profile = schema.PlatformProfile{
			Email: appProfile.Slug + "[bot]@users.noreply.github.com",
		}

		return nil

	default:
		return fmt.Errorf("unrecognised auth type: %s", p.auth.Type)
	}
}

// three kinds of authenticated request:
// - user: request using a simple token from config; this is used for all requests when operating as a user
// - app: request using a JWT for an application; this is used when operating as an app for requests that ARE NOT related to a specific installation of the app
// - installation: request using a short-lived token; this is used when operation an app for requests that ARE related to a specific installation

func (p *GitHubPlatform) authedUserOrInstallationRequest() (*resty.Client, *resty.Request, error) {
	if p.auth == nil {
		client := resty.New()
		request := client.NewRequest()
		return client, request, nil
	}

	switch p.auth.Type {
	case schema.AuthConfigTypeUserToken:
		return p.authedUserRequest()

	case schema.AuthConfigTypeApp:
		return p.authedInstallationRequest()

	default:
		return nil, nil, fmt.Errorf("unrecognised auth type: %s", p.auth.Type)
	}
}

func (p *GitHubPlatform) authedUserRequest() (*resty.Client, *resty.Request, error) {
	client := resty.New()
	request := client.NewRequest()

	if p.auth == nil {
		return nil, nil, fmt.Errorf("error making authed request to GitHub: no auth config found")
	}

	if p.auth.Type != schema.AuthConfigTypeUserToken {
		return nil, nil, fmt.Errorf("error making user-authed request to GitHub: auth type is not %s", schema.AuthConfigTypeUserToken)
	}

	request.SetHeader("Authorization", fmt.Sprintf("Bearer %s", p.auth.Token))
	request.SetHeader("User-Agent", "Tedium")

	return client, request, nil
}

func (p *GitHubPlatform) authedAppRequest() (*resty.Client, *resty.Request, error) {
	client := resty.New()
	request := client.NewRequest()

	if p.auth == nil {
		return nil, nil, fmt.Errorf("error making authed request to GitHub: no auth config found")
	}

	if p.auth.Type != schema.AuthConfigTypeApp {
		return nil, nil, fmt.Errorf("error making app-authed request to GitHub: auth type is not %s", schema.AuthConfigTypeApp)
	}

	jwt, err := p.auth.GenerateJwt()
	if err != nil {
		return nil, nil, fmt.Errorf("error making authed request to GitHub: %w", err)
	}

	request.SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwt))
	request.SetHeader("User-Agent", "Tedium")

	return client, request, nil
}

func (p *GitHubPlatform) authedInstallationRequest() (*resty.Client, *resty.Request, error) {
	client := resty.New()
	request := client.NewRequest()

	if p.auth == nil {
		return nil, nil, fmt.Errorf("error making authed request to GitHub: no auth config found")
	}

	if p.auth.Type != schema.AuthConfigTypeApp {
		return nil, nil, fmt.Errorf("error making installation-authed request to GitHub: auth type is not %s", schema.AuthConfigTypeApp)
	}

	// generate new installation token if we don't have one already
	if p.auth.AppInstallationToken == "" {
		var installationToken struct {
			Token string `json:"token"`
		}

		_, req, err := p.authedAppRequest()
		if err != nil {
			return nil, nil, err
		}
		req.SetResult(&installationToken)
		response, err := req.Post(fmt.Sprintf("%s/app/installations/%s/access_tokens", p.apiBaseUrl, p.auth.InstallationId))

		if err != nil {
			return nil, nil, fmt.Errorf("error generating installation access token: %w", err)
		}

		if response.IsError() {
			return nil, nil, fmt.Errorf("error generating installation access token, status: %v", response.Status())
		}

		p.auth.AppInstallationToken = installationToken.Token
	}

	request.SetHeader("Authorization", fmt.Sprintf("Bearer %s", p.auth.AppInstallationToken))
	request.SetHeader("User-Agent", "Tedium")

	return client, request, nil
}
