# Terraform Provider Idempotency Testing Guide

## Overview

This guide explains how to identify, fix, and test idempotency bugs in Terraform provider resources. The same patterns apply across all service resources in this repository.

## Table of Contents

1. [The Idempotency Problem](#the-idempotency-problem)
2. [Identifying the Bug](#identifying-the-bug)
3. [Fixing the Bug](#fixing-the-bug)
4. [Finding REST API Endpoints](#finding-rest-api-endpoints)
5. [Test Infrastructure](#test-infrastructure)
6. [Writing Tests](#writing-tests)
7. [Required Test Cases](#required-test-cases)
8. [Assertions](#assertions)
9. [Complete Example](#complete-example)

---

## The Idempotency Problem

### What is the bug?

When a user runs `terraform apply` and the context is canceled (timeout, Ctrl+C, network issue) during the wait phase:

1. ✅ Resource **is created** in the cloud API
2. ✅ Provider gets the resource ID from API response
3. ❌ Wait handler times out or fails
4. ❌ State is **never saved** to Terraform
5. ❌ Resource is **orphaned** - exists in cloud but Terraform doesn't know about it

### What happens next?

When user runs `terraform apply` again:

- **If API enforces unique names**: Second create fails with "name already exists" error
- **If API allows duplicate names**: Creates a second resource, wasting money
- **User experience**: Confused, frustrated, must manually import or delete resource

### The root cause

```go
// BUGGY CODE - DON'T DO THIS
createResp, err := r.client.CreateResource(ctx, projectId).Payload(*payload).Execute()
if err != nil {
    return
}
resourceId := *createResp.ResourceId

// Wait for resource to be ready
waitResp, err := wait.CreateResourceWaitHandler(ctx, r.client, projectId, resourceId).WaitWithContext(ctx)
if err != nil {
    return  // ❌ ERROR: State never saved!
}

// Only saves state if wait succeeds
resp.State.Set(ctx, model)
```

**Problem**: State is only saved at the end, after the wait succeeds. If wait fails, the resource ID is lost.

---

## Identifying the Bug

### Where to look

All resources in this repository follow the same pattern:

```
stackit/internal/services/<service>/<resource>/resource.go
```

Examples:
- `stackit/internal/services/mariadb/instance/resource.go`
- `stackit/internal/services/redis/instance/resource.go`
- `stackit/internal/services/postgres/instance/resource.go`

### How to identify

Look for these patterns in the `Create`, `Update`, and `Delete` functions:

#### Create Function (most critical)

```go
func (r *resourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
    // ... payload preparation ...

    // 1. API call that creates the resource
    createResp, err := r.client.CreateResource(ctx, projectId).Payload(*payload).Execute()
    if err != nil {
        return  // This is fine - nothing was created
    }
    resourceId := *createResp.ResourceId

    // 2. Wait for resource to be ready
    waitResp, err := wait.CreateResourceWaitHandler(ctx, r.client, projectId, resourceId).WaitWithContext(ctx)
    if err != nil {
        return  // ❌ BUG: Resource created but state not saved!
    }

    // 3. Save state (only reached if wait succeeds)
    resp.State.Set(ctx, model)  // ❌ BUG: Too late!
}
```

**Look for**: `resp.State.Set()` called **after** the wait handler. This is the bug.

#### Update Function

```go
func (r *resourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
    // ... payload preparation ...

    // 1. API call that triggers update
    err = r.client.UpdateResource(ctx, projectId, resourceId).Payload(*payload).Execute()
    if err != nil {
        return
    }

    // 2. Wait for update to complete
    waitResp, err := wait.UpdateResourceWaitHandler(ctx, r.client, projectId, resourceId).WaitWithContext(ctx)
    if err != nil {
        return  // ⚠️ Issue: Update triggered but state shows old values
    }

    // 3. Save state with new values
    resp.State.Set(ctx, model)
}
```

**Impact**: Less critical than Create, but causes drift. Cloud has new config, Terraform state has old config.

#### Delete Function

```go
func (r *resourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
    // 1. API call that triggers deletion
    err := r.client.DeleteResource(ctx, projectId, resourceId).Execute()
    if err != nil {
        return  // ❌ BUG: Fails if resource already deleted (404/410)
    }

    // 2. Wait for deletion to complete
    _, err = wait.DeleteResourceWaitHandler(ctx, r.client, projectId, resourceId).WaitWithContext(ctx)
    if err != nil {
        return  // ⚠️ Issue: Deletion triggered but confirmation timeout
    }

    // State is automatically removed on success
}
```

**Impact**:
1. **NOT idempotent**: Running `terraform destroy` twice fails with "resource not found" error
2. Resource may still be deleting if wait times out
3. User should be informed to check portal

---

## Fixing the Bug

### Create Operation Fix

**Before (buggy)**:
```go
createResp, err := r.client.CreateResource(ctx, projectId).Payload(*payload).Execute()
if err != nil {
    return
}
resourceId := *createResp.ResourceId

waitResp, err := wait.CreateResourceWaitHandler(ctx, r.client, projectId, resourceId).WaitWithContext(ctx)
if err != nil {
    return  // ❌ Bug here
}

resp.State.Set(ctx, model)
```

**After (fixed)**:
```go
createResp, err := r.client.CreateResource(ctx, projectId).Payload(*payload).Execute()
if err != nil {
    return
}
resourceId := *createResp.ResourceId

// ✅ FIX: Save minimal state immediately
model.ResourceId = types.StringValue(resourceId)
model.Id = types.StringValue(fmt.Sprintf("%s,%s", projectId, resourceId))
diags = resp.State.Set(ctx, model)
resp.Diagnostics.Append(diags...)
if resp.Diagnostics.HasError() {
    return
}

// Now wait for resource to be ready
waitResp, err := wait.CreateResourceWaitHandler(ctx, r.client, projectId, resourceId).WaitWithContext(ctx)
if err != nil {
    core.LogAndAddError(ctx, &resp.Diagnostics, "Error creating resource",
        fmt.Sprintf("Resource creation waiting: %v. The resource was created but is not yet ready. You can check its status in the STACKIT Portal or run 'terraform refresh' to update the state once it's ready.", err))
    return  // ✅ Now safe - state is already saved
}

// Map full response and update state
err = mapFields(waitResp, &model)
if err != nil {
    return
}
resp.State.Set(ctx, model)
```

**Key changes**:
1. Save `ResourceId` and `Id` immediately after API call succeeds
2. Add error handling for state save
3. Improve error message to guide user
4. Still update state with full response after wait succeeds

### Update Operation Fix

**Improved error message**:
```go
waitResp, err := wait.UpdateResourceWaitHandler(ctx, r.client, projectId, resourceId).WaitWithContext(ctx)
if err != nil {
    core.LogAndAddError(ctx, &resp.Diagnostics, "Error updating resource",
        fmt.Sprintf("Resource update waiting: %v. The update was triggered but may not be complete. Run 'terraform refresh' to check the current state.", err))
    return
}
```

### Delete Operation Fix

**Handle 404/410 as success for idempotency**:

```go
// Delete existing resource
err := r.client.DeleteResource(ctx, projectId, resourceId).Execute()
if err != nil {
    // If resource is already gone (404 or 410), treat as success for idempotency
    oapiErr, ok := err.(*oapierror.GenericOpenAPIError) //nolint:errorlint //complaining that error.As should be used to catch wrapped errors, but this error should not be wrapped
    if ok && (oapiErr.StatusCode == http.StatusNotFound || oapiErr.StatusCode == http.StatusGone) {
        tflog.Info(ctx, "Resource already deleted")
        return
    }
    core.LogAndAddError(ctx, &resp.Diagnostics, "Error deleting resource", fmt.Sprintf("Calling API: %v", err))
    return
}

// Wait for deletion to complete
_, err = wait.DeleteResourceWaitHandler(ctx, r.client, projectId, resourceId).WaitWithContext(ctx)
if err != nil {
    core.LogAndAddError(ctx, &resp.Diagnostics, "Error deleting resource",
        fmt.Sprintf("Resource deletion waiting: %v. The resource deletion was triggered but confirmation timed out. The resource may still be deleting. Check the STACKIT Portal or retry the operation.", err))
    return
}
```

**Why this matters**: Without this fix, running `terraform destroy` multiple times fails if the resource is already deleted. With the fix, `terraform destroy` is truly idempotent - it succeeds whether the resource exists or not.

---

## Finding REST API Endpoints

### Using go doc

To find the SDK methods and understand API behavior:

```bash
# List all methods for a service
go doc github.com/stackitcloud/stackit-sdk-go/services/<service>

# Example for MariaDB
go doc github.com/stackitcloud/stackit-sdk-go/services/mariadb

# Get details of a specific struct
go doc github.com/stackitcloud/stackit-sdk-go/services/mariadb.Instance

# Check wait handlers
go doc github.com/stackitcloud/stackit-sdk-go/services/mariadb/wait
```

### Understanding response structures

```bash
# See what fields a response has
go doc github.com/stackitcloud/stackit-sdk-go/services/mariadb.CreateInstanceResponse

# Output shows:
type CreateInstanceResponse struct {
    InstanceId *string `json:"instanceId,omitempty"`
}
```

### Finding status values

Look for constants in the SDK:

```bash
go doc github.com/stackitcloud/stackit-sdk-go/services/mariadb/wait

# Output shows:
const InstanceStatusActive = "active"
```

This tells you what JSON values to use in mock responses.

---

## Test Infrastructure

### File Organization

Create separate test files for each operation:

```
stackit/internal/services/<service>/<resource>/
├── resource.go                    # Implementation
├── resource_test.go              # Existing helper tests
├── resource_test_utils.go        # NEW: Shared test utilities
├── resource_create_test.go       # NEW: Create operation tests
├── resource_update_test.go       # NEW: Update operation tests
├── resource_delete_test.go       # NEW: Delete operation tests
└── resource_read_test.go         # NEW: Read operation tests
```

### Test Utilities Template

Create `resource_test_utils.go`:

```go
package <service>

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gorilla/mux"
    "github.com/hashicorp/terraform-plugin-framework/resource"
    "github.com/hashicorp/terraform-plugin-framework/tfsdk"
    "github.com/hashicorp/terraform-plugin-framework/types"
    "github.com/hashicorp/terraform-plugin-go/tftypes"
    "github.com/stackitcloud/stackit-sdk-go/core/config"
    "github.com/stackitcloud/stackit-sdk-go/core/utils"
    "github.com/stackitcloud/stackit-sdk-go/services/<service>"
)

// TestContext holds common test setup
type TestContext struct {
    T          *testing.T
    Server     *httptest.Server
    Client     *<service>.APIClient
    Resource   *<resource>Resource
    Router     *mux.Router
    Ctx        context.Context
    CancelFunc context.CancelFunc
}

// NewTestContext creates a new test context with mock server
func NewTestContext(t *testing.T) *TestContext {
    router := mux.NewRouter()
    server := httptest.NewServer(router)

    client, err := <service>.NewAPIClient(
        config.WithEndpoint(server.URL),
        config.WithoutAuthentication(),
    )
    if err != nil {
        t.Fatalf("Failed to create client: %v", err)
    }

    resource := &<resource>Resource{
        client: client,
    }

    return &TestContext{
        T:        t,
        Server:   server,
        Client:   client,
        Resource: resource,
        Router:   router,
        Ctx:      context.Background(),
    }
}

// Close cleans up the test context
func (tc *TestContext) Close() {
    if tc.CancelFunc != nil {
        tc.CancelFunc()
    }
    tc.Server.Close()
}

// GetSchema returns the resource schema
func (tc *TestContext) GetSchema() resource.SchemaResponse {
    schemaResp := resource.SchemaResponse{}
    tc.Resource.Schema(tc.Ctx, resource.SchemaRequest{}, &schemaResp)
    return schemaResp
}

// SetupCreateResourceHandler adds mock handler for Create API
func (tc *TestContext) SetupCreateResourceHandler(resourceId string, callCounter *int) {
    tc.Router.HandleFunc("/v1/projects/{projectId}/<resources>", func(w http.ResponseWriter, r *http.Request) {
        if callCounter != nil {
            *callCounter++
        }
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusAccepted)

        resp := <service>.Create<Resource>Response{
            <Resource>Id: utils.Ptr(resourceId),
        }
        respBytes, _ := json.Marshal(resp)
        w.Write(respBytes)
    }).Methods("POST")
}

// SetupGetResourceHandler adds mock handler for Get API
func (tc *TestContext) SetupGetResourceHandler(resourceId, name, status string) {
    tc.Router.HandleFunc("/v1/projects/{projectId}/<resources>/{resourceId}", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")

        // Return JSON directly to avoid SDK type issues
        jsonResp := fmt.Sprintf(`{
            "resourceId": "%s",
            "name": "%s",
            "status": "%s"
        }`, resourceId, name, status)
        w.Write([]byte(jsonResp))
    }).Methods("GET")
}

// SetupUpdateResourceHandler adds mock handler for Update API
func (tc *TestContext) SetupUpdateResourceHandler(callCounter *int) {
    tc.Router.HandleFunc("/v1/projects/{projectId}/<resources>/{resourceId}", func(w http.ResponseWriter, r *http.Request) {
        if callCounter != nil {
            *callCounter++
        }
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusAccepted)
        w.Write([]byte("{}"))
    }).Methods("PATCH")
}

// SetupDeleteResourceHandler adds mock handler for Delete API
func (tc *TestContext) SetupDeleteResourceHandler(callCounter *int) {
    tc.Router.HandleFunc("/v1/projects/{projectId}/<resources>/{resourceId}", func(w http.ResponseWriter, r *http.Request) {
        if callCounter != nil {
            *callCounter++
        }
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusAccepted)
        w.Write([]byte("{}"))
    }).Methods("DELETE")
}

// AssertStateFieldEquals checks a single field in the model
func AssertStateFieldEquals(t *testing.T, fieldName string, got, want types.String) {
    t.Helper()
    if !got.Equal(want) {
        t.Errorf("%s mismatch: got=%s, want=%s", fieldName, got.ValueString(), want.ValueString())
    }
}

// Request/Response builders
func CreateRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.CreateRequest {
    req := resource.CreateRequest{}
    req.Plan = tfsdk.Plan{
        Schema: schema.Schema,
        Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
    }
    req.Plan.Set(ctx, model)
    return req
}

func CreateResponse(schema resource.SchemaResponse) *resource.CreateResponse {
    resp := &resource.CreateResponse{}
    resp.State = tfsdk.State{
        Schema: schema.Schema,
        Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
    }
    return resp
}

// UpdateResponse creates a test Update response
// Optionally initialize with current state to simulate Terraform framework behavior
func UpdateResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.UpdateResponse {
    resp := &resource.UpdateResponse{}
    resp.State = tfsdk.State{
        Schema: schema.Schema,
        Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
    }
    // Initialize with current state to simulate framework behavior
    // When Update errors without calling State.Set(), this state is preserved
    if currentState != nil {
        resp.State.Set(ctx, *currentState)
    }
    return resp
}

// DeleteResponse creates a test Delete response
// Optionally initialize with current state to simulate Terraform framework behavior
func DeleteResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.DeleteResponse {
    resp := &resource.DeleteResponse{}
    resp.State = tfsdk.State{
        Schema: schema.Schema,
        Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
    }
    // Initialize with current state to simulate framework behavior
    // When Delete errors without calling State.RemoveResource(), this state is preserved
    if currentState != nil {
        resp.State.Set(ctx, *currentState)
    }
    return resp
}
```

### Key Utilities to Implement

1. **Mock HTTP Handlers**: Setup functions for each API endpoint
2. **Request Builders**: Create properly structured Terraform requests
3. **Response Builders**: Create Terraform responses with state
4. **Assertion Helpers**: Compare state fields
5. **Test Data Builders**: Create models with common test values

---

## Writing Tests

### Test File Template

Create `resource_create_test.go`:

```go
package <service>

import (
    "context"
    "fmt"
    "net/http"
    "testing"
    "time"

    "github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCreate_ContextCanceledDuringWait(t *testing.T) {
    tc := NewTestContext(t)
    defer tc.Close()

    // Test data
    projectId := "test-project-123"
    resourceId := "resource-abc-123"
    resourceName := "my-test-resource"

    // Setup mock handlers
    createCalled := 0
    tc.SetupCreateResourceHandler(resourceId, &createCalled)

    // Setup GetResource to simulate slow response (triggers timeout)
    tc.Router.HandleFunc("/v1/projects/{projectId}/<resources>/{resourceId}", func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(150 * time.Millisecond)
        w.Header().Set("Content-Type", "application/json")
        jsonResp := fmt.Sprintf(`{"resourceId": "%s", "name": "%s", "status": "creating"}`, resourceId, resourceName)
        w.Write([]byte(jsonResp))
    }).Methods("GET")

    // Create context with short timeout
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    tc.Ctx = ctx

    // Prepare request
    schema := tc.GetSchema()
    model := Model{
        ProjectId: types.StringValue(projectId),
        Name:      types.StringValue(resourceName),
    }
    req := CreateRequest(tc.Ctx, schema, model)
    resp := CreateResponse(schema)

    // Execute Create
    tc.Resource.Create(tc.Ctx, req, resp)

    // Assertions
    if createCalled == 0 {
        t.Fatal("CreateResource API should have been called")
    }

    if !resp.Diagnostics.HasError() {
        t.Fatal("Expected error due to context timeout")
    }

    // CRITICAL: Verify instance_id was saved
    var stateAfterCreate Model
    diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
    if diags.HasError() {
        t.Logf("Warnings getting state: %v", diags)
    }

    // Verify idempotency fix
    if stateAfterCreate.ResourceId.IsNull() || stateAfterCreate.ResourceId.ValueString() == "" {
        t.Fatal("BUG: ResourceId should be saved to state immediately after API succeeds")
    }

    AssertStateFieldEquals(t, "ResourceId", stateAfterCreate.ResourceId, types.StringValue(resourceId))
    AssertStateFieldEquals(t, "Id", stateAfterCreate.Id, types.StringValue(fmt.Sprintf("%s,%s", projectId, resourceId)))

    t.Log("GOOD: ResourceId saved even though wait failed - idempotency guaranteed")
}

func TestCreate_Success(t *testing.T) {
    tc := NewTestContext(t)
    defer tc.Close()

    // Test data
    projectId := "test-project-123"
    resourceId := "resource-abc-123"
    resourceName := "my-test-resource"

    // Setup mock handlers
    createCalled := 0
    tc.SetupCreateResourceHandler(resourceId, &createCalled)
    tc.SetupGetResourceHandler(resourceId, resourceName, "active")

    // Prepare request
    schema := tc.GetSchema()
    model := Model{
        ProjectId: types.StringValue(projectId),
        Name:      types.StringValue(resourceName),
    }
    req := CreateRequest(tc.Ctx, schema, model)
    resp := CreateResponse(schema)

    // Execute Create
    tc.Resource.Create(tc.Ctx, req, resp)

    // Assertions
    if createCalled == 0 {
        t.Fatal("CreateResource API should have been called")
    }

    if resp.Diagnostics.HasError() {
        t.Fatalf("Create should succeed, but got errors: %v", resp.Diagnostics.Errors())
    }

    // Extract final state
    var finalState Model
    diags := resp.State.Get(tc.Ctx, &finalState)
    if diags.HasError() {
        t.Fatalf("Failed to get state: %v", diags.Errors())
    }

    // Verify all fields
    AssertStateFieldEquals(t, "ResourceId", finalState.ResourceId, types.StringValue(resourceId))
    AssertStateFieldEquals(t, "Name", finalState.Name, types.StringValue(resourceName))

    t.Log("SUCCESS: All state fields correctly populated")
}
```

---

## Required Test Cases

### Create Operation (3 tests minimum)

#### 1. TestCreate_ContextCanceledDuringWait (CRITICAL - idempotency test)
**Purpose**: Verify resource ID is saved even when wait times out

**Setup**:
- Mock Create API returns resource ID immediately
- Mock Get API sleeps longer than context timeout
- Use short context timeout (100ms)

**Assertions**:
- ✅ Create API was called
- ✅ Get API was called (wait attempted)
- ✅ Error returned (timeout)
- ✅ ResourceId is NOT null
- ✅ ResourceId matches what API returned
- ✅ Composite Id is set correctly
- ✅ All required fields are populated

#### 2. TestCreate_Success
**Purpose**: Verify happy path works correctly

**Setup**:
- Mock Create API returns resource ID
- Mock Get API returns complete resource with "active" status

**Assertions**:
- ✅ Create API was called
- ✅ No errors
- ✅ All state fields match Get API response
- ✅ Computed fields are populated

#### 3. TestCreate_APICallFails
**Purpose**: Verify error handling when API fails

**Setup**:
- Mock Create API returns 500 error

**Assertions**:
- ✅ Error returned
- ✅ ResourceId is null (nothing created)

### Update Operation (3 tests minimum)

#### 1. TestUpdate_Success
**Purpose**: Verify successful update

**Setup**:
- Mock Update API succeeds
- Mock Get API returns updated resource

**Assertions**:
- ✅ Update API called
- ✅ No errors
- ✅ State updated with new values

#### 2. TestUpdate_ContextCanceledDuringWait
**Purpose**: Verify behavior when update wait times out and state is not incorrectly updated

**Setup**:
- Mock Update API succeeds
- Mock Get API sleeps longer than timeout
- Use short context timeout
- **IMPORTANT**: Initialize UpdateResponse with currentState to simulate framework behavior

**Assertions**:
- ✅ Update API called
- ✅ Error returned
- ✅ Helpful error message
- ✅ State does NOT have new planned values
- ✅ State has old values for PlanId and PlanName
- ✅ All other fields (Id, ProjectId, InstanceId, Name, Version) match currentState

#### 3. TestUpdate_APICallFails
**Purpose**: Verify error handling

**Setup**:
- Mock Update API returns error

**Assertions**:
- ✅ Error returned

### Delete Operation (5 tests minimum)

#### 1. TestDelete_Success
**Purpose**: Verify successful deletion

**Setup**:
- Mock Delete API succeeds
- Mock Get API returns 410 Gone (resource deleted)

**Assertions**:
- ✅ Delete API called
- ✅ No errors

#### 2. TestDelete_ContextCanceledDuringWait
**Purpose**: Verify behavior when delete wait times out and state is not incorrectly removed

**Setup**:
- Mock Delete API succeeds
- Mock Get API sleeps longer than timeout
- Use short context timeout
- **IMPORTANT**: Initialize DeleteResponse with currentState to simulate framework behavior

**Assertions**:
- ✅ Delete API called
- ✅ Error returned
- ✅ Helpful error message
- ✅ State is NOT removed (resource still tracked)
- ✅ All fields (Id, ProjectId, InstanceId, Name, Version, PlanId, PlanName) match original state

#### 3. TestDelete_APICallFails
**Purpose**: Verify error handling

**Setup**:
- Mock Delete API returns 500 error

**Assertions**:
- ✅ Error returned

#### 4. TestDelete_ResourceAlreadyDeleted (IDEMPOTENCY TEST)
**Purpose**: Verify Delete is idempotent when resource doesn't exist (404)

**Setup**:
- Mock Delete API returns 404 Not Found

**Assertions**:
- ✅ Delete API called
- ✅ No error (succeeds for idempotency)
- ✅ Operation completes successfully

#### 5. TestDelete_ResourceGone (IDEMPOTENCY TEST)
**Purpose**: Verify Delete is idempotent when resource is gone (410)

**Setup**:
- Mock Delete API returns 410 Gone

**Assertions**:
- ✅ Delete API called
- ✅ No error (succeeds for idempotency)
- ✅ Operation completes successfully

### Read Operation (5 tests minimum)

#### 1. TestRead_Success
**Purpose**: Verify state refresh

**Setup**:
- Mock Get API returns complete resource

**Assertions**:
- ✅ No errors
- ✅ All state fields match API response

#### 2. TestRead_ResourceNotFound (404)
**Purpose**: Verify resource removal from state

**Setup**:
- Mock Get API returns 404

**Assertions**:
- ✅ No error
- ✅ State removed

#### 3. TestRead_ResourceGone (410)
**Purpose**: Verify resource removal from state

**Setup**:
- Mock Get API returns 410

**Assertions**:
- ✅ No error
- ✅ State removed

#### 4. TestRead_APICallFails
**Purpose**: Verify error handling

**Setup**:
- Mock Get API returns 500

**Assertions**:
- ✅ Error returned

#### 5. TestRead_DetectDrift
**Purpose**: Verify drift detection

**Setup**:
- State has old values
- Mock Get API returns updated values

**Assertions**:
- ✅ No errors
- ✅ State updated with new values

---

## Assertions

### Essential Assertions

Every test should verify:

```go
// 1. API was called (if expected)
if callCounter == 0 {
    t.Fatal("API should have been called")
}

// 2. Error status
if resp.Diagnostics.HasError() {
    t.Fatalf("Should succeed, got errors: %v", resp.Diagnostics.Errors())
}

// 3. Extract state
var state Model
diags := resp.State.Get(ctx, &state)
if diags.HasError() {
    t.Fatalf("Failed to get state: %v", diags.Errors())
}

// 4. Field-by-field verification
AssertStateFieldEquals(t, "ResourceId", state.ResourceId, types.StringValue(expectedId))
AssertStateFieldEquals(t, "ProjectId", state.ProjectId, types.StringValue(expectedProjectId))
AssertStateFieldEquals(t, "Name", state.Name, types.StringValue(expectedName))

// 5. Composite ID format
expectedId := fmt.Sprintf("%s,%s", projectId, resourceId)
AssertStateFieldEquals(t, "Id", state.Id, types.StringValue(expectedId))

// 6. Computed fields are set (when applicable)
if state.SomeComputedField.IsNull() {
    t.Error("Computed field should be set from API")
}
```

### Critical Assertions for Idempotency Tests

```go
// After Create with timeout
var stateAfterCreate Model
resp.State.Get(ctx, &stateAfterCreate)

// THE MOST IMPORTANT ASSERTION
if stateAfterCreate.ResourceId.IsNull() {
    t.Fatal("BUG: ResourceId must be saved even when wait fails!")
}

// Verify all minimal required fields are set
AssertStateFieldEquals(t, "ResourceId", stateAfterCreate.ResourceId, types.StringValue(resourceId))
AssertStateFieldEquals(t, "Id", stateAfterCreate.Id, types.StringValue(compositeId))
AssertStateFieldEquals(t, "ProjectId", stateAfterCreate.ProjectId, types.StringValue(projectId))
```

### Assertions for Update Timeout Tests

**IMPORTANT**: Initialize UpdateResponse with currentState to simulate framework behavior:

```go
// Initialize response with current state (simulates framework preserving state on error)
resp := UpdateResponse(tc.Ctx, schema, &currentState)

// Execute Update
tc.Resource.Update(tc.Ctx, req, resp)

// After timeout, verify state was NOT updated with new values
var stateAfterUpdate Model
diags := resp.State.Get(tc.Ctx, &stateAfterUpdate)
if diags.HasError() {
    t.Fatalf("Failed to get state after update: %v", diags.Errors())
}

// Verify state doesn't have NEW planned values
if !stateAfterUpdate.PlanId.IsNull() {
    actualPlanId := stateAfterUpdate.PlanId.ValueString()
    if actualPlanId == newPlanId {
        t.Fatal("BUG: State has NEW PlanId even though update wait failed!")
    }
    AssertStateFieldEquals(t, "PlanId", stateAfterUpdate.PlanId, types.StringValue(oldPlanId))
}

if !stateAfterUpdate.PlanName.IsNull() {
    actualPlanName := stateAfterUpdate.PlanName.ValueString()
    if actualPlanName == newPlanName {
        t.Fatal("BUG: State has NEW PlanName even though update wait failed!")
    }
    AssertStateFieldEquals(t, "PlanName", stateAfterUpdate.PlanName, types.StringValue(oldPlanName))
}

// Verify ALL other fields remain unchanged
AssertStateFieldEquals(t, "Id", stateAfterUpdate.Id, currentState.Id)
AssertStateFieldEquals(t, "ProjectId", stateAfterUpdate.ProjectId, currentState.ProjectId)
AssertStateFieldEquals(t, "InstanceId", stateAfterUpdate.InstanceId, currentState.InstanceId)
AssertStateFieldEquals(t, "Name", stateAfterUpdate.Name, currentState.Name)
AssertStateFieldEquals(t, "Version", stateAfterUpdate.Version, currentState.Version)
```

### Assertions for Delete Timeout Tests

**IMPORTANT**: Initialize DeleteResponse with currentState to simulate framework behavior:

```go
// Initialize response with current state (simulates framework preserving state on error)
resp := DeleteResponse(tc.Ctx, schema, &state)

// Execute Delete
tc.Resource.Delete(tc.Ctx, req, resp)

// After timeout, verify state was NOT removed
var stateAfterDelete Model
diags := resp.State.Get(tc.Ctx, &stateAfterDelete)
if diags.HasError() {
    t.Fatalf("Failed to get state after delete: %v", diags.Errors())
}

// Verify ALL fields match original state (resource still tracked)
AssertStateFieldEquals(t, "InstanceId", stateAfterDelete.InstanceId, state.InstanceId)
AssertStateFieldEquals(t, "Id", stateAfterDelete.Id, state.Id)
AssertStateFieldEquals(t, "ProjectId", stateAfterDelete.ProjectId, state.ProjectId)
AssertStateFieldEquals(t, "Name", stateAfterDelete.Name, state.Name)
AssertStateFieldEquals(t, "Version", stateAfterDelete.Version, state.Version)
AssertStateFieldEquals(t, "PlanId", stateAfterDelete.PlanId, state.PlanId)
AssertStateFieldEquals(t, "PlanName", stateAfterDelete.PlanName, state.PlanName)
```

---

## Complete Example

See the MariaDB instance implementation for a complete example:

```
stackit/internal/services/mariadb/instance/
├── resource.go                    # Fixed implementation
├── resource_test_utils.go        # ~300 lines of utilities
├── resource_create_test.go       # 3 comprehensive tests
├── resource_update_test.go       # 3 comprehensive tests
├── resource_delete_test.go       # 5 comprehensive tests (includes idempotency tests)
├── resource_read_test.go         # 5 comprehensive tests
└── TESTING.md                     # Service-specific docs
```

**Run MariaDB tests to see patterns**:
```bash
go test -v ./stackit/internal/services/mariadb/instance
```

---

## Checklist for Fixing a Resource

- [ ] 1. Identify the bug in `resource.go` Create function
- [ ] 2. Fix Create: Save resource ID immediately after API call
- [ ] 3. Improve Update error message
- [ ] 4. Improve Delete error message
- [ ] 5. Use `go doc` to find SDK methods and response structures
- [ ] 6. Create `resource_test_utils.go` with:
  - [ ] TestContext struct
  - [ ] NewTestContext() function
  - [ ] SetupXxxHandler() functions for each API
  - [ ] Request/Response builders
  - [ ] Assertion helpers
- [ ] 7. Create `resource_create_test.go` with 3 tests
- [ ] 8. Create `resource_update_test.go` with 3 tests
- [ ] 9. Create `resource_delete_test.go` with 5 tests (including idempotency tests)
- [ ] 10. Create `resource_read_test.go` with 5 tests
- [ ] 11. Run tests: `go test -v ./stackit/internal/services/<service>/<resource>`
- [ ] 12. Verify all tests pass
- [ ] 13. Document service-specific details in TESTING.md

---

## Benefits

After following this guide:

- ✅ No orphaned resources when context is canceled
- ✅ Delete is truly idempotent (succeeds even if resource already deleted)
- ✅ Clear error messages guide users to recovery
- ✅ Comprehensive test coverage (16+ CRUD tests)
- ✅ Fast, reliable unit tests (<1 second)
- ✅ Reusable test utilities reduce duplication
- ✅ Organized by operation for easy maintenance
- ✅ Future-proof: Same patterns work for all resources

---

## Support

For questions or issues:
1. Review the MariaDB instance implementation (complete reference)
2. Check SDK documentation: `go doc github.com/stackitcloud/stackit-sdk-go/services/<service>`
3. Run existing tests to see patterns: `go test -v ./stackit/internal/services/mariadb/instance`
