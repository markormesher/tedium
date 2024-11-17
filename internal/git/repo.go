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
	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
)

var l = logging.Logger

// repos are only ever cloned inside an execution container, so this path doesn't change per-repo
var repoClonePath = "/tedium/repo"

func CloneRepo(repo *schema.Repo, conf *schema.TediumConfig) error {
	l.Info("Cloning repo", "url", repo.CloneUrl)

	err := os.MkdirAll(repoClonePath, os.ModePerm)
	if err != nil {
		return fmt.Errorf("Error creating repo storage: %v", err)
	}

	realRepo, err := git.PlainClone(repoClonePath, false, &git.CloneOptions{
		URL:  repo.CloneUrl,
		Auth: repo.Auth,
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

func CommitAndPushIfChanged(job *schema.Job, profile *schema.PlatformProfile) (bool, error) {
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
			Email: profile.Email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return false, fmt.Errorf("Error committing changes: %w", err)
	}

	reportRepoState(realRepo, "commit: after")

	err = realRepo.Push(&git.PushOptions{
		Force: false,
		Auth:  job.Repo.Auth,
	})
	if err != nil {
		return false, fmt.Errorf("Error pushing changes: %w", err)
	}

	return true, nil
}

func openRepo(r *schema.Repo) (*git.Repository, *git.Worktree, error) {
	repo, err := git.PlainOpen(repoClonePath)
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
		RefSpecs: []config.RefSpec{"+refs/heads/*:refs/remotes/origin/*", "+refs/*:refs/*"},
		Auth:     repo.Auth,
		Prune:    true,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("Error fetching repo refs: %w", err)
	}

	return nil
}

func reportRepoState(realRepo *git.Repository, note string) {
	head, err := realRepo.Head()
	if err != nil {
		l.Error("Failed to repo repo state", "error", err)
		return
	}

	l.Debug("Repo state @ "+note, "headName", head.Name(), "headHash", head.Hash().String())
}
