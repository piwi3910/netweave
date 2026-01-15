# GraphQL API Implementation Plan - Issue #249

## Status: IN PROGRESS - Foundation Established

This document outlines the implementation plan for GraphQL API support as described in issue #249.

## Completed Work

### 1. GraphQL Schema Design ✅
**File:** `internal/graphql/schema.graphql`

Comprehensive GraphQL schema covering:
- **O2-IMS Resources**: Deployment Managers, Resource Pools, Resources, Resource Types, Subscriptions
- **O2-DMS**: NF Deployments, Deployment Descriptors, Status tracking
- **O2-SMO**: Workflows, Service Models, Policies
- **Multi-tenancy (v3)**: Tenants, Quotas, Usage tracking
- **Real-time subscriptions**: Resource events, deployment events, workflow events
- **Batch operations (v2)**: Batch create/delete for resources and deployments
- **Cursor-based pagination**: Connection pattern with PageInfo

Key features:
- Flexible nested queries (get pools with resources in one query)
- Field selection (request only needed fields)
- Filter inputs for all entity types
- Proper error handling types
- Mutation inputs with validation

### 2. gqlgen Configuration ✅
**File:** `gqlgen.yml`

Configured gqlgen code generator with:
- Schema location mapping
- Generated code output paths
- Resolver layout (follow-schema pattern)
- Type mappings to existing adapter types
- Custom scalar mappings (Time, JSON)

### 3. Code Generation Attempt ⚠️
Ran `go run github.com/99designs/gqlgen generate` which:
- ✅ Generated resolver stubs at `internal/graphql/resolvers/`
- ✅ Generated models at `internal/graphql/model/models_gen.go`
- ✅ Generated execution runtime at `internal/graphql/generated/generated.go`
- ⚠️ Has compilation errors that need custom scalar marshaling

## Remaining Work

### Phase 1: Fix Code Generation (HIGH PRIORITY)

**Issue:** Custom scalar types (Time, JSON, WorkflowStatus, DeploymentStatus) need custom marshalers.

**Solution:**
1. Create `internal/graphql/scalars/scalars.go`:
```go
package scalars

import (
    "encoding/json"
    "fmt"
    "io"
    "time"
    "github.com/99designs/gqlgen/graphql"
)

func MarshalTime(t time.Time) graphql.Marshaler {
    return graphql.WriterFunc(func(w io.Writer) {
        io.WriteString(w, fmt.Sprintf(`"%s"`, t.Format(time.RFC3339)))
    })
}

func UnmarshalTime(v interface{}) (time.Time, error) {
    if str, ok := v.(string); ok {
        return time.Parse(time.RFC3339, str)
    }
    return time.Time{}, fmt.Errorf("time must be a string")
}

func MarshalJSON(v map[string]interface{}) graphql.Marshaler {
    return graphql.WriterFunc(func(w io.Writer) {
        json.NewEncoder(w).Encode(v)
    })
}

func UnmarshalJSON(v interface{}) (map[string]interface{}, error) {
    if m, ok := v.(map[string]interface{}); ok {
        return m, nil
    }
    return nil, fmt.Errorf("JSON must be an object")
}
```

2. Update `gqlgen.yml` to reference these marshalers:
```yaml
models:
  Time:
    model:
      - time.Time
    marshaler:
      - github.com/piwi3910/netweave/internal/graphql/scalars.MarshalTime
    unmarshaler:
      - github.com/piwi3910/netweave/internal/graphql/scalars.UnmarshalTime
  JSON:
    model:
      - map[string]interface{}
    marshaler:
      - github.com/piwi3910/netweave/internal/graphql/scalars.MarshalJSON
    unmarshaler:
      - github.com/piwi3910/netweave/internal/graphql/scalars.UnmarshalJSON
```

3. Re-run code generation

### Phase 2: Implement Core Query Resolvers (HIGH PRIORITY)

**Files to implement:**
- `internal/graphql/resolvers/query.resolvers.go`
- `internal/graphql/resolvers/resource_pool.resolvers.go`
- `internal/graphql/resolvers/resource.resolvers.go`
- `internal/graphql/resolvers/resource_type.resolvers.go`

**Key implementation pattern:**
```go
func (r *queryResolver) ResourcePools(ctx context.Context, filter *model.ResourcePoolFilter, pagination *model.Pagination) (*model.ResourcePoolConnection, error) {
    // 1. Convert GraphQL filter to adapter filter
    adapterFilter := convertResourcePoolFilter(filter)

    // 2. Call existing adapter
    pools, err := r.adapter.ListResourcePools(ctx, adapterFilter)
    if err != nil {
        return nil, err
    }

    // 3. Apply pagination and build connection
    return buildResourcePoolConnection(pools, pagination), nil
}
```

