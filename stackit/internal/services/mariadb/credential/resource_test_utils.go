package mariadb

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
	"github.com/stackitcloud/stackit-sdk-go/services/mariadb"
)

// TestContext holds common test setup
type TestContext struct {
	T          *testing.T
	Server     *httptest.Server
	Client     *mariadb.APIClient
	Resource   *credentialResource
	Router     *mux.Router
	Ctx        context.Context
	CancelFunc context.CancelFunc
}

// NewTestContext creates a new test context with mock server
func NewTestContext(t *testing.T) *TestContext {
	router := mux.NewRouter()
	server := httptest.NewServer(router)

	client, err := mariadb.NewAPIClient(
		config.WithEndpoint(server.URL),
		config.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	resource := &credentialResource{
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

// SetupCreateCredentialHandler adds mock handler for Create API
func (tc *TestContext) SetupCreateCredentialHandler(credentialId string, callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)

		resp := mariadb.CredentialsResponse{
			Id:  utils.Ptr(credentialId),
			Uri: utils.Ptr("mysql://user:pass@host:3306/db"),
		}
		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}).Methods("POST")
}

// SetupGetCredentialHandler adds mock handler for Get API
func (tc *TestContext) SetupGetCredentialHandler(credentialId, username, host string, port int64) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials/{credentialId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Return JSON directly to avoid SDK type issues
		jsonResp := fmt.Sprintf(`{
			"id": "%s",
			"uri": "mysql://%s:password@%s:%d/db",
			"raw": {
				"credentials": {
					"host": "%s",
					"hosts": ["%s"],
					"name": "db",
					"password": "password",
					"port": %d,
					"uri": "mysql://%s:password@%s:%d/db",
					"username": "%s"
				}
			}
		}`, credentialId, username, host, port, host, host, port, username, host, port, username)
		w.Write([]byte(jsonResp))
	}).Methods("GET")
}

// SetupDeleteCredentialHandler adds mock handler for Delete API
func (tc *TestContext) SetupDeleteCredentialHandler(callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials/{credentialId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("DELETE")
}

// SetupDeleteCredentialHandler404 adds mock handler that returns 404
func (tc *TestContext) SetupDeleteCredentialHandler404() {
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials/{credentialId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "not found"}`))
	}).Methods("DELETE")
}

// SetupDeleteCredentialHandler410 adds mock handler that returns 410 Gone
func (tc *TestContext) SetupDeleteCredentialHandler410() {
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials/{credentialId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`{"error": "gone"}`))
	}).Methods("DELETE")
}

// SetupGetCredentialHandler404 adds mock handler that returns 404
func (tc *TestContext) SetupGetCredentialHandler404() {
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials/{credentialId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "not found"}`))
	}).Methods("GET")
}

// SetupGetCredentialHandler410 adds mock handler that returns 410 Gone
func (tc *TestContext) SetupGetCredentialHandler410() {
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials/{credentialId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`{"error": "gone"}`))
	}).Methods("GET")
}

// SetupGetCredentialHandler500 adds mock handler that returns 500 error
func (tc *TestContext) SetupGetCredentialHandler500() {
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials/{credentialId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
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
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.Plan.Set(ctx, model)
	return req
}

// CreateResponse creates a test Create response
func CreateResponse(schema resource.SchemaResponse) *resource.CreateResponse {
	resp := &resource.CreateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	return resp
}

// ReadRequest creates a test Read request with current state
func ReadRequest(ctx context.Context, schema resource.SchemaResponse, currentState Model) resource.ReadRequest {
	req := resource.ReadRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, currentState)
	return req
}

// ReadResponse creates a test Read response
func ReadResponse(schema resource.SchemaResponse) *resource.ReadResponse {
	resp := &resource.ReadResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	return resp
}

// DeleteRequest creates a test Delete request with current state
func DeleteRequest(ctx context.Context, schema resource.SchemaResponse, currentState Model) resource.DeleteRequest {
	req := resource.DeleteRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, currentState)
	return req
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

// CreateTestModel creates a test model with common values and properly initialized null fields
func CreateTestModel(projectId, instanceId string) Model {
	return Model{
		ProjectId:    types.StringValue(projectId),
		InstanceId:   types.StringValue(instanceId),
		Id:           types.StringNull(),
		CredentialId: types.StringNull(),
		Host:         types.StringNull(),
		Hosts:        types.ListNull(types.StringType),
		Name:         types.StringNull(),
		Password:     types.StringNull(),
		Port:         types.Int64Null(),
		Uri:          types.StringNull(),
		Username:     types.StringNull(),
	}
}

// CreateFullTestModel creates a fully populated test model (for Read/Delete tests)
func CreateFullTestModel(projectId, instanceId, credentialId, username, host string) Model {
	return Model{
		Id:           types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, instanceId, credentialId)),
		CredentialId: types.StringValue(credentialId),
		ProjectId:    types.StringValue(projectId),
		InstanceId:   types.StringValue(instanceId),
		Username:     types.StringValue(username),
		Host:         types.StringValue(host),
		Hosts:        types.ListNull(types.StringType),
		Name:         types.StringNull(),
		Password:     types.StringNull(),
		Port:         types.Int64Null(),
		Uri:          types.StringNull(),
	}
}
