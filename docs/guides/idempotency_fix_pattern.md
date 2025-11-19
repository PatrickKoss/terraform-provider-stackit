---
page_title: "Idempotency Fix Pattern for Resources"
---

# Idempotency Fix Pattern for Resources

This guide documents the standard patterns for implementing proper idempotency, error handling, and Crossplane compatibility across all resources in the STACKIT Terraform provider.

## Overview

The idempotency fix pattern ensures that:

1. **Resources can be safely retried** - If a resource creation is interrupted, subsequent applies won't fail
2. **Minimal state is saved immediately** - Critical IDs are persisted as soon as the API call succeeds
3. **Crossplane compatibility** - Resources work correctly in async mode without waiting for operations to complete
4. **Proper error handling** - Network interruptions and edge cases are handled gracefully
5. **Concurrent operations are safe** - Resources handle 404/410 errors appropriately for already-deleted resources

## Reference Implementation

The `stackit_dns_zone` resource (`stackit/internal/services/dns/zone/resource.go`) serves as the canonical implementation of this pattern.

---

## Create Method

### Pattern Overview

The Create method must save a minimal state immediately after the API call succeeds, before waiting for the operation to complete. This ensures idempotency if the operation is interrupted.

### Implementation Steps

#### Step 1: Create Minimal Model from Plan

After retrieving the main model from the plan, create a second minimal model:

```go
func (r *exampleResource) Create(
    ctx context.Context,
    req resource.CreateRequest,
    resp *resource.CreateResponse,
) {
    // Retrieve values from plan
    var model Model
    resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
    if resp.Diagnostics.HasError() {
        return
    }

    // Get a fresh copy from plan for minimal state
    var minimalModel Model
    resp.Diagnostics.Append(req.Plan.Get(ctx, &minimalModel)...)
    if resp.Diagnostics.HasError() {
        return
    }

    projectId := model.ProjectId.ValueString()
    ctx = tflog.SetField(ctx, "project_id", projectId)

    // ... payload construction
}
```

**Reference:** `stackit/internal/services/dns/zone/resource.go:310-318`

#### Step 2: Call Create API

Make the API call to create the resource:

```go
// Create new resource
createResp, err := r.client.CreateResource(ctx, projectId).CreatePayload(*payload).Execute()
if err != nil {
    core.LogAndAddError(
        ctx,
        &resp.Diagnostics,
        "Error creating resource",
        fmt.Sprintf("Calling API: %v", err),
    )
    return
}
```

**Reference:** `stackit/internal/services/dns/zone/resource.go:332-341`

#### Step 3: Save Minimal State Immediately

As soon as the API call succeeds, populate the minimal model with only the IDs and save it to state:

```go
// Save minimal state immediately after API call succeeds to ensure idempotency
resourceId := *createResp.Resource.Id
minimalModel.ResourceId = types.StringValue(resourceId)
minimalModel.Id = utils.BuildInternalTerraformId(projectId, resourceId)

// Set all unknown/null fields to null before saving state
if err := utils.SetModelFieldsToNull(ctx, &minimalModel); err != nil {
    core.LogAndAddError(
        ctx,
        &resp.Diagnostics,
        "Error creating resource",
        fmt.Sprintf("Setting model fields to null: %v", err),
    )
    return
}

diags := resp.State.Set(ctx, minimalModel)
resp.Diagnostics.Append(diags...)
if diags.HasError() {
    return
}
```

**Key points:**
- Extract the resource ID from the API response
- Build the composite Terraform ID using `utils.BuildInternalTerraformId()`
- Call `utils.SetModelFieldsToNull()` to set all non-ID fields to null
- Save the minimal state immediately

**Reference:** `stackit/internal/services/dns/zone/resource.go:343-363`

#### Step 4: Exit Early for Crossplane

After saving minimal state, check if we should skip waiting (for Crossplane/Upjet compatibility):

```go
if !utils.ShouldWait() {
    tflog.Info(ctx, "Skipping wait; async mode for Crossplane/Upjet")
    return
}
```

**Reference:** `stackit/internal/services/dns/zone/resource.go:365-368`

#### Step 5: Wait for Operation with Error Handling

Use the wait handler with proper error handling:

