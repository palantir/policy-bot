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

package common

type PredicateResult struct {
	Satisfied bool

	Description string

	// Describes the values, used as "the $ValuesPhrase"; must be plural
	ValuePhrase string
	Values      []string

	// The negation used when skipping the predicate, used as "the $ValuePhrase $SkipPhrase $ConditionPhrase"
	SkipPhrase string

	// Describes the condition, used as "$ConditionPhrase" or "does not $ConditionPhrase"
	ConditionPhrase string
	// If non-empty, use the map, otherwise, use the regular list
	ConditionsMap   map[string][]string
	ConditionValues []string
}
