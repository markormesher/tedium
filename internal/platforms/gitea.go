package platforms

import (
	"encoding/base64"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
)

var l = logging.Logger

type GiteaPlatform struct {
	Endpoint string
	Auth     *schema.AuthConfig

	// private state
	finalAuth  *schema.AuthConfig
	botProfile *schema.PlatformBotProfile
}

// interface methods

func (p *GiteaPlatform) Init(conf *schema.TediumConfig) error {
	// resolve the auth config that should be used
	p.finalAuth = conf.GetAuthConfig(p.Endpoint)

	err := p.loadBotProfile(conf)
	if err != nil {
		return err
	}

	return nil
}

func (p *GiteaPlatform) Deinit() error {
	return nil
}

func (p *GiteaPlatform) BotProfile() *schema.PlatformBotProfile {
	return p.botProfile
}

func (p *GiteaPlatform) DiscoverRepos() ([]schema.Repo, error) {
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

	response, err := req.Get(fmt.Sprintf("%s/repos/search", p.Endpoint))

	if err != nil {
		return nil, fmt.Errorf("Error making Gitea API request: %v", err)
	}

	if response.IsError() {
		return nil, fmt.Errorf("Error making Gitea API request, status: %v", response.Status())
	}

	var output []schema.Repo
	for _, repo := range repoData.Data {
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

func (p *GiteaPlatform) RepoHasTediumConfig(repo *schema.Repo) (bool, error) {
	file, err := p.ReadRepoFile(repo, ".tedium.json")

	if err != nil {
		return false, fmt.Errorf("Failed to read Tedium file via Gitea API: %w", err)
	}

	return file != nil, nil
}

func (p *GiteaPlatform) ReadRepoFile(repo *schema.Repo, path string) ([]byte, error) {
	var repoFile struct {
		Content string `json:"content"`
	}

	_, req := p.authedRequest()
	req.SetResult(&repoFile)
	response, err := req.Get(fmt.Sprintf("%s/repos/%s/%s/contents/%s", p.Endpoint, repo.OwnerName, repo.Name, path))
	if err != nil {
		return nil, fmt.Errorf("Failed to read file via Gitea API: %w", err)
	}

	if response.StatusCode() == 404 {
		return nil, nil
	}

	fileStr, err := base64.StdEncoding.DecodeString(repoFile.Content)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode base64 string: %w", err)
	}

	return fileStr, nil
}

func (p *GiteaPlatform) OpenOrUpdatePullRequest(job *schema.Job) error {
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

	_, req := p.authedRequest()
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
		if pr.Base.Label == job.Repo.DefaultBranch && pr.Head.Label == branchName && pr.State == "open" {
			existingPrNum = pr.Num
			break
		}
	}

	prBody := map[string]interface{}{
		"base":  job.Repo.DefaultBranch,
		"head":  branchName,
		"title": job.Chore.PrTitle(),
		"body":  job.Chore.PrBody(),
	}

	_, req = p.authedRequest()
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

	if response.StatusCode() != 201 {
		return fmt.Errorf("Error opening or updating PR: %v", string(response.Body()))
	}

	return nil
}

// internal methods

func (p *GiteaPlatform) authedRequest() (client *resty.Client, request *resty.Request) {
	client = resty.New()
	request = client.NewRequest()
	if p.finalAuth != nil && p.finalAuth.Token != "" {
		request.SetHeader("Authorization", fmt.Sprintf("token %s", p.finalAuth.Token))
	}
	return
}

func (p *GiteaPlatform) loadBotProfile(conf *schema.TediumConfig) error {
	var user struct {
		Username string `json:"login"`
		Email    string `json:"email"`
	}

	_, req := p.authedRequest()
	req.SetResult(&user)
	response, err := req.Get(fmt.Sprintf("%s/user", p.Endpoint))

	if err != nil {
		return fmt.Errorf("Failed to load bot user: %v", err)
	}

	if response.IsError() {
		return fmt.Errorf("Failed to load bot user, status: %v", response.Status())
	}

	p.botProfile = &schema.PlatformBotProfile{
		Username: user.Username,
		Email:    user.Email,
	}

	return nil
}
