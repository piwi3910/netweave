# Code Style and Conventions

## Naming Conventions

- **Packages**: Short, lowercase, no underscores (e.g., storage, cache, o2ims)
- **Files**: Lowercase with underscores (e.g., subscription_store.go, redis_cache.go)
- **Types**: PascalCase (e.g., O2Subscription, DeploymentManager)
- **Functions/Methods**: PascalCase for exported, camelCase for unexported
- **Constants**: PascalCase (not SCREAMING_CASE)
- **Interfaces**: -er suffix for single-method (e.g., Storer, Cacher)

## Error Handling

- Always wrap errors with context using fmt.Errorf with %w
- Use sentinel errors for specific cases (var ErrSubscriptionNotFound = errors.New(...))
- Check error types using errors.Is()
- Never ignore errors (no _ = operator)
- Return errors, don't use panic except in init/constructor panics for invariants

## Context Handling

- Always pass context as first parameter: func(ctx context.Context, ...)
- Respect context cancellation in long-running operations
- NEVER store context in struct fields

## Concurrency

- Use sync.RWMutex for shared state
- Use buffered channels appropriately
- Close channels to signal completion
- Use sync.Pool for frequently allocated objects

## Documentation

- All exported types, functions, methods MUST have GoDoc comments
- Package documentation required at top of package files
- Function docs should describe parameters, return values, and errors
- Include examples for complex APIs

## Code Organization

```
internal/           # Private application code
  adapter/          # Adapter interface and registry
  adapters/         # Concrete adapter implementations
  config/           # Configuration loading
  controller/       # Subscription controller
  handlers/         # HTTP handlers
  o2ims/            # O2-IMS models and handlers
  server/           # HTTP server setup
  storage/          # Storage implementations
  
pkg/                # Public reusable libraries
  cache/            # Cache abstraction
  storage/          # Storage abstraction
  errors/           # Error types
  
cmd/                # Entry points
  gateway/          # Main gateway binary
```

## Import Organization

1. Standard library imports
2. External dependency imports
3. Internal project imports
4. Separate type imports

## Comments

- Comments must end with period (enforced by godot linter)
- Use complete sentences
- Explain WHY, not WHAT (code shows what)
