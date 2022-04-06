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

type PredicateInfo struct {
	Name string
	Type string

	ContributorInfo *ContributorInfo
	BranchInfo      *BranchInfo
	FileInfo        *FileInfo
	LabelInfo       *LabelInfo
	CommitInfo      *CommitInfo
	StatusInfo      *StatusInfo
	TitleInfo       *TitleInfo
}

type ContributorInfo struct {
	Organizations []string
	Teams         []string
	Users         []string
	Author        string
	Contributors  []string
}

type BranchInfo struct {
	Patterns []string
	Branch   string
}

type FileInfo struct {
	Paths              []string
	IgnorePaths        []string
	ChangedFiles       []string
	AdditionLimit      string
	DeletionLimit      string
	TotalLimit         string
	AddedLines         int64
	DeletedLines       int64
	TotalModifiedLines int64
}

type LabelInfo struct {
	RequiredLabels []string
	PRLabels       []string
}

type CommitInfo struct {
	Organizations []string
	Teams         []string
	Users         []string
	Signers       []string
	CommitHashes  []string
	Keys          []string
}

type StatusInfo struct {
	Type   string
	Status []string
}

type TitleInfo struct {
	MatchPatterns    []string
	NotMatchPatterns []string
	PRTitle          string
}
