package platforms

import (
	"encoding/base64"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
)

type GitHubPlatform struct {
	Endpoint string
	Auth     *schema.AuthConfig

	// private state
	originalPlatformConfig *schema.PlatformConfig
	finalAuth              *schema.AuthConfig
	botProfile             *schema.PlatformBotProfile
}

// interface methods

func (p *GitHubPlatform) Init(conf *schema.TediumConfig) error {
	// resolve the auth config that should be used
	p.finalAuth = conf.GetAuthConfig(p.Endpoint)

	// get auth token for the installation
	if p.finalAuth.InternalToken == "" {
		var installationToken struct {
			Token string `json:"token"`
		}

		_, req, err := p.authedAppRequest()
		if err != nil {
			return err
		}
		req.SetResult(&installationToken)
		response, err := req.Post(fmt.Sprintf("%s/app/installations/%s/access_tokens", p.Endpoint, p.finalAuth.InstallationId))

		if err != nil {
			return fmt.Errorf("Error generating installation access token: %w", err)
		}

		if response.IsError() {
			return fmt.Errorf("Error generating installation access token, status: %v", response.Status())
		}

		p.finalAuth.InternalToken = installationToken.Token
	}

	// load (and update, in the future) the bot profile
	err := p.loadBotProfile(conf)
	if err != nil {
		return err
	}

	return nil
}

func (p *GitHubPlatform) Deinit() error {
	return nil
}

func (p *GitHubPlatform) BotProfile() *schema.PlatformBotProfile {
	return p.botProfile
}

func (p *GitHubPlatform) DiscoverRepos() ([]schema.Repo, error) {
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
		return nil, fmt.Errorf("Error making GitHub API request: %w", err)
	}

	req.SetResult(&repoData)
	response, err := req.Get(fmt.Sprintf("%s/installation/repositories?per_page=100", p.Endpoint))

	if err != nil {
		return nil, fmt.Errorf("Error making GitHub API request: %w", err)
	}

	if response.IsError() {
		return nil, fmt.Errorf("Error making GitHub API request, status: %v", response.Status())
	}

	var output []schema.Repo
	for _, repo := range repoData.Repos {
		output = append(output, schema.Repo{
			AuthConfig:    p.finalAuth,
			CloneUrl:      repo.CloneUrl,
			OwnerName:     repo.Owner.Username,
			Name:          repo.Name,
			DefaultBranch: repo.DefaultBranch,
			Archived:      repo.Archived,
		})
	}

	return output, nil
}

func (p *GitHubPlatform) RepoHasTediumConfig(repo *schema.Repo) (bool, error) {
	file, err := p.ReadRepoFile(repo, utils.AddYamlJsonExtensions(".tedium"))

	if err != nil {
		return false, fmt.Errorf("Failed to read Tedium file via GitHub API: %w", err)
	}

	return file != nil, nil
}

func (p *GitHubPlatform) ReadRepoFile(repo *schema.Repo, pathCandidates []string) ([]byte, error) {
	var repoFile struct {
		Content string `json:"content"`
	}

	for _, path := range pathCandidates {
		_, req, err := p.authedInstallationRequest()
		if err != nil {
			return nil, fmt.Errorf("Failed to read file via GitHub API: %w", err)
		}
		req.SetResult(&repoFile)
		response, err := req.Get(fmt.Sprintf("%s/repos/%s/%s/contents/%s", p.Endpoint, repo.OwnerName, repo.Name, path))
		if err != nil {
			return nil, fmt.Errorf("Failed to read file via GitHub API: %w", err)
		}

		if response.StatusCode() == 404 {
			// no match for this candidate, but there may be others
			continue
		}

		fileStr, err := base64.StdEncoding.DecodeString(repoFile.Content)
		if err != nil {
			return nil, fmt.Errorf("Failed to decode base64 string: %w", err)
		}

		return fileStr, nil
	}

	// no result for any path candidate
	return nil, nil
}

