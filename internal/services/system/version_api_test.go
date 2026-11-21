package system

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestReadVersionMapsVersionJSON(t *testing.T) {
	const (
		name    = "v4"
		version = "26.2.2.1"
		hash    = "0123456789abcdef0123456789abcdef01234567"
	)
	body := fmt.Sprintf(`{"name":%q,"version":%q,"hash":%q,"ignored":"extra"}`, name, version, hash)
	server := versionJSONServer(t, func(int) (int, string) {
		return http.StatusOK, body
	})

	data := &VersionDataSourceModel{}
	if err := newTestVersionAPI(t, server.URL).readVersion(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Name.ValueString() != name {
		t.Errorf("name = %q", data.Name.ValueString())
	}
	if data.Version.ValueString() != version {
		t.Errorf("version = %q", data.Version.ValueString())
	}
	if data.Hash.ValueString() != hash {
		t.Errorf("hash = %q", data.Hash.ValueString())
	}
}

func TestReadVersionKeepsNameWhenItEqualsVersion(t *testing.T) {
	const (
		version = "26.2.2.1"
		hash    = "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	)
	body := fmt.Sprintf(`{"name":%q,"version":%q,"hash":%q}`, version, version, hash)
	server := versionJSONServer(t, func(int) (int, string) {
		return http.StatusOK, body
	})

	data := &VersionDataSourceModel{}
	if err := newTestVersionAPI(t, server.URL).readVersion(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Name.ValueString() != version || data.Version.ValueString() != version {
		t.Fatalf("name = %q version = %q", data.Name.ValueString(), data.Version.ValueString())
	}
	if data.Hash.ValueString() != hash {
		t.Errorf("hash = %q", data.Hash.ValueString())
	}
}

func TestReadVersionReturnsDecodeError(t *testing.T) {
	server := versionJSONServer(t, func(call int) (int, string) {
		if call == 1 {
			return http.StatusOK, `{"name":"v4","version":"26.2.2.1","hash":"0123456789abcdef0123456789abcdef01234567"}`
		}
		return http.StatusOK, `not-json`
	})

	data := &VersionDataSourceModel{}
	err := newTestVersionAPI(t, server.URL).readVersion(context.Background(), data)
	if err == nil {
		t.Fatal("expected an error decoding /version.json")
	}
}

func TestReadVersionRejectsEmptyInfo(t *testing.T) {
	server := versionJSONServer(t, func(int) (int, string) {
		return http.StatusOK, `{"version":"26.2.2.1"}`
	})
	api := newTestVersionAPI(t, server.URL)
	api.sdk.System = emptySystem{}

	err := api.readVersion(context.Background(), &VersionDataSourceModel{})
	if err == nil {
		t.Fatal("expected an error when version information is missing")
	}
}

type emptySystem struct{}

func (emptySystem) GetInfo(context.Context) (*vergeos.SystemInfo, error) {
	return nil, nil
}

func (emptySystem) GetVersion(context.Context) (string, error) {
	return "", nil
}

func newTestVersionAPI(t *testing.T, host string) *VersionApi {
	t.Helper()
	api, err := NewVersionApi(vergeio.NewClient(host, "testuser", "testpass", true))
	if err != nil {
		t.Fatalf("NewVersionApi: %v", err)
	}
	return api
}

func versionJSONServer(t *testing.T, respond func(call int) (int, string)) *httptest.Server {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/version.json" {
			http.NotFound(w, r)
			return
		}
		status, body := respond(int(calls.Add(1)))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write /version.json: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
