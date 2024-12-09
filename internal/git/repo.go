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
)

var l = logging.Logger

// repos are only ever cloned inside an execution container, so this path doesn't change per-repo
var repoClonePath = "/tedium/repo"

func CloneRepo(job *schema.Job, conf *schema.TediumConfig) error {
	repo := job.Repo

	l.Info("Cloning repo", "url", repo.CloneUrl)

	err := os.MkdirAll(repoClonePath, os.ModePerm)
	if err != nil {
		return fmt.Errorf("Error creating repo storage: %v", err)
	}

	_, err = git.PlainClone(repoClonePath, false, &git.CloneOptions{
		URL:  repo.CloneUrl,
		Auth: repo.Auth.ToTransportAuth(),
	})
	if err != nil {
		return fmt.Errorf("Error cloning repo: %w", err)
	}

	err = fetchAll(repo)
	if err != nil {
		return fmt.Errorf("Error running fetch: %w", err)
	}

	return nil
}

// CheckoutBranch checks out a branch in a repo, creating it if necessary
func CheckoutBranch(job *schema.Job, branchName string) error {
	branchRefName := plumbing.NewBranchReferenceName(branchName)

	l.Info("Checking out a branch for chore", "branch", branchName)

	realRepo, worktree, err := openRepo(job.Repo)
	if err != nil {
		return err
	}

	// sanity check: the repo should be in a clean state
	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("Error checking repo status: %w", err)
	}
	if !status.IsClean() {
		return fmt.Errorf("Refusing to checkout a new branch on an unclean repo")
	}

	branchExists, err := branchExists(realRepo, branchName)
	if err != nil {
		return fmt.Errorf("error checking whether chore branch already exists: %w", err)
	}

	if !branchExists {
		l.Info("Branch does not exist - it will be created", "branch", branchName)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: branchRefName,
		Create: !branchExists,
	})
	if err != nil {
		return fmt.Errorf("error checking out chore branch: %w", err)
	}

	return nil
}

func CommitIfChanged(job *schema.Job, profile *schema.PlatformProfile) (bool, error) {
	_, worktree, err := openRepo(job.Repo)
	if err != nil {
		return false, err
	}

	repoStatus, err := worktree.Status()
	if err != nil {
		return false, fmt.Errorf("Error checking worktree status: %w", err)
	}

	if repoStatus.IsClean() {
		return false, nil
	}

	l.Info("Committing changes")

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

	return true, nil
}

func TmpBranchHasChanges(job *schema.Job) (bool, error) {
	realRepo, _, err := openRepo(job.Repo)
	if err != nil {
		return false, err
	}

	finalBranchExists, err := branchExists(realRepo, job.FinalBranchName)
	if err != nil {
		return false, fmt.Errorf("error checking whether final branch exists: %w", err)
	}

	if !finalBranchExists {
		// final branch doesn't exist yet, so we definitely need to push changes
		return true, nil
	}

	// final branch does exist, so check whether it's different to the temp branch
	tmpBranchCommit, err := getLatestCommit(realRepo, job.TmpBranchName)
	if err != nil {
		return false, fmt.Errorf("error getting latest commit on temporary branch: %w", err)
	}

	finalBranchCommit, err := getLatestCommit(realRepo, job.FinalBranchName)
	if err != nil {
		return false, fmt.Errorf("error getting latest commit on final branch: %w", err)
	}

	hasChanges := tmpBranchCommit.TreeHash != finalBranchCommit.TreeHash

	return hasChanges, nil
}

func Push(job *schema.Job) error {
	realRepo, _, err := openRepo(job.Repo)
	if err != nil {
		return err
	}

	l.Info("Pushing changes")

	err = realRepo.Push(&git.PushOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec(plumbing.ReferenceName("refs/heads/"+job.TmpBranchName) + ":" + plumbing.ReferenceName("refs/heads/"+job.FinalBranchName)),
		},
		Auth:  job.Repo.Auth.ToTransportAuth(),
		Force: true,
	})
	if err != nil {
		return fmt.Errorf("error pushing changes: %w", err)
	}

	return nil
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
		Auth:     repo.Auth.ToTransportAuth(),
		Prune:    true,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("Error fetching repo refs: %w", err)
	}

	return nil
}

func branchExists(realRepo *git.Repository, branchName string) (bool, error) {
	branchRefName := plumbing.NewBranchReferenceName(branchName)
	branchExists := false
	branches, err := realRepo.Branches()
	if err != nil {
		return false, fmt.Errorf("error listing repo branches: %w", err)
	}
	err = branches.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name() == branchRefName {
			branchExists = true
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("error iterating branches: %w", err)
	}

	return branchExists, nil
}

func getLatestCommit(realRepo *git.Repository, branchName string) (*object.Commit, error) {
	branchRef, err := realRepo.Reference(plumbing.ReferenceName("refs/heads/"+branchName), true)
	if err != nil {
		return nil, fmt.Errorf("error getting branch reference: %w", err)
	}

	commit, err := realRepo.CommitObject(branchRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("error getting latest commit: %w", err)
	}

	return commit, nil
}