func (p *GitHubPlatform) OpenOrUpdatePullRequest(job *schema.Job) error {
	l.Info("Opening or updating PR", "chore", job.Chore.Name)

	branchName := utils.ConvertToBranchName(job.Chore.Name)

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

	_, req, err := p.authedInstallationRequest()
	if err != nil {
		return fmt.Errorf("Error fetching existing PRs: %w", err)
	}
	req.SetResult(&existingPrs)
	response, err := req.Get(fmt.Sprintf("%s/repos/%s/%s/pulls", p.Endpoint, job.Repo.OwnerName, job.Repo.Name))
	if err != nil {
		return fmt.Errorf("Error fetching existing PRs: %w", err)
	}

	if response.StatusCode() != 200 {
		return fmt.Errorf("Error fetching existing PRs: %v", string(response.Body()))
	}

	var existingPrNum int
	for _, pr := range existingPrs {
		if pr.Base.Label == fmt.Sprintf("%s:%s", job.Repo.OwnerName, job.Repo.DefaultBranch) && pr.Head.Label == fmt.Sprintf("%s:%s", job.Repo.OwnerName, branchName) && pr.State == "open" {
			existingPrNum = pr.Num
			break
		}
	}

	prBody := map[string]interface{}{
		"base":  job.Repo.DefaultBranch,
		"head":  fmt.Sprintf("%s:%s", job.Repo.OwnerName, branchName),
		"title": job.Chore.PrTitle(),
		"body":  job.Chore.PrBody(),
	}

	_, req, err = p.authedInstallationRequest()
	if err != nil {
		return fmt.Errorf("Error opening or updating PR: %w", err)
	}
	req.SetHeader("Content-type", "application/json")
	req.SetBody(prBody)

	if existingPrNum == 0 {
		l.Debug("Opening PR")
		response, err = req.Post(fmt.Sprintf("%s/repos/%s/%s/pulls", p.Endpoint, job.Repo.OwnerName, job.Repo.Name))
	} else {
		l.Debug("Updating PR")
		response, err = req.Patch(fmt.Sprintf("%s/repos/%s/%s/pulls/%d", p.Endpoint, job.Repo.OwnerName, job.Repo.Name, existingPrNum))
	}

	if err != nil {
		return fmt.Errorf("Error opening or updating PR: %w", err)
	}

	if response.StatusCode() != 200 {
		return fmt.Errorf("Error opening or updating PR: %v", string(response.StatusCode()))
	}

	return nil
}

// internal methods

func (p *GitHubPlatform) authedAppRequest() (*resty.Client, *resty.Request, error) {
	client := resty.New()
	request := client.NewRequest()

	if p.finalAuth == nil {
		return nil, nil, fmt.Errorf("Error making authed request to GitHub: no auth config found")
	}

	jwt, err := p.finalAuth.GenerateJwt()
	if err != nil {
		return nil, nil, fmt.Errorf("Error making authed request to GitHub: %w", err)
	}

	request.SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwt))

	request.SetHeader("User-Agent", "Tedium")

	return client, request, nil
}

func (p *GitHubPlatform) authedInstallationRequest() (*resty.Client, *resty.Request, error) {
	client := resty.New()
	request := client.NewRequest()

	if p.finalAuth == nil {
		return nil, nil, fmt.Errorf("Error making authed request to GitHub: no auth config found")
	}

	if p.finalAuth.InternalToken == "" {
		return nil, nil, fmt.Errorf("Error making authed request to GitHub: no installation token")
	}

	request.SetHeader("Authorization", fmt.Sprintf("Bearer %s", p.finalAuth.InternalToken))

	request.SetHeader("User-Agent", "Tedium")

	return client, request, nil
}

func (p *GitHubPlatform) loadBotProfile(conf *schema.TediumConfig) error {
	var appProfile struct {
		Slug string `json:"slug"`
	}

	_, req, err := p.authedAppRequest()
	if err != nil {
		return fmt.Errorf("Error loading bot profile: %w", err)
	}
	req.SetResult(&appProfile)
	response, err := req.Get(fmt.Sprintf("%s/app", p.Endpoint))

	if err != nil {
		return fmt.Errorf("Failed to load bot profile: %w", err)
	}

	if response.IsError() {
		return fmt.Errorf("Failed to load bot profile: %v", response.Status())
	}

	p.botProfile = &schema.PlatformBotProfile{
		Username: appProfile.Slug,
		Email:    appProfile.Slug + "[bot]@users.noreply.github.com",
	}

	return nil
}
