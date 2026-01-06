---
description: 'High-signal Go coding standards emphasizing simplicity and idiomatic patterns.'
applyTo: '**/*.go,**/go.mod'
---

# Go Engineering Standards

## Core Philosophy
- **Simplicity First:** Prioritize the "Go way"—explicit, readable, and minimalist. 
- **Happy Path:** Keep it left-aligned. Use early returns to minimize nesting.
- **No Cleverness:** Avoid complex abstractions, deep embedding, or unnecessary concurrency.

## Implementation Details
- **Standard Library:** Default to the stdlib (e.g., `net/http`, `encoding/json`, `errors`). Avoid third-party "frameworks" unless they are already in the project.
- **Error Handling:** Always use explicit `if err != nil`. No "clever" helper functions that hide error flow.
- **Testing:** Use table-driven tests by default. Place tests in `_test.go` files within the same package.
- **Interfaces:** Accept interfaces, return concrete types. Keep interfaces small (1-3 methods).
- **Comments:** English only. Document *why*, not *what*. Code should be self-documenting.

## Naming & Structure
- **Conciseness:** Use short, descriptive names. Avoid "stuttering" (e.g., `server.Server`, not `server.HTTPServer`).
- **Package Design:** Single-word, lowercase package names. No `util` or `common` packages.
- **Zero Values:** Design structs so the zero value is useful/ready to use.

## Performance (Context-Specific)
- Preallocate slices with `make([]T, 0, len)` if size is known.
- Avoid pointers for small structs or basic types unless mutability or `nil` signaling is required.
