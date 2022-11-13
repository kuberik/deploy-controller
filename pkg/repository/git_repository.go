package repository

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

const (
	commitsDir  = "commits"
	repoDirName = "repo"
)

type GitRepository struct {
	repo git.Repository
	auth transport.AuthMethod
	root string
}

func InitGitRepository(dir string, url string, auth transport.AuthMethod) (*GitRepository, error) {
	repoDir := path.Join(dir, repoDirName)
	r, err := git.PlainInit(repoDir, true)
	if err != nil {
		if err == git.ErrRepositoryAlreadyExists {
			if r, err := git.PlainOpen(repoDir); err == nil {
				return &GitRepository{
					repo: *r,
					auth: auth,
					root: repoDir,
				}, err
			}
		}
		return nil, err
	}

	_, err = r.CreateRemote(&config.RemoteConfig{
		Name: git.DefaultRemoteName,
		URLs: []string{url},
	})
	if err != nil {
		return nil, err
	}

	if err = os.Mkdir(path.Join(dir, commitsDir), 0775); err != nil {
		return nil, err
	}

	return &GitRepository{
		repo: *r,
		auth: auth,
		root: repoDir,
	}, nil
}

func (gr *GitRepository) FetchBranch(name string) (*plumbing.Hash, error) {
	branchRefSpec := config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", name, git.DefaultRemoteName, name))
	err := gr.repo.Fetch(&git.FetchOptions{
		Depth:    1,
		Auth:     gr.auth,
		RefSpecs: []config.RefSpec{branchRefSpec},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, err
	}

	branchReference, err := gr.repo.Reference(plumbing.NewRemoteReferenceName(git.DefaultRemoteName, name), true)
	if err != nil {
		return nil, err
	}

	hash := branchReference.Hash()
	return &hash, nil
}

func (gr *GitRepository) FetchCommit(commit string) error {
	if _, err := gr.repo.CommitObject(plumbing.NewHash(commit)); err == nil {
		return nil
	}
	branchRefSpec := config.RefSpec(fmt.Sprintf("%s:refs/remotes/%s/%s", commit, git.DefaultRemoteName, fmt.Sprintf("commit-%s", commit)))
	return gr.repo.Fetch(&git.FetchOptions{
		Depth:    1,
		Auth:     gr.auth,
		Force:    true,
		RefSpecs: []config.RefSpec{branchRefSpec},
	})
}

func (gr *GitRepository) CreateCommitDir(commit plumbing.Hash) (string, error) {
	commitDir := path.Join(path.Dir(gr.root), commitsDir, commit.String())
	repo, err := git.Open(gr.repo.Storer, osfs.New(commitDir))
	if err != nil {
		return "", err
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return "", err
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Hash:  commit,
		Force: true,
	})
	if err != nil {
		return "", err
	}

	return commitDir, nil
}

func (gr *GitRepository) ListBranches(match string) ([]string, error) {
	remote, err := gr.repo.Remote(git.DefaultRemoteName)
	if err != nil {
		return nil, err
	}
	refs, err := remote.List(&git.ListOptions{
		Auth: gr.auth,
	})
	if err != nil {
		return nil, err
	}

	matcher, err := regexp.Compile(match)
	if err != nil {
		return nil, err
	}

	branches := []string{}
	for _, ref := range refs {
		if ref.Name().IsBranch() && matcher.MatchString(ref.Name().Short()) {
			branches = append(branches, ref.Name().Short())
		}
	}
	sort.Strings(branches)
	return branches, nil
}
