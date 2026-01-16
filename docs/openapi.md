# OpenAPI Specification Usage and Extension Guide

Complete guide to using, extending, and maintaining the netweave O2-IMS OpenAPI specifications.

## Table of Contents

- [Specification Overview](#specification-overview)

- [Using the OpenAPI Specs](#using-the-openapi-specs)

- [Specification Structure](#specification-structure)

- [Extending the Specifications](#extending-the-specifications)

- [Code Generation](#code-generation)

- [Testing and Validation](#testing-and-validation)

- [Versioning Strategy](#versioning-strategy)

- [Documentation Generation](#documentation-generation)

- [CI/CD Integration](#cicd-integration)

---

## Specification Overview

### Available Specifications

netweave provides two OpenAPI 3.0.3 specifications:

| File | Purpose | API Version | Base Path |
|------|---------|-------------|-----------|
| **`api/openapi/o2ims.yaml`** | Public API spec with full documentation | 1.0.0 | `/o2ims-infrastructureInventory/v1` |
| **`internal/server/openapi/o2ims.yaml`** | Internal spec for server implementation | 1.0.0 | `/o2ims-infrastructureInventory/v1` |

### Specification Details

**O2-IMS API Specification** (`api/openapi/o2ims.yaml`):

- **Lines**: ~1,948 lines

- **Size**: ~60 KB

- **Compliance**: O-RAN O2-IMS v3.0.0 (95% compliant)

- **Features**:

  - Complete endpoint documentation with examples
  - Security schemes (mTLS, HMAC webhook signatures)
  - Rate limiting headers
  - Multi-tenancy support (v3)
  - Batch operations (v2+)

**Internal Server Specification** (`internal/server/openapi/o2ims.yaml`):

- **Size**: ~24 KB

- **Purpose**: Server-side validation and routing

- **Base Path**: O-RAN standard path format

### API Coverage

The specifications cover all O2-IMS endpoints:

**Core Resources:**

- `GET /subscriptions` - List subscriptions

- `POST /subscriptions` - Create subscription

- `GET /subscriptions/{id}` - Get subscription

- `DELETE /subscriptions/{id}` - Delete subscription

- `GET /resourcePools` - List resource pools

- `GET /resourcePools/{id}` - Get resource pool

- `GET /resources` - List resources

- `GET /resources/{id}` - Get resource

- `GET /resourceTypes` - List resource types

- `GET /resourceTypes/{id}` - Get resource type

- `GET /deploymentManagers` - List deployment managers

- `GET /deploymentManagers/{id}` - Get deployment manager

- `GET /` - Get O-Cloud infrastructure info

**Extended Resources (v2+):**

- `POST /batch/resources` - Batch create resources

- `PATCH /batch/resources` - Batch update resources

- `DELETE /batch/resources` - Batch delete resources

**Multi-tenancy (v3):**

- `GET /tenants` - List tenants

- `POST /tenants` - Create tenant

- `GET /tenants/{id}` - Get tenant

- `PATCH /tenants/{id}` - Update tenant

- `DELETE /tenants/{id}` - Delete tenant

- `PATCH /tenants/{id}/quotas` - Update tenant quotas

---

## Using the OpenAPI Specs

### 1. Viewing with Swagger UI

**Online (GitHub Pages):**

```bash

# Navigate to hosted Swagger UI
open https://piwi3910.github.io/netweave/api-docs/

```

**Local (Docker):**

```bash

# Run Swagger UI locally
docker run -p 8080:8080 \
  -e SWAGGER_JSON=/specs/o2ims.yaml \
  -v $(pwd)/api/openapi:/specs \
  swaggerapi/swagger-ui

# Open browser
open http://localhost:8080

```

**Local (npx):**

```bash

# Requires Node.js
npx @redocly/cli preview-docs api/openapi/o2ims.yaml

```

### 2. Generating Client SDKs

**OpenAPI Generator (Multiple Languages):**

```bash

# Install OpenAPI Generator
npm install -g @openapitools/openapi-generator-cli

# Generate Go client
openapi-generator-cli generate \
  -i api/openapi/o2ims.yaml \
  -g go \
  -o sdk/go/o2ims \
  --additional-properties=packageName=o2ims

# Generate Python client
openapi-generator-cli generate \
  -i api/openapi/o2ims.yaml \
  -g python \
  -o sdk/python/o2ims \
  --additional-properties=packageName=o2ims_client

# Generate TypeScript/JavaScript client
openapi-generator-cli generate \
  -i api/openapi/o2ims.yaml \
  -g typescript-axios \
  -o sdk/typescript/o2ims

```

**Supported Languages:**

- Go, Python, TypeScript/JavaScript

- Java, C#, Ruby, PHP

- Kotlin, Swift, Rust

- [50+ languages supported](https://openapi-generator.tech/docs/generators/)

**Using Generated Go Client:**

```go

package main

import (
    "context"
    "fmt"
    "log"

    o2ims "github.com/piwi3910/netweave/sdk/go/o2ims"
)

func main() {
    // Configure client
    cfg := o2ims.NewConfiguration()
    cfg.Host = "gateway.example.com"
    cfg.Scheme = "https"

    client := o2ims.NewAPIClient(cfg)

    // List resource pools
    pools, resp, err := client.ResourcePoolsApi.ListResourcePools(context.Background()).Execute()
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Found %d resource pools\n", len(pools.GetResourcePools()))
}

```

### 3. Validating API Requests

**Using Spectral (Recommended):**

```bash

# Install Spectral
npm install -g @stoplight/spectral-cli

# Create .spectral.yaml
cat > .spectral.yaml <<EOF
extends: ["spectral:oas"]
rules:
  oas3-api-servers: error
  operation-operationId: error
EOF

# Validate spec
spectral lint api/openapi/o2ims.yaml

```

**Using OpenAPI CLI:**

```bash

# Install Redocly CLI
npm install -g @redocly/cli

# Validate spec
redocly lint api/openapi/o2ims.yaml

# Bundle spec (resolve $refs)
redocly bundle api/openapi/o2ims.yaml -o dist/o2ims.yaml

```

**Using Swagger CLI:**

```bash

# Install Swagger CLI
npm install -g @apidevtools/swagger-cli

# Validate spec
swagger-cli validate api/openapi/o2ims.yaml

# Bundle spec
swagger-cli bundle api/openapi/o2ims.yaml -o dist/o2ims-bundled.yaml -t yaml

```

### 4. Request/Response Validation

**In Tests (Go):**

```go

import (
    "github.com/getkin/kin-openapi/openapi3"
    "github.com/getkin/kin-openapi/openapi3filter"
)

func TestAPICompliance(t *testing.T) {
    // Load OpenAPI spec
    loader := openapi3.NewLoader()
    doc, err := loader.LoadFromFile("api/openapi/o2ims.yaml")
    require.NoError(t, err)

    // Validate request
    router, err := openapi3filter.NewRouter().WithSwagger(doc)
    require.NoError(t, err)

    // Create test request
    req := httptest.NewRequest("GET", "/o2ims-infrastructureInventory/v1/resourcePools", nil)

    // Find route
    route, pathParams, err := router.FindRoute(req)
    require.NoError(t, err)

    // Validate request
    requestValidationInput := &openapi3filter.RequestValidationInput{
        Request:    req,
        PathParams: pathParams,
        Route:      route,
    }

    err = openapi3filter.ValidateRequest(context.Background(), requestValidationInput)
    require.NoError(t, err)
}

```

---

## Specification Structure

### High-Level Organization

```yaml

openapi: 3.0.3

info:                    # API metadata
  title: O2-IMS API
  version: 1.0.0
  description: |         # Markdown supported
    API overview...

servers:                 # Base URLs
  - url: /o2ims-infrastructureInventory/v1

tags:                    # Endpoint grouping
  - name: Subscriptions
  - name: Resource Pools

paths:                   # API endpoints
  /subscriptions:
    get: ...
    post: ...

components:              # Reusable schemas
  schemas: ...           # Data models
  parameters: ...        # Query/path params
  responses: ...         # Common responses
  securitySchemes: ...   # Auth methods
  examples: ...          # Example data

```

### Component Organization

**Schemas (`components/schemas`):**

```yaml

components:
  schemas:
    # Core O2-IMS models
    O2Subscription:
      type: object
      required:
        - subscriptionId
        - callback
      properties:
        subscriptionId:
          type: string
          format: uuid
        callback:
          type: string
          format: uri
          pattern: '^https://'
        filter:
          $ref: '#/components/schemas/SubscriptionFilter'

    # Pagination wrappers
    SubscriptionListResponse:
      type: object
      properties:
        subscriptions:
          type: array
          items:
            $ref: '#/components/schemas/O2Subscription'
        total:
          type: integer
          minimum: 0

```

**Parameters (`components/parameters`):**

```yaml

components:
  parameters:
    # Reusable query parameters
    OffsetParam:
      name: offset
      in: query
      schema:
        type: integer
        minimum: 0
        default: 0

    LimitParam:
      name: limit
      in: query
      schema:
        type: integer
        minimum: 1
        maximum: 1000
        default: 100

    # Path parameters
    SubscriptionIdParam:
      name: subscriptionId
      in: path
      required: true
      schema:
        type: string
        format: uuid

```

**Responses (`components/responses`):**

```yaml

components:
  responses:
    NotFound:
      description: Resource not found
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/ErrorResponse'
          example:
            code: "NOT_FOUND"
            message: "Subscription not found"

    InternalServerError:
      description: Internal server error
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/ErrorResponse'

```

**Security Schemes (`components/securitySchemes`):**

```yaml

components:
  securitySchemes:
    mTLS:
      type: mutualTLS
      description: |
        Mutual TLS authentication using client certificates.
        Clients must present valid certificates signed by a trusted CA.

    WebhookSignature:
      type: apiKey
      in: header
      name: X-Webhook-Signature
      description: |
        HMAC-SHA256 signature for webhook authentication.
        Format: sha256=<hex-digest>

```

### Schema Patterns

**Enumerations:**

```yaml

EventType:
  type: string
  enum:
    - create
    - update
    - delete
  description: Resource event type

```

**Discriminators (Polymorphism):**

```yaml

Resource:
  type: object
  discriminator:
    propertyName: resourceType
    mapping:
      compute: '#/components/schemas/ComputeResource'
      storage: '#/components/schemas/StorageResource'
  properties:
    resourceType:
      type: string

```

**Validation Constraints:**

```yaml

ResourceID:
  type: string
  minLength: 1
  maxLength: 256
  pattern: '^[a-z0-9][a-z0-9-]*[a-z0-9]$'
  example: "compute-node-1"

```

---

## Extending the Specifications

### 1. Adding New Endpoints

#### Step 1: Define the path and operations

```yaml

paths:
  /customResources:
    get:
      summary: List custom resources
      description: Retrieves a list of custom infrastructure resources
      operationId: listCustomResources
      tags:
        - Custom Resources
      parameters:
        - $ref: '#/components/parameters/OffsetParam'
        - $ref: '#/components/parameters/LimitParam'
        - name: resourceType
          in: query
          schema:
            type: string
            enum: [gpu, fpga, smartnic]
      responses:
        '200':
          description: List of custom resources
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/CustomResourceListResponse'
        '400':
          $ref: '#/components/responses/BadRequest'
        '500':
          $ref: '#/components/responses/InternalServerError'
      security:
        - mTLS: []

```

#### Step 2: Define schemas

```yaml

components:
  schemas:
    CustomResource:
      type: object
      required:
        - resourceId
        - resourceType
        - capabilities
      properties:
        resourceId:
          type: string
        resourceType:
          type: string
          enum: [gpu, fpga, smartnic]
        capabilities:
          type: object
          additionalProperties:
            type: string
        metadata:
          $ref: '#/components/schemas/ResourceMetadata'

    CustomResourceListResponse:
      type: object
      properties:
        resources:
          type: array
          items:
            $ref: '#/components/schemas/CustomResource'
        total:
          type: integer

```

#### Step 3: Add examples

```yaml

components:
  examples:
    GPUResource:
      value:
        resourceId: "gpu-a100-001"
        resourceType: "gpu"
        capabilities:
          model: "NVIDIA A100"
          memory: "40GB"
          cudaCores: "6912"

```

### 2. Adding Vendor Extensions

OpenAPI supports custom fields prefixed with `x-`:

```yaml

paths:
  /subscriptions:
    post:
      # Standard OpenAPI fields
      summary: Create subscription

      # Vendor extensions
      x-code-samples:
        - lang: curl
          source: |
            curl -X POST https://gateway.example.com/o2ims-infrastructureInventory/v1/subscriptions \
              -H "Content-Type: application/json" \
              -d '{"callback": "https://smo.example.com/notify"}'

        - lang: go
          source: |
            sub := &o2ims.O2Subscription{
                Callback: "https://smo.example.com/notify",
            }
            result, err := client.CreateSubscription(ctx, sub)

      x-rate-limit:
        limit: 100
        window: 60
        scope: tenant

      x-cache:
        enabled: false
        ttl: 0

```

**Custom Documentation Extensions:**

```yaml

components:
  schemas:
    O2Subscription:
      properties:
        subscriptionId:
          type: string
          format: uuid
          x-internal-only: false
          x-immutable: true
          x-audit-logged: true

```

### 3. Schema Composition

**Using `allOf` (Inheritance):**

```yaml

components:
  schemas:
    BaseResource:
      type: object
      required:
        - resourceId
      properties:
        resourceId:
          type: string
        createdAt:
          type: string
          format: date-time

    ComputeResource:
      allOf:
        - $ref: '#/components/schemas/BaseResource'
        - type: object
          required:
            - cpu
            - memory
          properties:
            cpu:
              type: integer
            memory:
              type: integer

```

**Using `oneOf` (Alternatives):**

```yaml

ResourceFilter:
  oneOf:
    - $ref: '#/components/schemas/ResourcePoolFilter'
    - $ref: '#/components/schemas/ResourceTypeFilter'
    - $ref: '#/components/schemas/ResourceIDFilter'
  discriminator:
    propertyName: filterType

```

---

## Code Generation

### 1. Server Stubs

**Generate Gin Server Stubs:**

```bash

# Using openapi-generator
openapi-generator-cli generate \
  -i api/openapi/o2ims.yaml \
  -g go-gin-server \
  -o generated/server \
  --additional-properties=packageName=server,sourceFolder=src

# Generated structure:
# generated/server/
#   ├── go/
#   │   ├── api_subscriptions.go
#   │   ├── api_resource_pools.go
#   │   ├── model_o2_subscription.go
#   │   └── routers.go
#   └── main.go

```

**Integrate Generated Code:**

```go

package main

import (
    "github.com/gin-gonic/gin"
    "github.com/piwi3910/netweave/generated/server/go"
)

// Implement the generated interface
type SubscriptionAPIService struct {
    store SubscriptionStore
}

func (s *SubscriptionAPIService) ListSubscriptions(c *gin.Context) {
    // Implementation
    subscriptions, err := s.store.List(c.Request.Context())
    if err != nil {
        c.JSON(500, sw.ErrorResponse{Message: err.Error()})
        return
    }

    c.JSON(200, sw.SubscriptionListResponse{
        Subscriptions: subscriptions,
        Total:         len(subscriptions),
    })
}

```

### 2. Model Generation

**Generate Go Models Only:**

```bash

openapi-generator-cli generate \
  -i api/openapi/o2ims.yaml \
  -g go \
  -o pkg/models \
  --global-property models \
  --additional-properties=packageName=models

```

**Generate with Validation:**

```bash

# Using oapi-codegen (better Go support)
go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest

# Generate types with validation
oapi-codegen -generate types \
  -package models \
  -o pkg/models/o2ims_types.go \
  api/openapi/o2ims.yaml

# Generate server interface
oapi-codegen -generate gin \
  -package server \
  -o internal/server/generated.go \
  api/openapi/o2ims.yaml

```

### 3. Client Generation

**Go Client with Retries:**

```bash

openapi-generator-cli generate \
  -i api/openapi/o2ims.yaml \
  -g go \
  -o sdk/go/o2ims \
  --additional-properties=packageName=o2ims,withGoMod=false

```

**Python Client with Type Hints:**

```bash

openapi-generator-cli generate \
  -i api/openapi/o2ims.yaml \
  -g python \
  -o sdk/python \
  --additional-properties=packageName=o2ims_client,generateSourceCodeOnly=true

```

---

## Testing and Validation

### 1. Spec Validation

**Automated Validation (Makefile):**

```makefile

.PHONY: validate-openapi
validate-openapi: ## Validate OpenAPI specifications
    @echo "Validating OpenAPI specs..."
    @npx @redocly/cli lint api/openapi/o2ims.yaml
    @npx @redocly/cli lint internal/server/openapi/o2ims.yaml
    @echo "✓ OpenAPI specs are valid"

.PHONY: bundle-openapi
bundle-openapi: ## Bundle OpenAPI specs (resolve $refs)
    @echo "Bundling OpenAPI specs..."
    @npx @redocly/cli bundle api/openapi/o2ims.yaml \
        -o dist/o2ims-bundled.yaml
    @echo "✓ Bundled spec: dist/o2ims-bundled.yaml"

```

**Pre-commit Hook:**

```bash

#!/bin/bash
# .git/hooks/pre-commit

# Validate OpenAPI specs before commit
if git diff --cached --name-only | grep -q "openapi.*\.yaml"; then
    echo "Validating OpenAPI specs..."
    make validate-openapi || exit 1
fi

```

### 2. API Compliance Testing

**Contract Testing (Go):**

```go

package server_test

import (
    "context"
    "net/http/httptest"
    "testing"

    "github.com/getkin/kin-openapi/openapi3"
    "github.com/getkin/kin-openapi/openapi3filter"
    "github.com/stretchr/testify/require"
)

func TestAPICompliance(t *testing.T) {
    // Load OpenAPI spec
    loader := openapi3.NewLoader()
    doc, err := loader.LoadFromFile("../../api/openapi/o2ims.yaml")
    require.NoError(t, err)

    // Validate spec
    err = doc.Validate(context.Background())
    require.NoError(t, err)

    // Create router
    router, err := openapi3filter.NewRouter().WithSwagger(doc)
    require.NoError(t, err)

    // Test each endpoint
    tests := []struct {
        method string
        path   string
        body   string
    }{
        {"GET", "/o2ims-infrastructureInventory/v1/subscriptions", ""},
        {"POST", "/o2ims-infrastructureInventory/v1/subscriptions", `{"callback":"https://example.com"}`},
    }

    for _, tt := range tests {
        t.Run(tt.method+" "+tt.path, func(t *testing.T) {
            req := httptest.NewRequest(tt.method, tt.path, nil)

            // Validate request against spec
            route, pathParams, err := router.FindRoute(req)
            require.NoError(t, err)

            input := &openapi3filter.RequestValidationInput{
                Request:    req,
                PathParams: pathParams,
                Route:      route,
            }

            err = openapi3filter.ValidateRequest(context.Background(), input)
            require.NoError(t, err)
        })
    }
}

```

### 3. Example Validation

**Validate Examples Match Schemas:**

```bash

# Using Spectral with custom rule
cat > .spectral.yaml <<EOF
extends: ["spectral:oas"]
rules:
  oas3-valid-schema-example: error
  oas3-valid-media-example: error
EOF

spectral lint api/openapi/o2ims.yaml

```

---

## Versioning Strategy

### Semantic Versioning

netweave follows semantic versioning for OpenAPI specs:

**Version Format:** `MAJOR.MINOR.PATCH`

- **MAJOR**: Breaking changes (incompatible API changes)

- **MINOR**: New features (backward-compatible additions)

- **PATCH**: Bug fixes (backward-compatible fixes)

### Version Management

**In `info` Section:**

```yaml

info:
  title: O2-IMS API
  version: 2.1.0  # Current version
  x-api-version:
    major: 2
    minor: 1
    patch: 0
  x-changelog:
    - version: 2.1.0
      date: 2026-01-15
      changes:
        - Added batch operations endpoints
        - Enhanced filtering capabilities
    - version: 2.0.0
      date: 2025-12-01
      changes:
        - Breaking: Changed subscription filter format
        - Added multi-tenancy support

```

### Deprecation Strategy

**Marking Deprecated Endpoints:**

```yaml

paths:
  /subscriptions/legacy:
    get:
      deprecated: true
      summary: List subscriptions (DEPRECATED)
      description: |
        **DEPRECATED**: Use `/subscriptions` instead.
        This endpoint will be removed in v3.0.0.
      x-deprecation:
        since: "2.0.0"
        removal: "3.0.0"
        replacement: "/subscriptions"
        migration: |
          Replace:
            GET /subscriptions/legacy
          With:
            GET /subscriptions

```

**Deprecation Headers:**

```yaml

responses:
  '200':
    description: Success (deprecated)
    headers:
      Deprecation:
        schema:
          type: string
        example: "true"
      Sunset:
        schema:
          type: string
          format: date-time
        example: "2026-06-01T00:00:00Z"

```

---

## Documentation Generation

### 1. Static HTML Documentation

**Using Redoc:**

```bash

# Install Redoc CLI
npm install -g @redocly/cli

# Generate standalone HTML
redocly build-docs api/openapi/o2ims.yaml \
  -o docs/api/index.html \
  --title "O2-IMS API Documentation"

# With custom theme
redocly build-docs api/openapi/o2ims.yaml \
  -o docs/api/index.html \
  --theme.openapi.theme=dark \
  --theme.openapi.primaryColor=#1976d2

```

**Using Swagger UI:**

```bash

# Generate Swagger UI distribution
npx swagger-ui-dist-package

# Copy spec to dist
cp api/openapi/o2ims.yaml swagger-ui-dist/

```

### 2. Markdown Documentation

**Using widdershins:**

```bash

# Install widdershins
npm install -g widdershins

# Generate Markdown
widdershins api/openapi/o2ims.yaml \
  -o docs/api/reference.md \
  --language_tabs 'go:Go' 'python:Python' 'shell:cURL' \
  --summary \
  --code

```

**Custom Markdown Template:**

```bash

# Create template
cat > templates/api-docs.hbs <<'EOF'
# {{info.title}}

{{info.description}}

## Endpoints

{{#each paths}}
### {{@key}}

{{#each this}}
**{{@key}}** {{summary}}

{{description}}

{{/each}}
{{/each}}
EOF

# Generate with template
widdershins api/openapi/o2ims.yaml \
  -o docs/api/reference.md \
  --user_templates templates/

```

### 3. Interactive Documentation

**Swagger UI Docker:**

```bash

# Run Swagger UI container
docker run -d -p 8080:8080 \
  -e SWAGGER_JSON=/specs/o2ims.yaml \
  -v $(pwd)/api/openapi:/specs \
  --name netweave-api-docs \
  swaggerapi/swagger-ui

# Access at http://localhost:8080

```

**Redoc Docker:**

```bash

# Run Redoc container
docker run -d -p 8080:80 \
  -e SPEC_URL=https://raw.githubusercontent.com/piwi3910/netweave/main/api/openapi/o2ims.yaml \
  --name netweave-redoc \
  redocly/redoc

```

---

## CI/CD Integration

### 1. GitHub Actions Workflow

**`.github/workflows/openapi.yml`:**

```yaml

name: OpenAPI Validation

on:
  pull_request:
    paths:
      - 'api/openapi/**'
      - 'internal/server/openapi/**'
  push:
    branches: [main]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install OpenAPI tools
        run: |
          npm install -g @redocly/cli
          npm install -g @stoplight/spectral-cli

      - name: Validate public spec
        run: |
          redocly lint api/openapi/o2ims.yaml
          spectral lint api/openapi/o2ims.yaml

      - name: Validate internal spec
        run: |
          redocly lint internal/server/openapi/o2ims.yaml

      - name: Check for breaking changes
        if: github.event_name == 'pull_request'
        run: |
          redocly diff \
            origin/${{ github.base_ref }}:api/openapi/o2ims.yaml \
            HEAD:api/openapi/o2ims.yaml

      - name: Generate documentation
        run: |
          redocly build-docs api/openapi/o2ims.yaml \
            -o dist/api-docs.html

      - name: Upload documentation artifact
        uses: actions/upload-artifact@v4
        with:
          name: api-documentation
          path: dist/api-docs.html

  test-contract:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Run contract tests
        run: |
          go test -v ./internal/server -run TestAPICompliance

```

### 2. Breaking Change Detection

**Using Redocly:**

```bash

# In CI pipeline
redocly diff \
  $BASE_SPEC \
  $HEAD_SPEC \
  --format markdown \
  > breaking-changes.md

# Exit with error if breaking changes found
if grep -q "Breaking changes" breaking-changes.md; then
  echo "❌ Breaking changes detected!"
  cat breaking-changes.md
  exit 1
fi

```

**Using oasdiff:**

```bash

# Install oasdiff
go install github.com/tufin/oasdiff@latest

# Check for breaking changes
oasdiff breaking \
  api/openapi/o2ims-v1.yaml \
  api/openapi/o2ims-v2.yaml

# Generate changelog
oasdiff changelog \
  api/openapi/o2ims-v1.yaml \
  api/openapi/o2ims-v2.yaml \
  -f markdown \
  -o CHANGELOG-API.md

```

### 3. Automated Publishing

**Publish to GitHub Pages:**

```yaml

# .github/workflows/publish-docs.yml
name: Publish API Docs

on:
  push:
    branches: [main]
    paths:
      - 'api/openapi/**'

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Generate HTML docs
        run: |
          npx @redocly/cli build-docs \
            api/openapi/o2ims.yaml \
            -o public/index.html

      - name: Deploy to GitHub Pages
        uses: peaceiris/actions-gh-pages@v3
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./public

```

### 4. Spec Versioning

**Tag Releases:**

```bash

# Tag OpenAPI spec versions
git tag -a api-v2.1.0 -m "API v2.1.0 - Add batch operations"
git push origin api-v2.1.0

# Generate changelog
git log api-v2.0.0..api-v2.1.0 \
  --pretty=format:"- %s" \
  --grep="API:" \
  > CHANGELOG-API.md

```

---

## Best Practices

### 1. Schema Design

**✅ DO:**

- Use clear, descriptive names

- Include examples for all schemas

- Add descriptions for complex fields

- Use `format` for specific types (uuid, uri, date-time)

- Define validation constraints (min, max, pattern)

- Use `$ref` for reusable components

**❌ DON'T:**

- Use generic names (e.g., `Data`, `Info`)

- Leave schemas without examples

- Use `additionalProperties: true` without justification

- Mix snake_case and camelCase in same spec

- Define duplicate schemas

### 2. Endpoint Design

**✅ DO:**

- Use consistent naming conventions

- Include operation IDs for all endpoints

- Add tags for logical grouping

- Provide detailed descriptions

- Document all possible responses

- Include security requirements

**❌ DON'T:**

- Use verbs in paths (use HTTP methods)

- Forget error responses

- Leave operation IDs auto-generated

- Mix REST and RPC styles

### 3. Documentation

**✅ DO:**

- Write descriptions in Markdown

- Include code examples

- Document rate limits

- Explain authentication

- Provide migration guides for breaking changes

**❌ DON'T:**

- Use technical jargon without explanation

- Assume users know the domain

- Forget to update examples

---

## Tools Reference

### Essential Tools

| Tool | Purpose | Installation |
|------|---------|--------------|
| **Redocly CLI** | Lint, bundle, build docs | `npm install -g @redocly/cli` |
| **Spectral** | Advanced linting | `npm install -g @stoplight/spectral-cli` |
| **OpenAPI Generator** | Code generation | `npm install -g @openapitools/openapi-generator-cli` |
| **oapi-codegen** | Go code generation | `go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest` |
| **Swagger CLI** | Validation, bundling | `npm install -g @apidevtools/swagger-cli` |
| **oasdiff** | Breaking change detection | `go install github.com/tufin/oasdiff@latest` |

### Useful Links

- **OpenAPI Specification**: https://spec.openapis.org/oas/v3.0.3

- **OpenAPI Generator**: https://openapi-generator.tech/

- **Redocly Docs**: https://redocly.com/docs/

- **Spectral Rules**: https://meta.stoplight.io/docs/spectral/

- **Swagger Tools**: https://swagger.io/tools/

---

## Troubleshooting

### Common Issues

#### 1. Validation Error: "Property not allowed"

```

Error: Property `example` is not expected to be here

```

**Solution**: Move `example` to correct location (schema level, not property level)

#### 2. Reference Resolution Failed

```

Error: Can't resolve $ref: #/components/schemas/Missing

```

**Solution**: Ensure referenced component exists and path is correct

#### 3. Circular Reference

```

Error: Circular $ref detected

```

**Solution**: Break circular references using `discriminator` or flatten schema

#### 4. Invalid Format

```

Error: "invalid-uri" is not a valid "uri"

```

**Solution**: Validate example data matches declared format

---

## See Also

- **[API Documentation](api/README.md)** - API usage guides

- **[Development Guide](development/README.md)** - Development setup

- **[Contributing](../CONTRIBUTING.md)** - Contribution guidelines

- **[O-RAN Specification](https://specifications.o-ran.org/)** - O-RAN compliance details
