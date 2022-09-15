// Copyright 2018 Palantir Technologies, Inc.
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

package pull

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
)

// GraphQLNodePrefixMatcher matches if the GraphQL query in the request selects
// a node that contains the matcher's prefix in its path. Prefixes are
// period-separated strings where each element is a level in the query tree.
// For example, the prefix:
//
//	repository.pullRequest.timeline
//
// corresponds to this GraphQL query:
//
//	repository(...) {
//	  pullRequest(...) {
//	    timeline(...) {
//	      ...
//	    }
//	  }
//	}
//
// If the query contains type conditions, fields selects for a specific type
// can by added to the prefix by adding the type name as an element.
type GraphQLNodePrefixMatcher string

func (m GraphQLNodePrefixMatcher) Matches(r *http.Request, body []byte) bool {
	if r.URL.Path != "/graphql" {
		return false
	}

	var d struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return false
	}

	query, err := parser.ParseQuery(&ast.Source{Input: d.Query})
	if err != nil {
		return false
	}

	names := extractNodeNames(query.Operations[0].SelectionSet)
	return names.HasPrefix(strings.Split(string(m), "."))
}

type nodeNameTree map[string]nodeNameTree

func (t nodeNameTree) HasPrefix(prefix []string) bool {
	current := t
	for _, name := range prefix {
		if next, exists := current[name]; exists {
			current = next
		} else {
			return false
		}
	}
	return true
}

func extractNodeNames(set ast.SelectionSet) nodeNameTree {
	if len(set) == 0 {
		return nil
	}

	t := make(nodeNameTree)
	for _, sel := range set {
		switch f := sel.(type) {
		case *ast.Field:
			t[f.Name] = extractNodeNames(f.SelectionSet)
		case *ast.InlineFragment:
			t[f.TypeCondition] = extractNodeNames(f.SelectionSet)
		}
	}
	return t
}
