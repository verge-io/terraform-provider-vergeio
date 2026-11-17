// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"

	vergeio "terraform-provider-vergeio/internal/client"
)

func TestAPIKeyEphemeralMetadataAndSchema(t *testing.T) {
	resource := NewAPIKeyEphemeralResource()
	meta := &ephemeral.MetadataResponse{}
	resource.Metadata(context.Background(), ephemeral.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_api_key" {
		t.Fatalf("type = %s", meta.TypeName)
	}

	schemaResp := &ephemeral.SchemaResponse{}
	resource.Schema(context.Background(), ephemeral.SchemaRequest{}, schemaResp)
	token, ok := schemaResp.Schema.Attributes["token"].(schema.StringAttribute)
	if !ok || !token.Computed || !token.Sensitive {
		t.Fatalf("token = %#v", schemaResp.Schema.Attributes["token"])
	}
	for _, name := range []string{"user_id", "name", "ttl_seconds"} {
		if !schemaResp.Schema.Attributes[name].IsRequired() {
			t.Fatalf("%s should be required", name)
		}
	}
	if schemaResp.Schema.Attributes["id"].IsRequired() || !schemaResp.Schema.Attributes["id"].IsComputed() {
		t.Fatal("id should be computed")
	}
}

func TestAPIKeyEphemeralConfigureRejectsWrongClient(t *testing.T) {
	resource := &APIKeyEphemeralResource{}
	resp := &ephemeral.ConfigureResponse{}
	resource.Configure(context.Background(), ephemeral.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a configure error")
	}
	if resource.sdk != nil {
		t.Fatal("sdk was set")
	}
}

func TestOpenAPIKeyMintsAndCloseDeletes(t *testing.T) {
	store := newAPIKeyStore()
	sdk := apiKeySDK(t, store)
	opened, private, renewAt, err := openAPIKey(context.Background(), sdk, apiKeyModel{
		UserID:      types.Int32Value(10),
		Name:        types.StringValue("terraform-tenant"),
		Description: types.StringValue("this run"),
		TTLSeconds:  types.Int64Value(3600),
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Token.ValueString() != "secret-token" || opened.ID.ValueString() != "1" {
		t.Fatalf("opened = id %s token %s", opened.ID, opened.Token)
	}
	if private.ID != 1 || private.TTLSeconds != 3600 {
		t.Fatalf("private = %#v", private)
	}
	if opened.Expires.ValueInt64() <= time.Now().Unix() {
		t.Fatalf("expires = %d", opened.Expires.ValueInt64())
	}
	if !renewAt.After(time.Now()) {
		t.Fatalf("renew at %s", renewAt)
	}
	body := store.createBody()
	if body["expires_type"] != vergeos.APIKeyExpiresDate || body["user"] != float64(10) || body["name"] != "terraform-tenant" {
		t.Fatalf("create body = %#v", body)
	}
	if err := closeAPIKey(context.Background(), sdk, private); err != nil {
		t.Fatal(err)
	}
	if !store.deleted(1) {
		t.Fatalf("deleted = %#v", store.deletedIDs())
	}
	if err := closeAPIKey(context.Background(), sdk, private); err != nil {
		t.Fatal(err)
	}
}

func TestOpenAPIKeyReplacesExistingName(t *testing.T) {
	store := newAPIKeyStore()
	store.seed(7, 10, "terraform-tenant")
	sdk := apiKeySDK(t, store)
	_, private, _, err := openAPIKey(context.Background(), sdk, apiKeyModel{
		UserID:     types.Int32Value(10),
		Name:       types.StringValue("terraform-tenant"),
		TTLSeconds: types.Int64Value(600),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.deleted(7) {
		t.Fatal("existing key was not deleted")
	}
	if private.ID == 7 || private.ID == 0 {
		t.Fatalf("new id = %d", private.ID)
	}
}

func TestOpenAPIKeyRejectsMissingToken(t *testing.T) {
	store := newAPIKeyStore()
	store.omitToken = true
	sdk := apiKeySDK(t, store)
	_, _, _, err := openAPIKey(context.Background(), sdk, apiKeyModel{
		UserID:     types.Int32Value(10),
		Name:       types.StringValue("terraform-tenant"),
		TTLSeconds: types.Int64Value(600),
	})
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("error = %v", err)
	}
	if store.liveCount() != 0 {
		t.Fatalf("live keys = %d, want the key deleted when no token is returned", store.liveCount())
	}
}

func TestRenewAPIKeyExtendsExpiry(t *testing.T) {
	store := newAPIKeyStore()
	store.seed(4, 10, "terraform-tenant")
	sdk := apiKeySDK(t, store)
	renewAt, err := renewAPIKey(context.Background(), sdk, apiKeyPrivateState{ID: 4, TTLSeconds: 600})
	if err != nil {
		t.Fatal(err)
	}
	if !renewAt.After(time.Now()) {
		t.Fatalf("renew at %s", renewAt)
	}
	if store.updateBody()["expires_type"] != vergeos.APIKeyExpiresDate {
		t.Fatalf("update = %#v", store.updateBody())
	}
}

func TestRenewLead(t *testing.T) {
	if got := renewLead(time.Hour); got != time.Minute {
		t.Fatalf("hour lead = %s", got)
	}
	if got := renewLead(60 * time.Second); got != 15*time.Second {
		t.Fatalf("short lead = %s", got)
	}
	if got := renewLead(10 * time.Second); got != 5*time.Second {
		t.Fatalf("tiny lead = %s", got)
	}
}

type apiKeyRecord struct {
	id     int
	userID int
	name   string
}

type apiKeyStore struct {
	mu         sync.Mutex
	next       int
	keys       map[int]apiKeyRecord
	deletedSet map[int]bool
	created    map[string]any
	updated    map[string]any
	omitToken  bool
}

func newAPIKeyStore() *apiKeyStore {
	return &apiKeyStore{
		next:       1,
		keys:       map[int]apiKeyRecord{},
		deletedSet: map[int]bool{},
	}
}

func (s *apiKeyStore) seed(id, userID int, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[id] = apiKeyRecord{id: id, userID: userID, name: name}
	if id >= s.next {
		s.next = id + 1
	}
}

func (s *apiKeyStore) deleted(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deletedSet[id]
}

func (s *apiKeyStore) deletedIDs() map[int]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[int]bool{}
	for id := range s.deletedSet {
		out[id] = true
	}
	return out
}

func (s *apiKeyStore) createBody() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.created
}

func (s *apiKeyStore) updateBody() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updated
}

