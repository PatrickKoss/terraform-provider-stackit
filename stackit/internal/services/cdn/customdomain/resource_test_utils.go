package cdn

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stackitcloud/stackit-sdk-go/core/config"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/cdn"
)

// TestContext holds common test setup
type TestContext struct {
	T          *testing.T
	Server     *httptest.Server
	Client     *cdn.APIClient
	Resource   *customDomainResource
	Router     *mux.Router
	Ctx        context.Context
	CancelFunc context.CancelFunc
}

// NewTestContext creates a new test context with mock server
func NewTestContext(t *testing.T) *TestContext {
	router := mux.NewRouter()
	server := httptest.NewServer(router)

	client, err := cdn.NewAPIClient(
		config.WithEndpoint(server.URL),
		config.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	resource := &customDomainResource{
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

// SetupPutCustomDomainHandler adds mock handler for PutCustomDomain API
func (tc *TestContext) SetupPutCustomDomainHandler(callCounter *int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("PUT")
}

// SetupGetCustomDomainHandler adds mock handler for GetCustomDomain API
// Valid status values: CREATING, ACTIVE, UPDATING, DELETING, ERROR
func (tc *TestContext) SetupGetCustomDomainHandler(name, status string) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Return JSON directly
		jsonResp := fmt.Sprintf(`{
			"customDomain": {
				"name": "%s",
				"status": "%s"
			},
			"certificate": {
				"type": "managed"
			}
		}`, name, status)
		w.Write([]byte(jsonResp))
	}).Methods("GET")
}

// SetupGetCustomDomainHandlerWithCertificate adds mock handler that returns custom certificate
func (tc *TestContext) SetupGetCustomDomainHandlerWithCertificate(name, status string, certVersion int64) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		jsonResp := fmt.Sprintf(`{
			"customDomain": {
				"name": "%s",
				"status": "%s"
			},
			"certificate": {
				"type": "custom",
				"version": %d
			}
		}`, name, status, certVersion)
		w.Write([]byte(jsonResp))
	}).Methods("GET")
}

// SetupDeleteCustomDomainHandler adds mock handler for DeleteCustomDomain API
func (tc *TestContext) SetupDeleteCustomDomainHandler(callCounter *int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("DELETE")
}

// SetupDeleteCustomDomainHandlerWithStatus returns specific status code
func (tc *TestContext) SetupDeleteCustomDomainHandlerWithStatus(statusCode int, callCounter *int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)

		if statusCode == http.StatusNotFound {
			w.Write([]byte(`{"error": "not found"}`))
		} else if statusCode == http.StatusGone {
			w.Write([]byte(`{"error": "gone"}`))
		} else {
			w.Write([]byte("{}"))
		}
	}).Methods("DELETE")
}

// SetupGetCustomDomainHandlerNotFound returns 404
func (tc *TestContext) SetupGetCustomDomainHandlerNotFound() {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "not found"}`))
	}).Methods("GET")
}

// SetupGetCustomDomainHandlerGone returns 410
func (tc *TestContext) SetupGetCustomDomainHandlerGone() {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`{"error": "gone"}`))
	}).Methods("GET")
}

// SetupGetCustomDomainHandlerError returns 500
func (tc *TestContext) SetupGetCustomDomainHandlerError() {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}).Methods("GET")
}

// SetupPutCustomDomainHandlerError returns 500
func (tc *TestContext) SetupPutCustomDomainHandlerError(callCounter *int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}).Methods("PUT")
}

// SetupDeleteCustomDomainHandlerError returns 500
func (tc *TestContext) SetupDeleteCustomDomainHandlerError(callCounter *int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}).Methods("DELETE")
}

// AssertStateFieldEquals checks a single field in the model
func AssertStateFieldEquals(t *testing.T, fieldName string, got, want types.String) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s mismatch: got=%s, want=%s", fieldName, got.ValueString(), want.ValueString())
	}
}

// CreateRequest builds a create request with the given model
func CreateRequest(ctx context.Context, schema resource.SchemaResponse, model CustomDomainModel) resource.CreateRequest {
	req := resource.CreateRequest{}
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.Plan.Set(ctx, model)
	return req
}

// CreateResponse builds a create response
func CreateResponse(schema resource.SchemaResponse) *resource.CreateResponse {
	resp := &resource.CreateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	return resp
}

// UpdateRequest builds an update request with the given model
func UpdateRequest(ctx context.Context, schema resource.SchemaResponse, model CustomDomainModel) resource.UpdateRequest {
	req := resource.UpdateRequest{}
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.Plan.Set(ctx, model)
	return req
}

// UpdateResponse creates a test Update response
// Optionally initialize with current state to simulate Terraform framework behavior
func UpdateResponse(ctx context.Context, schema resource.SchemaResponse, currentState *CustomDomainModel) *resource.UpdateResponse {
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

// ReadRequest builds a read request with the given model
func ReadRequest(ctx context.Context, schema resource.SchemaResponse, model CustomDomainModel) resource.ReadRequest {
	req := resource.ReadRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, model)
	return req
}

// ReadResponse creates a test Read response
func ReadResponse(ctx context.Context, schema resource.SchemaResponse, currentState *CustomDomainModel) *resource.ReadResponse {
	resp := &resource.ReadResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}

// DeleteRequest builds a delete request with the given model
func DeleteRequest(ctx context.Context, schema resource.SchemaResponse, model CustomDomainModel) resource.DeleteRequest {
	req := resource.DeleteRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, model)
	return req
}

// DeleteResponse creates a test Delete response
// Optionally initialize with current state to simulate Terraform framework behavior
func DeleteResponse(ctx context.Context, schema resource.SchemaResponse, currentState *CustomDomainModel) *resource.DeleteResponse {
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

// BuildTestModel creates a test model with common values
func BuildTestModel(projectId, distributionId, name string) CustomDomainModel {
	return CustomDomainModel{
		ID:             types.StringNull(), // Will be computed
		ProjectId:      types.StringValue(projectId),
		DistributionId: types.StringValue(distributionId),
		Name:           types.StringValue(name),
		Status:         types.StringNull(), // Will be computed
		Errors:         types.ListNull(types.StringType),
		Certificate:    types.ObjectNull(certificateTypes),
	}
}

// BuildTestModelWithCertificate creates a test model with custom certificate
func BuildTestModelWithCertificate(projectId, distributionId, name, cert, privateKey string) CustomDomainModel {
	certAttrs := map[string]attr.Value{
		"certificate": types.StringValue(cert),
		"private_key": types.StringValue(privateKey),
		"version":     types.Int64Null(),
	}
	certObj, _ := types.ObjectValue(certificateTypes, certAttrs)

	return CustomDomainModel{
		ProjectId:      types.StringValue(projectId),
		DistributionId: types.StringValue(distributionId),
		Name:           types.StringValue(name),
		Certificate:    certObj,
	}
}

// PtrString returns a pointer to a string
func PtrString(s string) *string {
	return utils.Ptr(s)
}

// PtrInt64 returns a pointer to an int64
func PtrInt64(i int64) *int64 {
	return utils.Ptr(i)
}
