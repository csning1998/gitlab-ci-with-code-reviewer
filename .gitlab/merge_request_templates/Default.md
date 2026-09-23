# type(scope): description

## Summary

This merge request does something ...

## Changes

### Core Architecture

1. **Feature/Change A**: Description...
2. **Feature/Change B**: Description...
3. Add below if any . . .

### Fixes & Technical Debt

- **[Issue/Module Name]**:
    - _Problem_: Describe the root cause or the error encountered.
    - _Solution_: Describe the fix implementation.
- **[Issue/Module Name]**:
    - _Problem_: ...
    - _Solution_: ...
- Add below if any . . .

### Refactoring

- **[Topic]**: Description of the refactor.
- **[Topic]**: Description of the refactor.
- Add below if any . . .

### Verification

- [ ] **Automated Tests**: Execute `go test -race -count=1 ./...` to verify unit tests and race safety.
- [ ] **Static Analysis**: Execute `golangci-lint run ./...` to verify linter compliance.
- [ ] **Formatting**: Execute `gofmt -l .` to verify Go source code formatting.
- [ ] **Quality Gate**: Execute SonarQube analysis to verify coverage and quality gate status.
- [ ] **Subprocess Execution**: Verify CLI tool execution (e.g., `cmd/review`, `cmd/mr-gate`).