```go
waitResp, err := wait.CreateResourceWaitHandler(ctx, r.client, projectId, resourceId).
    WaitWithContext(ctx)
if err != nil {
    if utils.ShouldIgnoreWaitError(err) {
        tflog.Warn(
            ctx,
            fmt.Sprintf(
                "Resource creation waiting failed: %v. The resource creation was triggered but waiting for completion was interrupted. The resource may still be creating.",
                err,
            ),
        )
        return
    }
    core.LogAndAddError(
        ctx,
        &resp.Diagnostics,
        "Error creating resource",
        fmt.Sprintf("Waiting for resource creation: %v", err),
    )
    return
}
```

**Key points:**
- Use `utils.ShouldIgnoreWaitError(err)` to detect ignorable errors (context cancellation, network interruptions)
- Log a warning instead of failing when the error should be ignored
- Only add a diagnostic error for non-ignorable errors

**Reference:** `stackit/internal/services/dns/zone/resource.go:370-390`

#### Step 6: Map Full Response and Update State

Finally, map the full response to the model and save the complete state:

```go
// Map response body to schema
err = mapFields(ctx, waitResp, &model)
if err != nil {
    core.LogAndAddError(
        ctx,
        &resp.Diagnostics,
        "Error creating resource",
        fmt.Sprintf("Processing API payload: %v", err),
    )
    return
}
// Set state to fully populated data
resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
if resp.Diagnostics.HasError() {
    return
}
tflog.Info(ctx, "Resource created")
```

**Reference:** `stackit/internal/services/dns/zone/resource.go:392-409`

---

## Read Method

### Pattern Overview

The Read method must handle resources that have been deleted outside of Terraform (via the API, console, or other means).

### Implementation Steps

#### Handle 404 and 410 Errors

When calling the Get API, catch 404 (Not Found) and 410 (Gone) errors and remove the resource from state:

```go
func (r *exampleResource) Read(
    ctx context.Context,
    req resource.ReadRequest,
    resp *resource.ReadResponse,
) {
    var model Model
    diags := req.State.Get(ctx, &model)
    resp.Diagnostics.Append(diags...)
    if resp.Diagnostics.HasError() {
        return
    }

    projectId := model.ProjectId.ValueString()
    resourceId := model.ResourceId.ValueString()
    ctx = tflog.SetField(ctx, "project_id", projectId)
    ctx = tflog.SetField(ctx, "resource_id", resourceId)

    resourceResp, err := r.client.GetResource(ctx, projectId, resourceId).Execute()
    if err != nil {
        var oapiErr *oapierror.GenericOpenAPIError
        ok := errors.As(err, &oapiErr)
        if ok &&
            (oapiErr.StatusCode == http.StatusNotFound || oapiErr.StatusCode == http.StatusGone) {
            resp.State.RemoveResource(ctx)
            return
        }
        core.LogAndAddError(
            ctx,
            &resp.Diagnostics,
            "Error reading resource",
            fmt.Sprintf("Calling API: %v", err),
        )
        return
    }

    // Optional: Also check for DELETE_SUCCEEDED state if applicable
    if resourceResp != nil && resourceResp.Resource.State != nil &&
        *resourceResp.Resource.State == api.STATE_DELETE_SUCCEEDED {
        resp.State.RemoveResource(ctx)
        return
    }

    // Map response body to schema
    err = mapFields(ctx, resourceResp, &model)
    if err != nil {
        core.LogAndAddError(
            ctx,
            &resp.Diagnostics,
            "Error reading resource",
            fmt.Sprintf("Processing API payload: %v", err),
        )
        return
    }
    // Set refreshed state
    diags = resp.State.Set(ctx, model)
    resp.Diagnostics.Append(diags...)
    if resp.Diagnostics.HasError() {
        return
    }
    tflog.Info(ctx, "Resource read")
}
```

**Key points:**
- Use `errors.As()` to check if the error is an `oapierror.GenericOpenAPIError`
- Check for both `http.StatusNotFound` (404) and `http.StatusGone` (410)
- Call `resp.State.RemoveResource(ctx)` to remove the resource from state
- Optionally check for `DELETE_SUCCEEDED` state if the API supports it

