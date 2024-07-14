// Copyright 2019 Palantir Technologies, Inc.
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

package predicate

import (
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
)

func assertPredicateResult(t *testing.T, expected, actual *common.PredicateResult) {
	assert.Equal(t, expected.Satisfied, actual.Satisfied, "predicate was not correct")
	assert.Equal(t, expected.Values, actual.Values, "values were not correct")
	assert.Equal(t, expected.ConditionsMap, actual.ConditionsMap, "conditions were not correct")
	assert.Equal(t, expected.ConditionValues, actual.ConditionValues, "conditions were not correct")
}
