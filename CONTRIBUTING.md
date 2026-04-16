# Contributing

Contributions are welcome. By submitting a Contribution you accept the terms of the [Contributor License Agreement](CLA.md).

## Development guidelines

- All public API changes must be reflected in the README.
- All new `internal/` packages must have a package-level doc comment.
- Pipeline handlers must be named methods on a `Handlers` struct -- anonymous functions inline in `Register` are not permitted.
- Built-in handlers register at priority 10. Do not register built-in handlers at any other priority without updating the priority table in the README.
- Tests are black-box (`package core_test`) at the root level and white-box within `internal/` sub-packages.

## Running tests

```sh
go test ./...
```
