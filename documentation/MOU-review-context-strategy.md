# Memorandum of Understanding: Review Context Strategy

## Section 1. Status

This document records an architectural decision that remains deferred. No implementation work described below has begun. This memorandum exists to preserve the analysis for future reference.

## Section 2. Current Architecture

The `claude-review` and `gemini-review` jobs construct their prompt from the annotated diff of changed files in the merge request under review, not from the full repository content. The prompt template in `internal/review/review.go` states this explicitly, each file section carries only the diff hunk with line number annotations, and the total diff is capped at 300000 characters (`maxTotalDiff`).

## Section 3. Considered Alternative

An alternative design would supply the full repository content as context on every review invocation, so that Claude could reason about the change in relation to the entire codebase rather than the diff alone. This alternative was not implemented. It was raised as a question during a design discussion and is recorded here for future evaluation.

## Section 4. Feasibility Findings

### Task A. Context Window

Claude Sonnet 5 provides a 1000000 token context window, which SHOULD accommodate the full content of a repository of this project's scale. The actual token count MUST be measured with the `count_tokens` endpoint before any implementation begins, rather than assumed.

### Task B. Cache Anchor Selection

Prompt caching operates on exact prefix matching, whereby any byte difference anywhere in the cached prefix invalidates every cache entry positioned after it. Anchoring the cached content to the merge request's own working tree would invalidate the cache on every commit pushed to that merge request, because the working tree content changes with each push. The cached content SHOULD instead be anchored to the target branch (`main`) content, positioned before the cache breakpoint, with the merge request diff placed after the breakpoint. Under this arrangement, the cached prefix remains stable across every merge request opened between two merges to `main`, and invalidates only when `main` itself changes.

### Task C. Economic Viability

Prompt cache writes cost 1.25 times the standard input rate at the default five minute time to live, or 2 times at a one hour time to live. Cache reads cost approximately 0.1 times the standard rate. Under a five minute time to live, at least two cache reads within the window are required to recover the write premium; under a one hour time to live, at least three reads are required. Whether this threshold is met depends on the actual review request volume relative to the repository's merge cadence, which has not been measured for the repositories in scope.

### Task D. Provider Scope

Prompt caching, as described here, applies only to the Claude provider. The Gemini provider maintains a separate, independently priced context caching mechanism that this document does not evaluate. Any implementation that includes the Gemini review job MUST evaluate Gemini's own caching mechanism separately.

### Task E. Implementation Cost

Supplying full repository content as context requires new logic to enumerate and serialize the repository, excluding version control metadata, build artifacts, and dependency directories that provide low signal relative to their token cost. This is a new subsystem, not an incremental change to the existing diff assembly logic.

## Section 5. Recommendation

The diff based design SHOULD be retained. The whole repository context design is technically feasible under the cache anchor described in Section 4. Task B, but its economic viability depends on unmeasured request volume, and its implementation cost is nontrivial. Streaming SHOULD be adopted for the existing diff based design independently of this decision, to resolve request timeout and output truncation risk on large diffs, and carries no token cost implication either direction.

## Section 6. Prerequisites Before Implementation

Should this alternative be pursued in the future, the following MUST be completed first.

1. Measure actual token counts for the repositories in scope using the `count_tokens` endpoint.
2. Measure actual review request volume and inter request timing for the repositories in scope.
3. Evaluate the Gemini provider's own context caching mechanism if the Gemini review job is to be included.
4. Design the repository enumeration and serialization subsystem, including its exclusion rules.
