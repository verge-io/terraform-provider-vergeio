package compute

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestReadCloudinitFilesLoadsContentsFromDownload(t *testing.T) {
	const (
		probeBody = "#cloud-config\nhostname: zzrc\n"
		otherBody = "instance-id: other\n"
	)
	var downloads []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/cloudinit_files":
			if r.URL.Query().Get("download") != "" {
				t.Errorf("list sent download=%q", r.URL.Query().Get("download"))
			}
			_, _ = w.Write([]byte(`[
				{"$key":99,"name":"/zzrc-ci-probe","filesize":29,"contents":"from-list","contains_variables":false},
				{"$key":100,"name":"meta-data","filesize":19,"contents":"also-from-list","contains_variables":true}
			]`))
		case "/api/v4/cloudinit_files/99", "/api/v4/cloudinit_files/100":
			if r.Method != http.MethodGet || r.URL.Query().Get("download") != "1" {
				t.Errorf("contents request %s %s", r.Method, r.URL.RequestURI())
			}
			downloads = append(downloads, r.URL.Path)
			body := probeBody
			if strings.HasSuffix(r.URL.Path, "/100") {
				body = otherBody
			}
			_, _ = w.Write([]byte(body))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewCloudinitFileApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	data := &CloudinitFileDataSourceModel{}
	if err := api.readCloudinitFiles(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if len(downloads) != 2 || downloads[0] != "/api/v4/cloudinit_files/99" || downloads[1] != "/api/v4/cloudinit_files/100" {
		t.Fatalf("downloads = %v, want both files via download=1", downloads)
	}
	if len(data.CloudinitFiles) != 2 {
		t.Fatalf("got %d files, want 2", len(data.CloudinitFiles))
	}
	probe := data.CloudinitFiles[0]
	if probe.Id.ValueString() != "99" || probe.Name.ValueString() != "/zzrc-ci-probe" || probe.Filesize.ValueInt64() != 29 {
		t.Fatalf("probe metadata = id %s name %s size %d", probe.Id.ValueString(), probe.Name.ValueString(), probe.Filesize.ValueInt64())
	}
	if probe.Contents.ValueString() != probeBody || probe.ContainsVariables.ValueBool() {
		t.Fatalf("probe contents = %q contains_variables = %v", probe.Contents.ValueString(), probe.ContainsVariables.ValueBool())
	}
	other := data.CloudinitFiles[1]
	if other.Contents.ValueString() != otherBody || !other.ContainsVariables.ValueBool() {
		t.Fatalf("other contents = %q contains_variables = %v", other.Contents.ValueString(), other.ContainsVariables.ValueBool())
	}
}

func TestReadCloudinitFilesDownloadsOnlyKeptNames(t *testing.T) {
	var downloads []string
	server := cloudinitContentsServer(t, &downloads, map[int]string{
		99:  "probe-body",
		100: "other-body",
	}, `[
		{"$key":99,"name":"/zzrc-ci-probe","filesize":35,"contents":"from-list"},
		{"$key":100,"name":"meta-data","filesize":10,"contents":"also-from-list"}
	]`)
	t.Cleanup(server.Close)

	api := mustAPI(NewCloudinitFileApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	data := &CloudinitFileDataSourceModel{FilterName: types.StringValue("/zzrc-ci-probe")}
	if err := api.readCloudinitFiles(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if len(data.CloudinitFiles) != 1 || data.CloudinitFiles[0].Name.ValueString() != "/zzrc-ci-probe" {
		t.Fatalf("got %#v, want the probe file", data.CloudinitFiles)
	}
	if data.CloudinitFiles[0].Contents.ValueString() != "probe-body" {
		t.Fatalf("contents = %q, want the download body", data.CloudinitFiles[0].Contents.ValueString())
	}
	if len(downloads) != 1 || downloads[0] != "/api/v4/cloudinit_files/99" {
		t.Fatalf("downloads = %v, want only the kept file", downloads)
	}
}

func TestReadCloudinitFilesSkipsDownloadWhenNothingMatches(t *testing.T) {
	var downloads []string
	server := cloudinitContentsServer(t, &downloads, map[int]string{99: "probe-body"}, `[
		{"$key":99,"name":"/zzrc-ci-probe","filesize":35,"contents":"from-list"}
	]`)
	t.Cleanup(server.Close)

	api := mustAPI(NewCloudinitFileApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	data := &CloudinitFileDataSourceModel{FilterName: types.StringValue("missing")}
	if err := api.readCloudinitFiles(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if len(data.CloudinitFiles) != 0 {
		t.Fatalf("got %d files, want none", len(data.CloudinitFiles))
	}
	if len(downloads) != 0 {
		t.Fatalf("downloads = %v, want none", downloads)
	}
}

func TestReadCloudinitFilesDownloadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/cloudinit_files":
			_, _ = w.Write([]byte(`[{"$key":99,"name":"/zzrc-ci-probe","filesize":35,"contents":"from-list"}]`))
		case "/api/v4/cloudinit_files/99":
			if r.URL.Query().Get("download") != "1" {
				t.Errorf("download=%q, want 1", r.URL.Query().Get("download"))
			}
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"err":"permission denied"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewCloudinitFileApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	data := &CloudinitFileDataSourceModel{}
	err := api.readCloudinitFiles(t.Context(), data)
	if err == nil {
		t.Fatal("expected download error")
	}
	if len(data.CloudinitFiles) != 0 {
		t.Fatalf("stored %#v after a failed download", data.CloudinitFiles)
	}
	if !strings.Contains(err.Error(), "/zzrc-ci-probe") {
		t.Fatalf("error = %v, want the file name", err)
	}
}

func cloudinitContentsServer(t *testing.T, downloads *[]string, bodies map[int]string, listJSON string) *httptest.Server {
	t.Helper()
	if err := json.Unmarshal([]byte(listJSON), &[]map[string]any{}); err != nil {
		t.Fatalf("list json: %v", err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.URL.Path == "/api/v4/cloudinit_files":
			if r.URL.Query().Get("download") != "" {
				t.Errorf("list sent download=%q", r.URL.Query().Get("download"))
			}
			_, _ = w.Write([]byte(listJSON))
		case strings.HasPrefix(r.URL.Path, "/api/v4/cloudinit_files/"):
			if r.URL.Query().Get("download") != "1" {
				t.Errorf("download=%q, want 1", r.URL.Query().Get("download"))
			}
			id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/v4/cloudinit_files/"))
			if err != nil {
				t.Errorf("file id: %v", err)
				http.Error(w, "bad id", http.StatusBadRequest)
				return
			}
			body, ok := bodies[id]
			if !ok {
				t.Errorf("unexpected download %s", r.URL.Path)
				http.Error(w, "unexpected", http.StatusNotFound)
				return
			}
			*downloads = append(*downloads, r.URL.Path)
			_, _ = w.Write([]byte(body))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
}