### Phase 3: Implement Mutation Resolvers (MEDIUM PRIORITY)

**Files:**
- `internal/graphql/resolvers/mutation.resolvers.go`

Delegate to existing adapters:
```go
func (r *mutationResolver) CreateResourcePool(ctx context.Context, input model.CreateResourcePoolInput) (*adapter.ResourcePool, error) {
    pool := &adapter.ResourcePool{
        Name:        input.Name,
        Description: *input.Description,
        Location:    *input.Location,
        OCloudID:    input.OCloudID,
    }
    return r.adapter.CreateResourcePool(ctx, pool)
}
```

### Phase 4: GraphQL Server Setup (MEDIUM PRIORITY)

**File:** `internal/graphql/server.go`

```go
package graphql

import (
    "github.com/99designs/gqlgen/graphql/handler"
    "github.com/99designs/gqlgen/graphql/handler/extension"
    "github.com/99designs/gqlgen/graphql/handler/transport"
    "github.com/99designs/gqlgen/graphql/playground"
    "github.com/piwi3910/netweave/internal/graphql/generated"
    "github.com/piwi3910/netweave/internal/graphql/resolvers"
    "time"
)

func NewServer(resolver *resolvers.Resolver) *handler.Server {
    srv := handler.NewDefaultServer(
        generated.NewExecutableSchema(
            generated.Config{Resolvers: resolver},
        ),
    )

    // Enable introspection
    srv.Use(extension.Introspection{})

    // Enable automatic query complexity limits
    srv.Use(extension.AutomaticPersistedQuery{
        Cache: lru.New(100),
    })

    // Add WebSocket transport for subscriptions
    srv.AddTransport(transport.Websocket{
        KeepAlivePingInterval: 10 * time.Second,
    })

    return srv
}
```

### Phase 5: Server Integration (MEDIUM PRIORITY)

**File:** `internal/server/graphql_routes.go`

```go
package server

import (
    "github.com/gin-gonic/gin"
    gqlserver "github.com/piwi3910/netweave/internal/graphql"
    "github.com/piwi3910/netweave/internal/graphql/resolvers"
)

func (s *Server) setupGraphQLRoutes() {
    resolver := resolvers.NewResolver(s.adapter, s.store, s.dmsHandler, s.smoHandler)
    gqlSrv := gqlserver.NewServer(resolver)

    // GraphQL endpoint
    s.router.POST("/graphql", gin.WrapH(gqlSrv))

    // GraphQL playground UI (only in dev mode)
    if s.config.Server.GinMode != "release" {
        s.router.GET("/graphql", gin.WrapH(playground.Handler("GraphQL playground", "/graphql")))
    }
}
```

### Phase 6: Implement Subscription Resolvers (LOW PRIORITY)

**File:** `internal/graphql/resolvers/subscription.resolvers.go`

Connect to existing event system:
```go
func (r *subscriptionResolver) ResourceCreated(ctx context.Context, poolID *string, typeID *string) (<-chan *adapter.Resource, error) {
    ch := make(chan *adapter.Resource)

    // Subscribe to events
    sub := r.eventBus.Subscribe("resource.created", func(event *Event) {
        if resource, ok := event.Data.(*adapter.Resource); ok {
            // Apply filters
            if poolID != nil && resource.ResourcePoolID != *poolID {
                return
            }
            if typeID != nil && resource.ResourceTypeID != *typeID {
                return
            }
            ch <- resource
        }
    })

    go func() {
        <-ctx.Done()
        r.eventBus.Unsubscribe(sub)
        close(ch)
    }()

    return ch, nil
}
```

### Phase 7: Testing (HIGH PRIORITY)

**Files to create:**
- `internal/graphql/resolvers/query_test.go`
- `internal/graphql/resolvers/mutation_test.go`
- `internal/graphql/integration_test.go`

Test patterns:
```go
func TestQueryResourcePools(t *testing.T) {
    // 1. Create test resolver with mock adapter
    mockAdapter := &MockAdapter{}
    resolver := resolvers.NewResolver(mockAdapter, nil, nil, nil)

    // 2. Execute GraphQL query
    resp := executeQuery(t, resolver, `
        query {
            resourcePools(filter: {location: "us-east-1"}) {
                edges {
                    node {
                        id
                        name
                        resourceCount
                    }
                }
            }
        }
    `)

    // 3. Assert results
    assert.NoError(t, resp.Errors)
    assert.Equal(t, 2, len(resp.Data.ResourcePools.Edges))
}
```