**Reference:** `stackit/internal/services/dns/zone/resource.go:428-449`

---

## Update Method

### Pattern Overview

The Update method must support Crossplane async mode and handle wait errors gracefully.

### Implementation Steps

#### Step 1: Add Crossplane Skip Check

After calling the Update API, check if we should skip waiting:

```go
func (r *exampleResource) Update(
    ctx context.Context,
    req resource.UpdateRequest,
    resp *resource.UpdateResponse,
) {
    // Retrieve values from plan
    var model Model
    diags := req.Plan.Get(ctx, &model)
    resp.Diagnostics.Append(diags...)
    if resp.Diagnostics.HasError() {
        return
    }

    projectId := model.ProjectId.ValueString()
    resourceId := model.ResourceId.ValueString()
    ctx = tflog.SetField(ctx, "project_id", projectId)
    ctx = tflog.SetField(ctx, "resource_id", resourceId)

    // Generate API request body from model
    payload, err := toUpdatePayload(&model)
    if err != nil {
        core.LogAndAddError(
            ctx,
            &resp.Diagnostics,
            "Error updating resource",
            fmt.Sprintf("Creating API payload: %v", err),
        )
        return
    }

    // Update existing resource
    _, err = r.client.PartialUpdateResource(ctx, projectId, resourceId).
        PartialUpdatePayload(*payload).
        Execute()
    if err != nil {
        core.LogAndAddError(
            ctx,
            &resp.Diagnostics,
            "Error updating resource",
            fmt.Sprintf("Calling API: %v", err),
        )
        return
    }

    if !utils.ShouldWait() {
        tflog.Info(ctx, "Skipping wait; async mode for Crossplane/Upjet")
        return
    }

    // Continue with wait handler...
}
```

**Reference:** `stackit/internal/services/dns/zone/resource.go:514-517`

#### Step 2: Add Wait Handler with Error Handling

Use the same error handling pattern as in Create:

```go
waitResp, err := wait.PartialUpdateResourceWaitHandler(ctx, r.client, projectId, resourceId).
    WaitWithContext(ctx)
if err != nil {
    if utils.ShouldIgnoreWaitError(err) {
        tflog.Warn(
            ctx,
            fmt.Sprintf(
                "Resource update waiting failed: %v. The resource update was triggered but waiting for completion was interrupted. The resource may still be updating.",
                err,
            ),
        )
        return
    }
    core.LogAndAddError(
        ctx,
        &resp.Diagnostics,
        "Error updating resource",
        fmt.Sprintf("Waiting for resource update: %v", err),
    )
    return
}

err = mapFields(ctx, waitResp, &model)
if err != nil {
    core.LogAndAddError(
        ctx,
        &resp.Diagnostics,
        "Error updating resource",
        fmt.Sprintf("Processing API payload: %v", err),
    )
    return
}
diags = resp.State.Set(ctx, model)
resp.Diagnostics.Append(diags...)
if resp.Diagnostics.HasError() {
    return
}
tflog.Info(ctx, "Resource updated")
```

**Reference:** `stackit/internal/services/dns/zone/resource.go:519-556`

---

## Delete Method

### Pattern Overview

The Delete method must be idempotent - calling delete on an already-deleted resource should succeed, not fail.

### Implementation Steps

#### Step 1: Handle 404/410 in Delete API Call

Catch 404 and 410 errors when calling the Delete API:

```go
func (r *exampleResource) Delete(
    ctx context.Context,
    req resource.DeleteRequest,
    resp *resource.DeleteResponse,
) {
    // Retrieve values from state
    var model Model
    diags := req.State.Get(ctx, &model)
    resp.Diagnostics.Append(diags...)
    if resp.Diagnostics.HasError() {
        return
    }

    projectId := model.ProjectId.ValueString()
    resourceId := model.ResourceId.ValueString()
    ctx = tflog.SetField(ctx, "project_id", projectId)
    ctx = tflog.SetField(ctx, "resource_id", resourceId)

    // Delete existing resource
    _, err := r.client.DeleteResource(ctx, projectId, resourceId).Execute()
    if err != nil {
        // If resource is already gone (404 or 410), treat as success for idempotency
        var oapiErr *oapierror.GenericOpenAPIError
        ok := errors.As(err, &oapiErr)
        if ok &&
            (oapiErr.StatusCode == http.StatusNotFound || oapiErr.StatusCode == http.StatusGone) {
            tflog.Info(ctx, "Resource already deleted")
            return
        }
        core.LogAndAddError(
            ctx,
            &resp.Diagnostics,
            "Error deleting resource",
            fmt.Sprintf("Calling API: %v", err),
        )
        return
    }

    if !utils.ShouldWait() {
        tflog.Info(ctx, "Skipping wait; async mode for Crossplane/Upjet")
        return
    }

    // Continue with wait handler...
}
```

