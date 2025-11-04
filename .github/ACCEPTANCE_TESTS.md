# Per-Service Acceptance Testing

This document describes the automated per-service acceptance testing system for pull requests.

## Quick Start for Maintainers

**TL;DR**: When a PR modifies services, just comment `/test <service-name>` to run acceptance tests.

```bash
/test mariadb     # Run tests for mariadb service
/test redis       # Run tests for redis service
```

That's it! The bot handles everything else.

---

## Overview

When you create a pull request that modifies one or more services in `stackit/internal/services/`, the system will:

1. **Detect** which services have been changed
2. **Create pending status checks** for each changed service
3. **Post a comment** on the PR with commands to trigger acceptance tests for each service

## How It Works

### 1. Automatic Detection

When you open or update a PR that changes files in `stackit/internal/services/`, the `pr-service-detection` workflow runs automatically and:

- Analyzes the diff to identify changed services
- Creates a pending status check for each service (e.g., `acceptance-test/mariadb`)
- Posts a comment listing all services that need testing

### 2. Running Tests with Commands

The PR comment will include simple commands to trigger acceptance tests. Example:

```
## 🔬 Acceptance Tests Required

The following services have been modified and require acceptance testing:

### `mariadb` 🟡 pending

**To run tests, comment:**
```
/test mariadb
```

### `redis` 🟡 pending

**To run tests, comment:**
```
/test redis
```
```

**How it works:**

1. **Maintainer** simply posts a comment with `/test <service>` (e.g., `/test mariadb`)
2. The system automatically:
   - Validates the commenter has write/maintain/admin permissions
   - Checks out the PR branch
   - Runs acceptance tests for that service
   - Updates the commit status check
   - Posts results back to the PR

**Important**: Only users with `write`, `maintain`, or `admin` permissions can trigger tests. PR authors without these permissions will need to request that a maintainer run the tests.

### 3. Command Formats

You can use either of these formats:

```bash
/test mariadb         # Standard format
/test-mariadb         # Alternative format with dash
```

Both will trigger the same acceptance tests for the `mariadb` service

### 4. Alternative: Manual Workflow Dispatch

While the comment-based approach is recommended, you can also trigger tests manually via GitHub Actions:

1. Go to **Actions** → **Service Acceptance Tests**
2. Click **Run workflow**
3. Fill in:
   - **service**: The service name (e.g., `mariadb`)
   - **pr_number**: The PR number
4. Click **Run workflow**

This is useful for debugging or edge cases, but the `/test` command is much faster.

### 5. Status Checks and Merging

Each changed service gets its own status check in the format `acceptance-test/{service}`. These checks:

- Start as **pending** when the PR is opened
- Update to **running** when tests are triggered
- Complete as **success** or **failure** based on test results

**Merge Requirements**: All acceptance tests must pass before the PR can be merged.

## Running Tests Locally

You can also run acceptance tests locally for a specific service:

```bash
make test-acceptance-service \
  SERVICE=mariadb \
  TF_ACC_PROJECT_ID=your-project-id \
  TF_ACC_ORGANIZATION_ID=your-org-id \
  TF_ACC_REGION=eu01
```

Required environment variables:
- `STACKIT_SERVICE_ACCOUNT_TOKEN`
- `TF_ACC_TEST_PROJECT_SERVICE_ACCOUNT_EMAIL`
- `TF_ACC_TEST_PROJECT_SERVICE_ACCOUNT_TOKEN`
- `TF_ACC_TEST_PROJECT_PARENT_CONTAINER_ID`
- `TF_ACC_TEST_PROJECT_PARENT_UUID`
- `TF_ACC_TEST_PROJECT_USER_EMAIL`

## Setting Up Branch Protection (For Repository Admins)

To enforce acceptance testing before merging:

1. Go to **Settings** > **Branches** > **Branch protection rules**
2. Edit the rule for your main branch (e.g., `main`)
3. Enable **"Require status checks to pass before merging"**
4. Search for and select status checks matching the pattern:
   - `acceptance-test/mariadb`
   - `acceptance-test/redis`
   - etc.

   Or use a branch protection rule that requires all status checks starting with `acceptance-test/` to pass.

5. **Optional**: Enable "Require branches to be up to date before merging"

## Troubleshooting

### "User does not have permission to run acceptance tests"

This means you don't have write access to the repository. Contact a repository maintainer to trigger the tests for you.

### Command not recognized

If your `/test` command doesn't trigger anything:
- Make sure you're commenting on a pull request (not an issue)
- Ensure the command is on its own line
- Check the format: `/test <service>` or `/test-<service>`
- Verify the service name matches exactly (case-sensitive)

### Service not detected

Make sure your changes are in the `stackit/internal/services/{service}/` directory. The detection workflow only looks at files in this path.

### Tests timing out

Acceptance tests have a 30-minute timeout. If tests consistently timeout, consider:
- Breaking up large test suites
- Optimizing resource creation/deletion
- Checking for resource leaks

### Status check stuck on "pending"

If a status check remains pending:
1. Check that the acceptance tests were actually triggered
2. Look at the workflow run logs for errors
3. Manually re-run the workflow if needed

## Architecture

### Workflows

1. **pr-service-detection.yaml**
   - Trigger: PR opened/synchronized on `stackit/internal/services/**`
   - Detects changed services by analyzing git diff
   - Creates pending status checks for each service
   - Posts PR comment with test commands

2. **test-command.yaml** (Primary method)
   - Trigger: Issue comment starting with `/test`
   - Validates commenter has write/maintain/admin permissions
   - Parses service name from command
   - Runs acceptance tests for that service
   - Updates status checks and posts results

3. **service-acc-test.yaml** (Fallback method)
   - Trigger: Manual (workflow_dispatch)
   - Alternative way to run tests via Actions UI
   - Same functionality as test-command but requires more clicks

### Makefile Targets

- `make test-acceptance-tf`: Run all acceptance tests (existing)
- `make test-acceptance-service SERVICE=<name>`: Run acceptance tests for a specific service (new)

## Examples

### Example PR Workflow

1. Developer creates PR modifying `stackit/internal/services/mariadb/` and `stackit/internal/services/redis/`
2. Bot automatically posts:
   ```
   ## 🔬 Acceptance Tests Required

   ### `mariadb` 🟡 pending
   **To run tests, comment:**
   ```
   /test mariadb
   ```

   ### `redis` 🟡 pending
   **To run tests, comment:**
   ```
   /test redis
   ```
   ```
3. Maintainer comments: `/test mariadb`
4. Bot reacts with 🚀 and replies "Starting acceptance tests..."
5. Tests run and pass → status check updates to ✅
6. Bot comments: "✅ Acceptance tests for `mariadb` PASSED"
7. Maintainer comments: `/test redis`
8. Tests run and pass → status check updates to ✅
9. Bot comments: "✅ Acceptance tests for `redis` PASSED"
10. All checks green → PR can be merged

### Example Local Testing

```bash
# Test the mariadb service locally
export STACKIT_SERVICE_ACCOUNT_TOKEN="your-token"
export TF_ACC_TEST_PROJECT_SERVICE_ACCOUNT_EMAIL="email@example.com"
# ... set other required env vars

make test-acceptance-service \
  SERVICE=mariadb \
  TF_ACC_PROJECT_ID=abc123 \
  TF_ACC_ORGANIZATION_ID=org456 \
  TF_ACC_REGION=eu01
```
