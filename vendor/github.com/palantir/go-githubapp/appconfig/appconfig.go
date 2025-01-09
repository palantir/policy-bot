// Copyright 2021 Palantir Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package appconfig loads repository configuration for GitHub apps. It
// supports loading directly from a file in a repository, loading from remote
// references, and loading an organization-level default. The config itself can
// be in any format.
package appconfig

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/google/go-github/v68/github"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// RemoteRefParser attempts to parse a RemoteRef from bytes. The parser should
// return nil with a nil error if b does not encode a RemoteRef and nil with a
// non-nil error if b encodes an invalid RemoteRef.
type RemoteRefParser func(path string, b []byte) (*RemoteRef, error)

// RemoteRef identifies a configuration file in a different repository.
type RemoteRef struct {
	// The repository in "owner/name" format. Required.
	Remote string `yaml:"remote" json:"remote"`

	// The path to the config file in the repository. If empty, use the first
	// path configured in the loader.
	Path string `yaml:"path" json:"path"`

	// The reference (branch, tag, or SHA) to read in the repository. If empty,
	// use the default branch of the repository.
	Ref string `yaml:"ref" json:"ref"`
}

func (r RemoteRef) SplitRemote() (owner, repo string, err error) {
	slash := strings.IndexByte(r.Remote, '/')
	if slash <= 0 || slash >= len(r.Remote)-1 {
		return "", "", errors.Errorf("invalid remote value: %s", r.Remote)
	}
	return r.Remote[:slash], r.Remote[slash+1:], nil
}

// Config contains unparsed configuration data and metadata about where it was found.
type Config struct {
	Content []byte

	// Source contains the repository and ref in "owner/name@ref" format. The
	// ref component ("@ref") is optional and may not be present.
	Source   string
	Path     string
	IsRemote bool
}

// IsUndefined returns true if the Config's content is empty and there is no
// metadata giving a source.
func (c Config) IsUndefined() bool {
	return len(c.Content) == 0 && c.Source == "" && c.Path == ""
}

// Loader loads configuration for repositories.
type Loader struct {
	paths []string

	parser       RemoteRefParser
	defaultRepo  string
	defaultPaths []string
}

// NewLoader creates a Loader that loads configuration from paths.
func NewLoader(paths []string, opts ...Option) *Loader {
	defaultPaths := make([]string, len(paths))
	for i, p := range paths {
		defaultPaths[i] = strings.TrimPrefix(p, ".github/")
	}

	ld := Loader{
		paths:        paths,
		parser:       YAMLRemoteRefParser,
		defaultRepo:  ".github",
		defaultPaths: defaultPaths,
	}

	for _, opt := range opts {
		opt(&ld)
	}

	return &ld
}

// LoadConfig loads configuration for the repository owner/repo. It first tries
// the Loader's paths in order, following remote references if they exist. If
// no configuration exists at any path in the repository, it tries to load
// default configuration defined by owner for all repositories. If no default
// configuration exists, it returns an undefined Config and a nil error.
//
// If error is non-nil, the Source and Path fields of the returned Config tell
// which file LoadConfig was processing when it encountered the error.
func (ld *Loader) LoadConfig(ctx context.Context, client *github.Client, owner, repo, ref string) (Config, error) {
	logger := zerolog.Ctx(ctx)

	c := Config{
		Source: fmt.Sprintf("%s/%s@%s", owner, repo, ref),
	}

	for _, p := range ld.paths {
		c.Path = p

		logger.Debug().Msgf("Trying configuration at %s in %s", c.Path, c.Source)
		content, exists, err := getFileContents(ctx, client, owner, repo, ref, p)
		if err != nil {
			return c, err
		}
		if !exists {
			continue
		}

		// if remote refs are enabled, see if the file is a remote reference
		if ld.parser != nil {
			remote, err := ld.parser(p, content)
			if err != nil {
				return c, err
			}
			if remote != nil {
				logger.Debug().Msgf("Found remote configuration at %s in %s", p, c.Source)
				return ld.loadRemoteConfig(ctx, client, *remote, c)
			}
		}

		// non-remote content found, don't try any other paths
		logger.Debug().Msgf("Found configuration at %s in %s", c.Path, c.Source)
		c.Content = content
		return c, nil
	}

	// if the repository defined no configuration and org defaults are enabled,
	// try falling back to the defaults
	if ld.defaultRepo != "" && len(ld.defaultPaths) > 0 {
		return ld.loadDefaultConfig(ctx, client, owner)
	}

	// couldn't find configuration anyhere, so return an empty/undefined one
	return Config{}, nil
}

