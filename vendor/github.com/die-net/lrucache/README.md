LruCache [![Build Status](https://github.com/die-net/lrucache/actions/workflows/go-test.yml/badge.svg)](https://github.com/die-net/lrucache/actions/workflows/go-test.yml) [![Coverage Status](https://coveralls.io/repos/github/die-net/lrucache/badge.svg?branch=main)](https://coveralls.io/github/die-net/lrucache?branch=main) [![Go Report Card](https://goreportcard.com/badge/github.com/die-net/lrucache)](https://goreportcard.com/report/github.com/die-net/lrucache)
========

## This project is now archived, since `httpcache.Cache` is itself archived, and I haven't used this in years.

---

LruCache is a thread-safe, in-memory [httpcache.Cache](https://github.com/gregjones/httpcache) implementation that evicts the least recently used entries when a byte size limit or optional max age would be exceeded.

Using the included [TwoTier](https://github.com/die-net/lrucache/tree/main/twotier) wrapper, it could also be used as a small and fast cache for popular objects, falling back to a larger and slower cache (such as [s3cache](https://github.com/sourcegraph/s3cache)) for less popular ones.

Also see the godoc API documentation for [LruCache](https://godoc.org/github.com/die-net/lrucache) or [TwoTier](https://godoc.org/github.com/die-net/lrucache/twotier).

Included are a test suite with close to 100% test coverage and a parallel benchmark suite that shows individual Set, Get, and Delete operations take under 400ns to complete.

License
-------

Copyright 2016 Aaron Hopkins and contributors

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License at: http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.
