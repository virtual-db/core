# Contributing to core

> [!IMPORTANT]
> ### Public Contributions Are Not Yet Open
>
> VirtualDB is not currently accepting public pull requests. We intend to open the project to community contributions in the future — when we do, this document will be updated accordingly.
>
> **In the meantime, we encourage you to participate by [opening a GitHub Issue](https://github.com/virtual-db/core/issues).** Issues are the best way to report bugs, request features, ask questions, and start discussions with the team.
>
> Thank you for your interest in VirtualDB.

---

## Development Guidelines

- All public API changes must be reflected in the README.
- All new `internal/` packages must have a package-level doc comment.
- Pipeline handlers must be named methods on a `Handlers` struct — anonymous functions inline in `Register` are not permitted.
- Built-in handlers register at priority 10. Do not register built-in handlers at any other priority without updating the priority table in the README.
- Tests are black-box (`package core_test`) at the root level and white-box within `internal/` sub-packages.

## Running Tests

```sh
go test ./...
```