func (s *apiKeyStore) liveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.keys)
}

func (s *apiKeyStore) handler(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}
	if r.URL.Path == "/version.json" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/user_api_keys":
		s.list(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/user_api_keys":
		s.create(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/user_api_keys/"):
		s.get(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/user_api_keys/"):
		s.update(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/user_api_keys/"):
		s.delete(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *apiKeyStore) list(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filter := r.URL.Query().Get("filter")
	var rows []map[string]any
	for _, key := range s.keys {
		if filter != "" && !strings.Contains(filter, key.name) {
			continue
		}
		rows = append(rows, map[string]any{"$key": key.id, "user": key.userID, "name": key.name})
	}
	_ = json.NewEncoder(w).Encode(rows)
}

func (s *apiKeyStore) create(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	s.mu.Lock()
	id := s.next
	s.next++
	name, _ := payload["name"].(string)
	user, _ := payload["user"].(float64)
	s.keys[id] = apiKeyRecord{id: id, userID: int(user), name: name}
	s.created = payload
	omit := s.omitToken
	s.mu.Unlock()
	response := map[string]any{}
	if !omit {
		response["token"] = "secret-token"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"$key": id, "response": response})
}

func (s *apiKeyStore) get(w http.ResponseWriter, r *http.Request) {
	id := keyID(r.URL.Path)
	s.mu.Lock()
	key, ok := s.keys[id]
	s.mu.Unlock()
	if !ok {
		http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"$key": key.id, "user": key.userID, "name": key.name})
}

func (s *apiKeyStore) update(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	s.mu.Lock()
	s.updated = payload
	_, ok := s.keys[keyID(r.URL.Path)]
	s.mu.Unlock()
	if !ok {
		http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *apiKeyStore) delete(w http.ResponseWriter, r *http.Request) {
	id := keyID(r.URL.Path)
	s.mu.Lock()
	_, ok := s.keys[id]
	if ok {
		delete(s.keys, id)
		s.deletedSet[id] = true
	}
	s.mu.Unlock()
	if !ok {
		http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func keyID(path string) int {
	path = strings.TrimPrefix(path, "/api/v4/user_api_keys/")
	var id int
	for _, c := range path {
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + int(c-'0')
	}
	return id
}

func apiKeySDK(t *testing.T, store *apiKeyStore) *vergeos.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(store.handler))
	t.Cleanup(server.Close)
	sdk, err := vergeio.NewClient(server.URL, "user", "pass", true).NewVergeosClient()
	if err != nil {
		t.Fatal(err)
	}
	return sdk
}
