package regclient

import (
	"context"

	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/repo"
)

type repoLister interface {
	RepoList(ctx context.Context, hostname string, opts ...scheme.RepoOpts) (*repo.RepoList, error)
}

func (rc *RegClient) RepoList(ctx context.Context, hostname string, opts ...scheme.RepoOpts) (*repo.RepoList, error) {
	schemeAPI, err := rc.getScheme("reg")
	if err != nil {
		return nil, err
	}
	rl, ok := schemeAPI.(repoLister)
	if !ok {
		return nil, types.ErrNotImplemented
	}
	return rl.RepoList(ctx, hostname, opts...)

}
