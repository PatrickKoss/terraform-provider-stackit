package instance

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
	"github.com/stackitcloud/stackit-sdk-go/services/git"
)

// TestContext holds common test setup
type TestContext struct {
	T          *testing.T
	Server     *httptest.Server
	Client     *git.APIClient
	Resource   *gitResource
	Router     *mux.Router
	Ctx        context.Context
	CancelFunc context.CancelFunc
}

// NewTestContext creates a new test context with mock server
func NewTestContext(t *testing.T) *TestContext {
	router := mux.NewRouter()

	// Add logging middleware to debug URL patterns
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Logf("Request: %s %s", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
		})
	})

	server := httptest.NewServer(router)

	client, err := git.NewAPIClient(
		config.WithEndpoint(server.URL),
		config.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	resource := &gitResource{
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

// SetupCreateInstanceHandler adds mock handler for Create API
func (tc *TestContext) SetupCreateInstanceHandler(instanceId string, callCounter *int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/instances", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)

		resp := git.Instance{
			Id: utils.Ptr(instanceId),
		}
		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}).Methods("POST")
}

// SetupGetInstanceHandler adds mock handler for Get API
func (tc *TestContext) SetupGetInstanceHandler(instanceId, name, state string) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Return JSON directly to avoid SDK type issues
		// Note: The wait handler checks the "state" field (not "status")
		jsonResp := fmt.Sprintf(`{
			"id": "%s",
			"name": "%s",
			"state": "%s",
			"flavor": "git-100",
			"version": "1.0.0",
			"url": "https://git.example.com",
			"consumedDisk": "1024",
			"consumedObjectStorage": "512",
			"created": "2023-01-01T00:00:00Z"
		}`, instanceId, name, state)
		w.Write([]byte(jsonResp))
	}).Methods("GET")
}

// SetupGetInstanceHandlerWithStatusCode adds mock handler for Get API with custom status code
func (tc *TestContext) SetupGetInstanceHandlerWithStatusCode(statusCode int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode == http.StatusNotFound || statusCode == http.StatusGone {
			w.Write([]byte(`{"error": "instance not found"}`))
		} else {
			w.Write([]byte(`{"error": "internal server error"}`))
		}
	}).Methods("GET")
}

// SetupDeleteInstanceHandler adds mock handler for Delete API
func (tc *TestContext) SetupDeleteInstanceHandler(callCounter *int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("DELETE")
}

// SetupDeleteInstanceHandlerWithStatusCode adds mock handler for Delete API with custom status code
func (tc *TestContext) SetupDeleteInstanceHandlerWithStatusCode(statusCode int, callCounter *int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode == http.StatusNotFound {
			w.Write([]byte(`{"error": "instance not found"}`))
		} else if statusCode == http.StatusGone {
			w.Write([]byte(`{"error": "instance gone"}`))
		} else {
			w.Write([]byte(`{"error": "internal server error"}`))
		}
	}).Methods("DELETE")
}

// SetupDeleteWaitHandlerWithGone sets up the Get handler to return 410 Gone (deletion complete)
func (tc *TestContext) SetupDeleteWaitHandlerWithGone() {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`{"error": "instance gone"}`))
	}).Methods("GET")
}

// AssertStateFieldEquals checks a single field in the model
func AssertStateFieldEquals(t *testing.T, fieldName string, got, want types.String) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s mismatch: got=%s, want=%s", fieldName, got.ValueString(), want.ValueString())
	}
}

// CreateRequest creates a test Create request
func CreateRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.CreateRequest {
	req := resource.CreateRequest{}

	// Initialize plan with null value, then set the model
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
	}

	diags := req.Plan.Set(ctx, model)
	if diags.HasError() {
		panic(fmt.Sprintf("Failed to set plan: %v", diags.Errors()))
	}

	return req
}

// CreateResponse creates a test Create response
func CreateResponse(schema resource.SchemaResponse) *resource.CreateResponse {
	resp := &resource.CreateResponse{}

	// Initialize state with null value
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(context.Background()), nil),
	}
	return resp
}

// ReadRequest creates a test Read request
func ReadRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.ReadRequest {
	req := resource.ReadRequest{}

	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
	}

	diags := req.State.Set(ctx, model)
	if diags.HasError() {
		panic(fmt.Sprintf("Failed to set state: %v", diags.Errors()))
	}

	return req
}

// ReadResponse creates a test Read response
// Optionally initialize with current state to simulate Terraform framework behavior
func ReadResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.ReadResponse {
	resp := &resource.ReadResponse{}

	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
	}

	// Initialize with current state to simulate framework behavior
	if currentState != nil {
		diags := resp.State.Set(ctx, *currentState)
		if diags.HasError() {
			panic(fmt.Sprintf("Failed to set state: %v", diags.Errors()))
		}
	}
	return resp
}

// DeleteRequest creates a test Delete request
func DeleteRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.DeleteRequest {
	req := resource.DeleteRequest{}

	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
	}

	diags := req.State.Set(ctx, model)
	if diags.HasError() {
		panic(fmt.Sprintf("Failed to set state: %v", diags.Errors()))
	}

	return req
}

// DeleteResponse creates a test Delete response
// Optionally initialize with current state to simulate Terraform framework behavior
func DeleteResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.DeleteResponse {
	resp := &resource.DeleteResponse{}

	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
	}

	// Initialize with current state to simulate framework behavior
	// When Delete errors without calling State.RemoveResource(), this state is preserved
	if currentState != nil {
		diags := resp.State.Set(ctx, *currentState)
		if diags.HasError() {
			panic(fmt.Sprintf("Failed to set state: %v", diags.Errors()))
		}
	}
	return resp
}
