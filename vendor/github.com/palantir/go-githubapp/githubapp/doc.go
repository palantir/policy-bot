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

// Package githubapp implements an http.Handler for GitHub events and provides
// utilities for building GitHub applications. Most users will create
// implementations of githubapp.EventHandler to handle different webhook event
// types and register them with the event dispatcher.
//
// Many functions are instrumented with optional logging and metrics
// collection. The package also defines functions to create authenticated
// GitHub clients and manage application installations.
package githubapp
