# Changelog

## [0.7.0](https://github.com/hairyhenderson/go-codeowners/compare/v0.6.1...v0.7.0) (2024-12-07)


### Features

* add PatternIndex method ([#48](https://github.com/hairyhenderson/go-codeowners/issues/48)) ([a5c2e59](https://github.com/hairyhenderson/go-codeowners/commit/a5c2e593dcbe38fa52c24c95a72d1ca3f5e47ee8))


### Dependencies

* **go:** Bump github.com/stretchr/testify from 1.9.0 to 1.10.0 ([#49](https://github.com/hairyhenderson/go-codeowners/issues/49)) ([4f55176](https://github.com/hairyhenderson/go-codeowners/commit/4f551769326fc31237d06b29a4a0497c78bd882f))

## [0.6.1](https://github.com/hairyhenderson/go-codeowners/compare/v0.6.0...v0.6.1) (2024-10-25)


### Bug Fixes

* **parsing:** Check for CODEOWNERS files for both GitHub and GitLab ([#45](https://github.com/hairyhenderson/go-codeowners/issues/45)) ([db9c01e](https://github.com/hairyhenderson/go-codeowners/commit/db9c01eb61a0e5975521721a102e8c54b6dc2876)), closes [#44](https://github.com/hairyhenderson/go-codeowners/issues/44)
* **parsing:** Ignore block and inline comments ([#46](https://github.com/hairyhenderson/go-codeowners/issues/46)) ([968c9ea](https://github.com/hairyhenderson/go-codeowners/commit/968c9eaf0924c1912731d2729f26d2c691b8d4b1))

## [0.6.0](https://github.com/hairyhenderson/go-codeowners/compare/v0.5.0...v0.6.0) (2024-09-27)


### Features

* **errors:** Allow checking codeowners file not found ([#41](https://github.com/hairyhenderson/go-codeowners/issues/41)) ([d659b73](https://github.com/hairyhenderson/go-codeowners/commit/d659b73a08c1a1111c5e9c4c1136472c8ca28a4b))


### Bug Fixes

* **perf:** Reduce allocations ([#39](https://github.com/hairyhenderson/go-codeowners/issues/39)) ([2ca66e0](https://github.com/hairyhenderson/go-codeowners/commit/2ca66e0194d2b9077c63a9d1eace8f1083675fa9))
