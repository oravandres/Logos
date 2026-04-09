# Logos

# Engineering Standards

## Toolchain
- Go version is pinned in `go.mod`
- Local development should use the version declared by the repository
- CI is the source of truth for merge quality

## Local quality gate
Run before pushing:

```bash
make verify
```
