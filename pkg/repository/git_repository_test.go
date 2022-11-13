package repository

import (
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	fixtures "github.com/go-git/go-git-fixtures/v4"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	gitconfig "github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"gotest.tools/v3/assert"
)

func githubToken(t *testing.T) string {
	const githubTokenEnv = "GITHUB_TOKEN"
	token, run := os.LookupEnv(githubTokenEnv)
	if !run {
		t.Skipf("Skipping TestFetchWithTokenAuth as '%s' environment variable is not set", githubTokenEnv)
	}
	return token
}

func TestGitRepository(t *testing.T) {
	repoRoot := t.TempDir()
	repoURL := fixtures.Basic().One().DotGit().Root()

	repo, err := InitGitRepository(repoRoot, repoURL, nil)
	assert.NilError(t, err, "failed to init git repository")
	assert.Check(t, repo != nil, "repository should not be nil")

	// Reinitilizing should be a no-op
	repo, err = InitGitRepository(repoRoot, repoURL, nil)
	assert.NilError(t, err, "failed to get already initialized git repository")
	assert.Check(t, repo != nil, "repository should not be nil")

	branchCommit := "e8d3ffab552895c19b9fcf7aa264d277cde33881"
	commit, err := repo.FetchBranch("branch")
	assert.NilError(t, err, "failed to fetch branch")
	assert.Equal(t, commit.String(), branchCommit, "commit sha mismatch")

	// Refetching should be a no-op
	commit, err = repo.FetchBranch("branch")
	assert.NilError(t, err, "failed to fetch branch")
	assert.Equal(t, commit.String(), branchCommit, "commit sha mismatch")

	commitDir, err := repo.CreateCommitDir(*commit)
	assert.NilError(t, err, "failed to create commit dir")
	assert.Equal(t, commitDir, path.Join(repoRoot, "commits", branchCommit))

	entries, err := os.ReadDir(commitDir)
	assert.NilError(t, err, "failed to read commit dir")
	assert.Equal(t, len(entries), 8)

	masterCommit := "6ecf0ef2c2dffb796033e5a02219af86ec6584e5"
	commit, err = repo.FetchBranch("master")
	assert.NilError(t, err, "failed to fetch branch")
	assert.Equal(t, commit.String(), masterCommit, "commit sha mismatch")

	commitDir, err = repo.CreateCommitDir(*commit)
	assert.NilError(t, err, "failed to create commit dir")
	assert.Equal(t, commitDir, path.Join(repoRoot, "commits", masterCommit))

	entries, err = os.ReadDir(commitDir)
	assert.NilError(t, err, "failed to read commit dir")
	assert.Equal(t, len(entries), 8)
}

func TestForcePushBranch(t *testing.T) {
	repoRoot := t.TempDir()
	repoURL := fixtures.Basic().One().DotGit().Root()

	remoteRepo, err := git.PlainOpen(repoURL)
	assert.NilError(t, err, "failed to open repo")
	config, err := remoteRepo.Config()
	assert.NilError(t, err, "failed to get config")

	config.Raw.Sections = append(config.Raw.Sections, &gitconfig.Section{
		Name: "uploadpack",
		Options: []*gitconfig.Option{
			{Key: "allowReachableSHA1InWant", Value: "true"},
		},
	})
	assert.NilError(t, remoteRepo.SetConfig(config), "failed to set config")

	repo, err := InitGitRepository(repoRoot, repoURL, nil)
	assert.NilError(t, err, "failed to init git repository")
	assert.Check(t, repo != nil, "repository should not be nil")

	branchCommit := "e8d3ffab552895c19b9fcf7aa264d277cde33881"
	commit, err := repo.FetchBranch("branch")
	assert.NilError(t, err, "failed to fetch branch")
	assert.Equal(t, commit.String(), branchCommit, "commit sha mismatch")

	err = repo.FetchCommit(commit.String())
	assert.NilError(t, err, "failed to fetch commit")

	clonedRepo, err := git.PlainClone(t.TempDir(), false, &git.CloneOptions{
		URL:           repoURL,
		ReferenceName: plumbing.NewBranchReferenceName("branch"),
	})
	assert.NilError(t, err, "failed to clone repo")

	worktree, err := clonedRepo.Worktree()
	assert.NilError(t, err, "failed to open worktree")

	resetCommit := "918c48b83bd081e863dbe1b80f8998f058cd8294"
	err = worktree.Reset(&git.ResetOptions{
		Commit: plumbing.NewHash(resetCommit),
		Mode:   git.HardReset,
	})
	assert.NilError(t, err, "failed to reset worktree")

	err = clonedRepo.Push(&git.PushOptions{
		Force: true,
	})
	assert.NilError(t, err, "failed to force push")

	commit, err = repo.FetchBranch("branch")
	assert.NilError(t, err, "failed to fetch branch")
	assert.Equal(t, commit.String(), resetCommit, "commit sha mismatch")
}

