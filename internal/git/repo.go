package git

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
)

var l = logging.Logger

func CloneAndUpdateRepo(repo *schema.Repo, conf *schema.TediumConfig) error {
	err := CloneRepo(repo, conf)
	if err != nil {
		return err
	}

	err = UpdateRepo(repo)
	if err != nil {
		return err
	}

	return nil
}

func CloneRepo(repo *schema.Repo, conf *schema.TediumConfig) error {
	l.Info("Cloning repo", "url", repo.CloneUrl)

	// don't use the auto-generated path if one has already been set
	if repo.PathOnDisk == "" {
		repo.PathOnDisk = repoStoragePath(conf, repo)
	}

	isPresent, err := isPresentOnDisk(repo)
	if err != nil {
		return fmt.Errorf("Error checking whether repo is already present on disk: %w", err)
	}

	if isPresent {
		l.Debug("Repo is already present - doing nothing", "source", repo.CloneUrl, "destination", repo.PathOnDisk)
		return nil
	}

	err = os.MkdirAll(repo.PathOnDisk, os.ModePerm)
	if err != nil {
		return fmt.Errorf("Error creating repo storage: %v", err)
	}

	realRepo, err := git.PlainClone(repo.PathOnDisk, false, &git.CloneOptions{
		URL:  repo.CloneUrl,
		Auth: repoAuth(repo),
	})
	if err != nil {
		return fmt.Errorf("Error cloning repo: %w", err)
	}

	err = fetchAll(repo)
	if err != nil {
		return fmt.Errorf("Error running fetch: %w", err)
	}

	reportRepoState(realRepo, "clone: after")

	return nil
}

func UpdateRepo(repo *schema.Repo) error {
	l.Info("Updating repo", "url", repo.CloneUrl)

	isPresent, err := isPresentOnDisk(repo)
	if err != nil {
		return fmt.Errorf("Error checking whether repo is already present on disk: %w", err)
	}

	if !isPresent {
		return fmt.Errorf("Cannot update repo that is not present on disk")
	}

	realRepo, worktree, err := openRepo(repo)
	if err != nil {
		return err
	}

	reportRepoState(realRepo, "update: before pull")

	err = fetchAll(repo)
	if err != nil {
		return fmt.Errorf("Error running fetch: %w", err)
	}

	reportRepoState(realRepo, "update: after fetch")

	err = worktree.Pull(&git.PullOptions{
		Auth: repoAuth(repo),
	})
	if err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			l.Debug("Repo has no changes")
		} else {
			return fmt.Errorf("Error pulling repo updates: %w", err)
		}
	}

	reportRepoState(realRepo, "update: after pull")

	return nil
}

// CheckoutBranchForJob checks out a named branch on a repo, creating it if necessary.
func CheckoutBranchForJob(job *schema.Job) error {
	branchName := utils.ConvertToBranchName(job.Chore.Name)
	branchRefName := plumbing.NewBranchReferenceName(branchName)

	l.Info("Checking out a branch for chore", "branch", branchName)

	realRepo, worktree, err := openRepo(job.Repo)
	if err != nil {
		return err
	}

	// sanity check: the repo should be in a clean state, otherwise something else is misbehaving; if it is not, bail out to avoid making things worse
	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("Error checking repo status: %w", err)
	}
	if !status.IsClean() {
		return fmt.Errorf("Refusing to checkout a new branch on an unclean repo")
	}

	reportRepoState(realRepo, "chore branch: before")

	// iterate branches to check whether we have one already (checking via .Branch(name string) doesn't reliably work)
	branchExists := false
	branches, err := realRepo.Branches()
	if err != nil {
		return fmt.Errorf("Error listing repo branches: %w", err)
	}
	err = branches.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name() == branchRefName {
			branchExists = true
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("Error checking whether chore branch already exists: %w", err)
	}

	if !branchExists {
		l.Info("Branch does not exist - it will be created", "branch", branchName)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: branchRefName,
		Create: !branchExists,
	})
	if err != nil {
		return fmt.Errorf("Error checking out chore branch: %w", err)
	}

	reportRepoState(realRepo, "chore branch: after checkout")

	return nil
}

