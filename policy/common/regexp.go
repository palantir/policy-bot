// Copyright 2020 Palantir Technologies, Inc.
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

import (
	"encoding/json"
	"regexp"
)

// Regexp is a regexp.Regexp that only supports matching and can be
// deserialized from a string.
type Regexp struct {
	r *regexp.Regexp
}

func NewRegexp(pattern string) (Regexp, error) {
	r, err := regexp.Compile(pattern)
	if err != nil {
		return Regexp{}, err
	}
	return Regexp{r: r}, nil
}

func NewCompiledRegexp(r *regexp.Regexp) Regexp {
	return Regexp{r: r}
}

func (r Regexp) Matches(s string) bool {
	if r.r == nil {
		return false
	}
	return r.r.MatchString(s)
}

func (r Regexp) String() string {
	if r.r == nil {
		return ""
	}
	return r.r.String()
}

func (r *Regexp) UnmarshalYAML(unmarshal func(interface{}) error) (err error) {
	var pattern string
	if err := unmarshal(&pattern); err != nil {
		return err
	}
	*r, err = NewRegexp(pattern)
	return err
}

func (r *Regexp) UnmarshalJSON(data []byte) (err error) {
	var pattern string
	if err := json.Unmarshal(data, &pattern); err != nil {
		return err
	}
	*r, err = NewRegexp(pattern)
	return err
}
