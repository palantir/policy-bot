// Copyright 2025 Palantir Technologies, Inc.
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

package options

import (
	"context"
)

type contextKey string

const (
	ignoreEditedCommentsKey contextKey = "ignore-edited-comments"
)

// WithIgnoreEditedComments returns a new context with the IgnoreEditedComments option set.
func WithIgnoreEditedComments(ctx context.Context, ignore bool) context.Context {
	return context.WithValue(ctx, ignoreEditedCommentsKey, ignore)
}

// ShouldIgnoreEditedComments returns whether edited comments should be ignored based on the context.
// Returns false if the option is not set in the context.
func ShouldIgnoreEditedComments(ctx context.Context) bool {
	if v, ok := ctx.Value(ignoreEditedCommentsKey).(bool); ok {
		return v
	}
	return false
}