func CommitAndPushIfChanged(job *schema.Job, botProfile *schema.PlatformBotProfile) (bool, error) {
	realRepo, worktree, err := openRepo(job.Repo)
	if err != nil {
		return false, err
	}

	repoStatus, err := worktree.Status()
	if err != nil {
		return false, fmt.Errorf("Error checking worktree status: %w", err)
	}

	if repoStatus.IsClean() {
		l.Info("Chore did not modify repo")
		return false, nil
	}

	l.Info("Committing and pushing changes")

	reportRepoState(realRepo, "commit: before")

	_, err = worktree.Add(".")
	if err != nil {
		return false, fmt.Errorf("Error adding changes: %w", err)
	}

	msg := fmt.Sprintf("Apply chore: %s", job.Chore.Name)
	_, err = worktree.Commit(msg, &git.CommitOptions{
		All: true,
		Author: &object.Signature{
			Name:  botProfile.Username,
			Email: botProfile.Email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return false, fmt.Errorf("Error committing changes: %w", err)
	}

	reportRepoState(realRepo, "commit: after")

	err = realRepo.Push(&git.PushOptions{
		Force: false,
		Auth:  repoAuth(job.Repo),
	})
	if err != nil {
		return false, fmt.Errorf("Error pushing changes: %w", err)
	}

	return true, nil
}

func ReadFile(repo *schema.Repo, pathCandidates []string) ([]byte, error) {
	isPresent, err := isPresentOnDisk(repo)
	if err != nil {
		return nil, fmt.Errorf("Error checking before reading file from repo: %w", err)
	}

	if !isPresent {
		return nil, fmt.Errorf("Cannot read a file from a repo that is not present on disk")
	}

	for _, path := range pathCandidates {
		fullPath := fmt.Sprintf("%s/%s", repo.PathOnDisk, path)

		_, pathErr := os.Stat(fullPath)
		if pathErr != nil {
			if os.IsNotExist(pathErr) {
				continue
			}

			return nil, fmt.Errorf("Error checking whether repo file exists: %w", err)
		}

		file, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("Error reading file from repo: %w", err)
		}

		return file, nil
	}

	return nil, fmt.Errorf("Could not ready from any candidate path: %v", pathCandidates)
}

func repoAuth(repo *schema.Repo) transport.AuthMethod {
	authConfig := repo.AuthConfig
	if authConfig == nil {
		return nil
	}

	if authConfig.Token != "" {
		return &http.TokenAuth{
			Token: authConfig.Token,
		}
	}

	return nil
}

func openRepo(r *schema.Repo) (*git.Repository, *git.Worktree, error) {
	repo, err := git.PlainOpen(r.PathOnDisk)
	if err != nil {
		return nil, nil, fmt.Errorf("Error opening repo: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return nil, nil, fmt.Errorf("Error accessing repo work tree: %w", err)
	}

	return repo, worktree, nil
}

func fetchAll(repo *schema.Repo) error {
	realRepo, _, err := openRepo(repo)
	if err != nil {
		return fmt.Errorf("Error opening repo: %w", err)
	}

	origin, err := realRepo.Remote("origin")
	if err != nil {
		return fmt.Errorf("Error getting origin remote: %w", err)
	}

	err = origin.Fetch(&git.FetchOptions{
		RefSpecs: []config.RefSpec{"+refs/heads/*:refs/remotes/origin/*", "+refs/*:refs/*", "+HEAD:refs/heads/HEAD"},
		Auth:     repoAuth(repo),
		Prune:    true,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("Error fetching repo refs: %w", err)
	}

	return nil
}

func isPresentOnDisk(repo *schema.Repo) (bool, error) {
	if repo.PathOnDisk == "" {
		return false, fmt.Errorf("Repo does not have a disk path set")
	}

	// check whether the directory this repo is cloned to exists
	_, repoDirErr := os.Stat(repo.PathOnDisk)
	if repoDirErr != nil {
		if os.IsNotExist(repoDirErr) {
			return false, nil
		} else {
			return false, fmt.Errorf("Error checking whether repo exists on disk: %w", repoDirErr)
		}
	}

	// check whether the directory this repo is cloned to contains a .git folder
	_, gitDirErr := os.Stat(repo.PathOnDisk + "/.git")
	if gitDirErr != nil {
		if os.IsNotExist(gitDirErr) {
			return false, nil
		} else {
			return false, fmt.Errorf("Error checking whether repo exists on disk: %w", gitDirErr)
		}
	}

	return true, nil
}

func reportRepoState(realRepo *git.Repository, note string) {
	head, err := realRepo.Head()
	if err != nil {
		l.Error("Failed to repo repo state", "error", err)
		return
	}

	l.Debug("Repo state @ "+note, "headName", head.Name(), "headHash", head.Hash().String())
}