func (ld *Loader) loadRemoteConfig(ctx context.Context, client *github.Client, remote RemoteRef, c Config) (Config, error) {
	logger := zerolog.Ctx(ctx)
	notFoundErr := fmt.Errorf("invalid remote reference: file does not exist")

	owner, repo, err := remote.SplitRemote()
	if err != nil {
		return c, err
	}

	path := remote.Path
	if path == "" && len(ld.paths) > 0 {
		path = ld.paths[0]
	}

	// After this point, all errors will be about the remote file, not the
	// local file containing the reference.
	c.Source = fmt.Sprintf("%s/%s", owner, repo)
	c.Path = path
	c.IsRemote = true

	ref := remote.Ref
	if ref == "" {
		// This is technically not necessary, as passing an empty ref to GitHub
		// uses the default branch. However, callers may expect the Source
		// field in the Config we return to have a non-empty ref.
		r, _, err := client.Repositories.Get(ctx, owner, repo)
		if err != nil {
			if isNotFound(err) {
				return c, notFoundErr
			}
			return c, errors.Wrap(err, "failed to get remote repository")
		}
		ref = r.GetDefaultBranch()
	}
	c.Source = fmt.Sprintf("%s@%s", c.Source, ref)

	logger.Debug().Msgf("Trying remote configuration at %s in %s", c.Path, c.Source)
	content, exists, err := getFileContents(ctx, client, owner, repo, ref, c.Path)
	if err != nil {
		return c, err
	}
	if !exists {
		// Referencing a remote file that does not exist is an error because
		// this condition is annoying to debug otherwise. From the perspective
		// of a repository, it appears that the application has a configuration
		// file and it is easy to miss that e.g. the ref is wrong.
		return c, notFoundErr
	}

	c.Content = content
	return c, nil
}

func (ld *Loader) loadDefaultConfig(ctx context.Context, client *github.Client, owner string) (Config, error) {
	logger := zerolog.Ctx(ctx)

	r, _, err := client.Repositories.Get(ctx, owner, ld.defaultRepo)
	if err != nil {
		if isNotFound(err) {
			// if the owner has no default repo, return empty/undefined config
			return Config{}, nil
		}
		c := Config{Source: fmt.Sprintf("%s/%s", owner, ld.defaultRepo)}
		return c, errors.Wrap(err, "failed to get default repository")
	}

	ref := r.GetDefaultBranch()
	c := Config{
		Source: fmt.Sprintf("%s/%s@%s", owner, r.GetName(), ref),
	}

	for _, p := range ld.defaultPaths {
		c.Path = p

		logger.Debug().Msgf("Trying default configuration at %s in %s", c.Path, c.Source)
		content, exists, err := getFileContents(ctx, client, owner, r.GetName(), ref, p)
		if err != nil {
			return c, err
		}
		if !exists {
			continue
		}

		// if remote refs are enabled, see if the file is a remote reference
		if ld.parser != nil {
			remote, err := ld.parser(p, content)
			if err != nil {
				return c, err
			}
			if remote != nil {
				logger.Debug().Msgf("Found remote default configuration at %s in %s", p, c.Source)
				return ld.loadRemoteConfig(ctx, client, *remote, c)
			}
		}

		// non-remote content found, don't try any other paths
		logger.Debug().Msgf("Found default configuration at %s in %s", c.Path, c.Source)
		c.Content = content
		return c, nil
	}

	// no default configuration, return an empty/undefined one
	return Config{}, nil
}

// getFileContents returns the content of the file at path on ref in owner/repo
// if it exists. Returns an empty slice and false if the file does not exist.
func getFileContents(ctx context.Context, client *github.Client, owner, repo, ref, path string) ([]byte, bool, error) {
	file, _, _, err := client.Repositories.GetContents(ctx, owner, repo, path, &github.RepositoryContentGetOptions{
		Ref: ref,
	})
	if err != nil {
		switch {
		case isNotFound(err):
			return nil, false, nil
		case isTooLargeError(err):
			b, err := getLargeFileContents(ctx, client, owner, repo, ref, path)
			return b, true, err
		}
		return nil, false, errors.Wrap(err, "failed to read file")
	}

	// file will be nil if the path exists but is a directory
	if file == nil {
		return nil, false, nil
	}

	content, err := file.GetContent()
	if err != nil {
		return nil, true, errors.Wrap(err, "failed to decode file content")
	}

	return []byte(content), true, nil
}

// getLargeFileContents is similar to getFileContents, but works for files up
// to 100MB. Unlike getFileContents, it returns an error if the file does not
// exist.
func getLargeFileContents(ctx context.Context, client *github.Client, owner, repo, ref, path string) ([]byte, error) {
	body, res, err := client.Repositories.DownloadContents(ctx, owner, repo, path, &github.RepositoryContentGetOptions{
		Ref: ref,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to read file")
	}
	defer func() {
		_ = body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to read file: unexpected status code %d", res.StatusCode)
	}

	b, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read file")
	}
	return b, nil
}

func isNotFound(err error) bool {
	if rerr, ok := err.(*github.ErrorResponse); ok {
		return rerr.Response.StatusCode == http.StatusNotFound
	}
	return false
}

func isTooLargeError(err error) bool {
	if rerr, ok := err.(*github.ErrorResponse); ok {
		for _, err := range rerr.Errors {
			if err.Code == "too_large" {
				return true
			}
		}
	}
	return false
}
