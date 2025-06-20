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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIgnoreEditedCommentsContext(t *testing.T) {
	t.Run("default value is false", func(t *testing.T) {
		ctx := context.Background()
		assert.False(t, ShouldIgnoreEditedComments(ctx))
	})

	t.Run("can set to true", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithIgnoreEditedComments(ctx, true)
		assert.True(t, ShouldIgnoreEditedComments(ctx))
	})

	t.Run("can set to false", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithIgnoreEditedComments(ctx, false)
		assert.False(t, ShouldIgnoreEditedComments(ctx))
	})

	t.Run("can override previous value", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithIgnoreEditedComments(ctx, true)
		assert.True(t, ShouldIgnoreEditedComments(ctx))

		ctx = WithIgnoreEditedComments(ctx, false)
		assert.False(t, ShouldIgnoreEditedComments(ctx))
	})
}