func TestFetchWithTokenAuth(t *testing.T) {
	repoURL := "https://github.com/kuberik/git-auth-kustomize-test.git"

	repo, err := InitGitRepository(t.TempDir(), repoURL, &http.BasicAuth{
		Username: "notImportant",
		Password: githubToken(t),
	})
	assert.NilError(t, err, "failed to init git repository")
	assert.Check(t, repo != nil, "repository should not be nil")

	commit, err := repo.FetchBranch("main")
	assert.NilError(t, err, "failed to fetch branch")
	assert.Equal(t, "c984d9d19a53160658b0b70a326586ca3dc66874", commit.String())
}

func TestGitRepositoryFetchCommit(t *testing.T) {
	repoRoot := t.TempDir()

	repo, err := InitGitRepository(repoRoot, "https://github.com/git-fixtures/basic.git", nil)
	assert.NilError(t, err, "failed to init git repository")
	assert.Check(t, repo != nil, "repository should not be nil")

	commit := "35e85108805c84807bc66a02d91535e1e24b38b9"
	err = repo.FetchCommit(commit)
	assert.NilError(t, err, "failed to fetch commit")

	// Refetching should be a no-op
	err = repo.FetchCommit(commit)
	assert.NilError(t, err, "failed to fetch commit")

	commitDir, err := repo.CreateCommitDir(plumbing.NewHash(commit))
	assert.NilError(t, err, "failed to create commit dir")
	assert.Equal(t, commitDir, path.Join(repoRoot, "commits", commit))

	entries, err := os.ReadDir(commitDir)
	assert.NilError(t, err, "failed to read commit dir")
	assert.Equal(t, len(entries), 3)
}

func TestListBranches(t *testing.T) {
	repoRoot := t.TempDir()

	repo, err := InitGitRepository(repoRoot, "https://github.com/git-fixtures/basic.git", nil)
	assert.NilError(t, err, "failed to init git repository")
	assert.Check(t, repo != nil, "repository should not be nil")

	testCases := []struct {
		match string
		want  []string
	}{{
		match: "ma.+",
		want:  []string{"master"},
	}, {
		match: "feature/.+",
		want:  []string{},
	}, {
		match: "a",
		want:  []string{"branch", "master"},
	}, {
		match: "",
		want:  []string{"branch", "master"},
	}, {
		match: "^a",
		want:  []string{},
	}, {
		match: "^branch$",
		want:  []string{"branch"},
	}}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			branches, err := repo.ListBranches(tc.match)
			assert.NilError(t, err, "failed to list branches")
			assert.DeepEqual(t, branches, tc.want)
		})
	}
}

func TestListBranchesWithAuth(t *testing.T) {
	repoURL := "https://github.com/kuberik/git-auth-kustomize-test.git"

	repo, err := InitGitRepository(t.TempDir(), repoURL, &http.BasicAuth{
		Username: "notImportant",
		Password: githubToken(t),
	})
	assert.NilError(t, err, "failed to init git repository")
	assert.Check(t, repo != nil, "repository should not be nil")

	branches, err := repo.ListBranches("")
	assert.NilError(t, err, "failed to list branches")
	assert.DeepEqual(t, branches, []string{"main"})
}

// Issue: https://github.com/go-git/go-git/issues/328
// Solved by downgrading to 5.3.0 - https://github.com/go-git/go-git/issues/328#issuecomment-1086651486
func TestGitRepositoryEmptyUploadPack(t *testing.T) {
	repo, err := InitGitRepository(t.TempDir(), "https://github.com/git-fixtures/basic.git", nil)
	assert.NilError(t, err, "failed to init git repository")
	assert.Check(t, repo != nil, "repository should not be nil")

	branchCommit := "e8d3ffab552895c19b9fcf7aa264d277cde33881"
	commit, err := repo.FetchBranch("branch")
	assert.NilError(t, err, "failed to fetch branch")
	assert.Equal(t, commit.String(), branchCommit, "commit sha mismatch")

	// Refetching should be a no-op
	commit, err = repo.FetchBranch("branch")
	assert.NilError(t, err, "failed to fetch branch")
	assert.Equal(t, commit.String(), branchCommit, "commit sha mismatch")
}

