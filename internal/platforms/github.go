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

type GitHubPlatform struct {
	schema.PlatformConfig

	// supplied via config
	baseURLs []*urllib.URL
	auth     *schema.AuthConfig

	// generated locally
	apiBaseURL *urllib.URL
	profile    schema.PlatformProfile
}

func githubPlatformFromConfig(platformConfig schema.PlatformConfig) (*GitHubPlatform, error) {
	p := GitHubPlatform{
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
	apiBaseURL := urlParsed.JoinPath("")
	apiBaseURL.Host = "api." + apiBaseURL.Host
	p.apiBaseURL = apiBaseURL

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

func (p *GitHubPlatform) Init(conf schema.TediumConfig) error {
	err := p.loadProfile()
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

func (p *GitHubPlatform) APIBaseURL() *urllib.URL {
	return p.apiBaseURL
}

func (p *GitHubPlatform) AcceptsURL(url string) (string, bool) {
	urlParsed, err := urllib.Parse(url)
	if err != nil {
		return "", false
	}

	for _, url := range p.baseURLs {
		if urlParsed.Scheme == url.Scheme && urlParsed.Host == url.Host {
			urlParsed.Scheme = url.Scheme
			urlParsed.Host = url.Host
			return urlParsed.String(), true
		}
	}

	return "", false
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
		slog.Warn("no auth configured for paltform; skipping repo discovery", "baseURL", p.baseURLs)
		return []schema.Repo{}, nil
	}

	switch p.auth.Type {
	case schema.AuthConfigTypeUserToken:
		var repoData []struct {
			Name          string `json:"name"`
			CloneURL      string `json:"clone_url"`
			DefaultBranch string `json:"default_branch"`
			Archived      bool   `json:"archived"`
			Owner         struct {
				Username string `json:"login"`
			} `json:"owner"`
		}

		var output []schema.Repo
		url := fmt.Sprintf("%s/user/repos?page=1&per_page=50", p.apiBaseURL)

		for {
			_, req, err := p.authedUserRequest()
			if err != nil {
				return nil, fmt.Errorf("error making GitHub API request: %w", err)
			}

			req.SetResult(&repoData)

			response, err := req.Get(url)
			if err != nil {
				return nil, fmt.Errorf("error making GitHub API request: %w", err)
			}

			if response.IsError() {
				return nil, fmt.Errorf("error making GitHub API request, status: %v", response.Status())
			}

			for _, repo := range repoData {
				cloneURL, ok := p.AcceptsURL(repo.CloneURL)
				if !ok {
					return nil, fmt.Errorf("platform returned a repo with an unaccepted clone URL: %s", repo.CloneURL)
				}

				output = append(output, schema.Repo{
					OwnerName: repo.Owner.Username,
					Name:      repo.Name,

					CloneURL: cloneURL,
					Auth: schema.RepoAuth{
						Username: "x-access-token",
						Password: p.auth.Token,
					},
					DefaultBranch: repo.DefaultBranch,
					Archived:      repo.Archived,
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

	case schema.AuthConfigTypeApp:
		var repoData struct {
			Repos []struct {
				Name          string `json:"name"`
				CloneURL      string `json:"clone_url"`
				DefaultBranch string `json:"default_branch"`
				Archived      bool   `json:"archived"`
				Owner         struct {
					Username string `json:"login"`
				} `json:"owner"`
			} `json:"repositories"`
		}

		var output []schema.Repo
		url := fmt.Sprintf("%s/installation/repositories?page=1&per_page=50", p.apiBaseURL)

		for {
			_, req, err := p.authedInstallationRequest()
			if err != nil {
				return nil, fmt.Errorf("error making GitHub API request: %w", err)
			}

			req.SetResult(&repoData)
			response, err := req.Get(url)

			if err != nil {
				return nil, fmt.Errorf("error making GitHub API request: %w", err)
			}

			if response.IsError() {
				return nil, fmt.Errorf("error making GitHub API request, status: %v", response.Status())
			}

			for _, repo := range repoData.Repos {
				cloneURL, ok := p.AcceptsURL(repo.CloneURL)
				if !ok {
					return nil, fmt.Errorf("platform returned a repo with an unaccepted clone URL: %s", repo.CloneURL)
				}

				output = append(output, schema.Repo{
					OwnerName: repo.Owner.Username,
					Name:      repo.Name,

					CloneURL: cloneURL,
					Auth: schema.RepoAuth{
						Username: "x-access-token",
						Password: p.auth.AppInstallationToken,
					},
					DefaultBranch: repo.DefaultBranch,
					Archived:      repo.Archived,
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

	default:
		return nil, fmt.Errorf("unrecognised auth type: %s", p.auth.Type)
	}
}

func (p *GitHubPlatform) RepoHasTediumConfig(repo schema.Repo) (bool, error) {
	file, err := p.ReadRepoFile(repo, "", utils.AddConfigFileExtensions(".tedium"))

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
		url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", p.apiBaseURL, repo.OwnerName, repo.Name, path)
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
	slog.Info("opening or updating PR", "chore", job.Chore.Name)

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
	response, err := req.Get(fmt.Sprintf("%s/repos/%s/%s/pulls", p.apiBaseURL, job.Repo.OwnerName, job.Repo.Name))
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

	prBody := map[string]any{
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
		slog.Debug("opening PR")
		response, err = req.Post(fmt.Sprintf("%s/repos/%s/%s/pulls", p.apiBaseURL, job.Repo.OwnerName, job.Repo.Name))
	} else {
		slog.Debug("updating PR")
		response, err = req.Patch(fmt.Sprintf("%s/repos/%s/%s/pulls/%d", p.apiBaseURL, job.Repo.OwnerName, job.Repo.Name, existingPrNum))
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

func (p *GitHubPlatform) loadProfile() error {
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
		response, err := req.Get(fmt.Sprintf("%s/user/emails", p.apiBaseURL))

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
		response, err := req.Get(fmt.Sprintf("%s/app", p.apiBaseURL))

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
		response, err := req.Post(fmt.Sprintf("%s/app/installations/%s/access_tokens", p.apiBaseURL, p.auth.InstallationID))

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
