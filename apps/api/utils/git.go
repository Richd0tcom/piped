package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/richd0tcom/piped/internals/models"
)


func CloneRepo(ctx context.Context, repoURL string, opts models.ImportOptions) (string, error) {

	repoPath := fmt.Sprintf("%s/repo-%d", models.TempDir, time.Now().Unix())

	cloneOpts:= git.CloneOptions{
		URL: repoURL,
		Progress: nil, 
		Depth: 1, 
	}

	if opts.AuthToken != "" {
		cloneOpts.Auth = &http.BasicAuth{
			Username: "token", // Can be anything for token auth
			Password: opts.AuthToken,
		}
	}

	_, err := git.PlainCloneContext(ctx, repoPath, false, &cloneOpts)
	if err != nil {
		return "", err
	}

	return repoPath, nil
}