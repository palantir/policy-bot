[![Go Reference][doc-image]][docs]
[![Build][build-image]][build-url]

# go-codeowners

A package that finds and parses [`CODEOWNERS`](https://help.github.com/articles/about-codeowners/) files.

Features:
- operates on local repos
- doesn't require a cloned repo (i.e. doesn't need a `.git` directory to be 
  present at the repo's root)
- can be called from within a repo (doesn't have to be at the root)
- will find `CODEOWNERS` files in all documented locations: the repo's root,
  `docs/`, and `.github/` (or `.gitlab/` for GitLab repos)

## Usage

```console
go get -u github.com/hairyhenderson/go-codeowners
```

To find the owner of the README.md file:

```go
import "github.com/hairyhenderson/go-codeowners"

func main() {
	c, _ := FromFile(cwd())
	owners := c.Owners("README.md")
	for i, o := range owners {
		fmt.Printf("Owner #%d is %s\n", i, o)
	}
}
```

See the [docs][] for more information.

## License

[The MIT License](http://opensource.org/licenses/MIT)

Copyright (c) 2018-2023 Dave Henderson

[docs]: https://pkg.go.dev/github.com/hairyhenderson/go-codeowners
[doc-image]: https://pkg.go.dev/badge/github.com/hairyhenderson/go-codeowners.svg

[build-image]: https://github.com/hairyhenderson/go-codeowners/actions/workflows/build.yml/badge.svg
[build-url]: https://github.com/hairyhenderson/go-codeowners/actions/workflows/build.yml