### Phase 8: Authentication & Authorization (HIGH PRIORITY)

Add auth middleware to GraphQL context:
```go
func AuthMiddleware(authMw AuthMiddleware) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Validate JWT token
        claims, err := authMw.ValidateToken(c)
        if err != nil {
            c.AbortWithStatus(401)
            return
        }

        // Add to GraphQL context
        ctx := context.WithValue(c.Request.Context(), "user", claims)
        c.Request = c.Request.WithContext(ctx)
        c.Next()
    }
}
```

Check permissions in resolvers:
```go
func (r *mutationResolver) DeleteResourcePool(ctx context.Context, id string) (bool, error) {
    user := auth.GetUser(ctx)
    if !user.HasPermission("resource-pools:delete") {
        return false, errors.New("permission denied")
    }
    return r.adapter.DeleteResourcePool(ctx, id) == nil, nil
}
```

### Phase 9: Performance Optimization (LOW PRIORITY)

1. **DataLoader pattern** to prevent N+1 queries:
```go
type Loaders struct {
    ResourceTypeLoader *ResourceTypeLoader
}

func (l *ResourceTypeLoader) Load(ctx context.Context, id string) (*adapter.ResourceType, error) {
    // Batch load resource types
}
```

2. **Query complexity limits**:
```go
srv.Use(extension.FixedComplexityLimit(1000))
```

3. **Response caching**:
```go
srv.Use(extension.AutomaticPersistedQuery{
    Cache: lru.New(1000),
})
```

### Phase 10: Documentation (MEDIUM PRIORITY)

1. Update `README.md` with GraphQL examples
2. Create `docs/graphql-api.md` with:
   - Getting started guide
   - Common query patterns
   - Subscription examples
   - Authentication setup
3. Add GraphQL playground screenshots

## Dependencies

```bash
# Already installed
go get github.com/99designs/gqlgen@latest

# Need to add for subscriptions
go get github.com/gorilla/websocket@latest

# Need for DataLoader pattern (optional)
go get github.com/graph-gophers/dataloader@latest
```

## Testing Strategy

1. **Unit tests**: Test each resolver in isolation with mocks
2. **Integration tests**: Test full GraphQL queries against test adapters
3. **Subscription tests**: Test WebSocket connections and event delivery
4. **Performance tests**: Benchmark query execution times
5. **Load tests**: Test concurrent query handling

## Rollout Plan

1. **Phase 1**: Get code generation working (IMMEDIATE)
2. **Phase 2**: Implement core query resolvers (WEEK 1)
3. **Phase 3**: Add mutations (WEEK 1-2)
4. **Phase 4**: Server integration (WEEK 2)
5. **Phase 5**: Add auth (WEEK 2)
6. **Phase 6**: Add subscriptions (WEEK 3)
7. **Phase 7**: Testing & docs (WEEK 3-4)
8. **Phase 8**: Production deployment (WEEK 4)

## Example Queries

### Get Resource Pools with Nested Resources
```graphql
query GetPoolsWithResources {
  resourcePools(filter: {location: "us-east-1"}) {
    edges {
      node {
        id
        name
        resources(pagination: {limit: 5}) {
          edges {
            node {
              id
              resourceType {
                name
                vendor
              }
            }
          }
        }
      }
    }
  }
}
```

### Create Resource with Nested Type Info
```graphql
mutation CreateResource {
  createResource(input: {
    resourceTypeId: "type-gpu"
    resourcePoolId: "pool-123"
    description: "NVIDIA A100 GPU"
  }) {
    id
    resourceType {
      name
      vendor
    }
    resourcePool {
      name
      location
    }
  }
}
```

### Subscribe to Real-time Events
```graphql
subscription WatchResources {
  resourceCreated(poolId: "pool-123") {
    id
    resourceType {
      name
    }
  }
}
```

## Related Issues

- #249: This issue
- #243: O2-DMS v2/v3 (provides DMS entities for GraphQL)
- #244: O2-SMO v2/v3 (provides SMO entities for GraphQL)
- #245: Advanced filtering (complements GraphQL filtering)

## Notes

- GraphQL API runs **in parallel** with REST API (not a replacement)
- Reuses all existing adapter layer (no duplication)
- Reuses existing auth middleware
- Provides better flexibility for frontend clients
- Self-documenting via introspection
- GraphQL playground available in dev mode at `/graphql`

## Next Steps

1. Fix scalar marshaling (see Phase 1)
2. Implement core resolvers (see Phase 2)
3. Add basic tests
4. Integrate with server
5. Document usage

## Contact

For questions about this implementation, see issue #249 or the README GraphQL section (line 1038).