**Key points:**
- Check for 404/410 errors after calling the Delete API
- Return successfully (without error) if the resource is already gone
- Log an informational message for debugging

**Reference:** `stackit/internal/services/dns/zone/resource.go:578-596`

#### Step 2: Add Wait Handler with Error Handling

Use the same error handling pattern:

```go
_, err = wait.DeleteResourceWaitHandler(ctx, r.client, projectId, resourceId).WaitWithContext(ctx)
if err != nil {
    if utils.ShouldIgnoreWaitError(err) {
        tflog.Warn(
            ctx,
            fmt.Sprintf(
                "Resource deletion waiting failed: %v. The resource deletion was triggered but waiting for completion was interrupted. The resource may still be deleting.",
                err,
            ),
        )
        return
    }
    core.LogAndAddError(
        ctx,
        &resp.Diagnostics,
        "Error deleting resource",
        fmt.Sprintf("Waiting for resource deletion: %v", err),
    )
    return
}

tflog.Info(ctx, "Resource deleted")
```

**Reference:** `stackit/internal/services/dns/zone/resource.go:603-624`

---

## Utility Functions Reference

### `utils.BuildInternalTerraformId(parts ...string) types.String`

Builds the composite Terraform ID from multiple parts (typically project ID and resource ID).

```go
model.Id = utils.BuildInternalTerraformId(projectId, resourceId)
```

### `utils.SetModelFieldsToNull(ctx context.Context, model interface{}) error`

Sets all fields in the model to null except for explicitly set values. Used when creating minimal state.

```go
if err := utils.SetModelFieldsToNull(ctx, &minimalModel); err != nil {
    // handle error
}
```

### `utils.ShouldWait() bool`

Returns false when running in Crossplane/Upjet async mode, true otherwise.

```go
if !utils.ShouldWait() {
    tflog.Info(ctx, "Skipping wait; async mode for Crossplane/Upjet")
    return
}
```

### `utils.ShouldIgnoreWaitError(err error) bool`

Returns true if the wait error should be ignored (context cancellation, network interruption) rather than failing the operation.

```go
if utils.ShouldIgnoreWaitError(err) {
    tflog.Warn(ctx, fmt.Sprintf("Waiting interrupted: %v", err))
    return
}
```

---

## Checklist for Implementing the Pattern

Use this checklist when implementing idempotency fixes in a resource:

### Create Method
- [ ] Create `minimalModel` from plan
- [ ] After API call succeeds, populate `minimalModel` with IDs
- [ ] Call `utils.SetModelFieldsToNull()` on `minimalModel`
- [ ] Save `minimalModel` to state immediately
- [ ] Add `utils.ShouldWait()` check before wait handler
- [ ] Wrap wait handler error with `utils.ShouldIgnoreWaitError()` check
- [ ] Map full response and update state after wait completes

### Read Method
- [ ] Catch 404 and 410 errors from Get API call
- [ ] Call `resp.State.RemoveResource(ctx)` on 404/410
- [ ] Optionally check for `DELETE_SUCCEEDED` state

### Update Method
- [ ] Add `utils.ShouldWait()` check before wait handler
- [ ] Wrap wait handler error with `utils.ShouldIgnoreWaitError()` check

### Delete Method
- [ ] Catch 404 and 410 errors from Delete API call
- [ ] Return successfully on 404/410 for idempotency
- [ ] Add `utils.ShouldWait()` check before wait handler
- [ ] Wrap wait handler error with `utils.ShouldIgnoreWaitError()` check