func TestForcePushThenFetch(t *testing.T) {
	testCases := []struct {
		fetchFunc func(t *testing.T, repo *GitRepository, commit plumbing.Hash)
	}{{
		fetchFunc: func(t *testing.T, repo *GitRepository, commit plumbing.Hash) {
			assert.NilError(t, repo.FetchCommit(commit.String()), "failed to fetch commit")
		},
	}, {
		fetchFunc: func(t *testing.T, repo *GitRepository, commit plumbing.Hash) {
			fetchedCommit, err := repo.FetchBranch("branch")
			assert.NilError(t, err, "failed to fetch branch")
			assert.Equal(t, commit.String(), fetchedCommit.String(), "commit sha mismatch")
		},
	}, {
		fetchFunc: func(t *testing.T, repo *GitRepository, commit plumbing.Hash) {
			fetchedCommit, err := repo.FetchBranch("branch")
			assert.NilError(t, err, "failed to fetch branch")
			assert.Equal(t, commit.String(), fetchedCommit.String(), "commit sha mismatch")

			assert.NilError(t, repo.FetchCommit(commit.String()), "failed to fetch commit")
		},
	}}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test-case-%d", i), func(t *testing.T) {
			repoRoot := t.TempDir()
			repoURL := fixtures.Basic().One().DotGit().Root()

			remoteRepo, err := git.PlainOpen(repoURL)
			assert.NilError(t, err, "failed to open repo")
			config, err := remoteRepo.Config()
			assert.NilError(t, err, "failed to get config")

			config.Raw.Sections = append(config.Raw.Sections, &gitconfig.Section{
				Name: "uploadpack",
				Options: []*gitconfig.Option{
					{Key: "allowReachableSHA1InWant", Value: "true"},
				},
			})
			assert.NilError(t, remoteRepo.SetConfig(config), "failed to set config")

			repo, err := InitGitRepository(repoRoot, repoURL, nil)
			assert.NilError(t, err, "failed to init git repository")
			assert.Check(t, repo != nil, "repository should not be nil")

			commit := "e8d3ffab552895c19b9fcf7aa264d277cde33881"
			err = repo.FetchCommit(commit)
			assert.NilError(t, err, "failed to fetch commit")

			clonedRepo, err := git.PlainClone(t.TempDir(), false, &git.CloneOptions{
				URL:           repoURL,
				ReferenceName: plumbing.NewBranchReferenceName("branch"),
			})
			assert.NilError(t, err, "failed to clone repo")

			worktree, err := clonedRepo.Worktree()
			assert.NilError(t, err, "failed to open worktree")

			// Force push overwrite last commit 5 times
			for j := 0; j < 5; j++ {
				resetCommit := "918c48b83bd081e863dbe1b80f8998f058cd8294"
				err = worktree.Reset(&git.ResetOptions{
					Commit: plumbing.NewHash(resetCommit),
					Mode:   git.HardReset,
				})
				assert.NilError(t, err, "failed to reset worktree")

				file, err := worktree.Filesystem.Create("foo")
				assert.NilError(t, err, "failed to add file")
				_, err = file.Write([]byte(time.Now().String()))
				assert.NilError(t, err, "failed to write file")
				_, err = worktree.Add("foo")
				assert.NilError(t, err, "failed to add file to staging")
				newCommit, err := worktree.Commit("Foo commit", &git.CommitOptions{
					Author: &object.Signature{
						Name:  "John Doe",
						Email: "john@doe.org",
						When:  time.Now(),
					},
				})
				assert.NilError(t, err, "failed to commit")

				err = clonedRepo.Push(&git.PushOptions{
					Force: true,
				})
				assert.NilError(t, err, "failed to force push")

				t.Run(fmt.Sprintf("commit %d", j), func(t *testing.T) {
					tc.fetchFunc(t, repo, newCommit)
				})
			}
		})
	}
}
