package compute

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestDiskInterfacesIncludeVergeOSValues(t *testing.T) {
	got := diskInterfaces()
	want := []string{
		"virtio",
		"ide",
		"ahci",
		"lsi53c895a",
		"megasas",
		"megasas-gen2",
		"mptsas1068",
		"virtio-scsi",
		"virtio-scsi-dedicated",
		"nvme",
		"usb",
		"cifs",
		"nfs",
		"vsan",
		"pflash",
		"direct",
		"tpm_state",
	}
	if len(got) != len(want) {
		t.Fatalf("disk interfaces = %v, want %v", got, want)
	}
	seen := make(map[string]bool, len(got))
	for i, iface := range got {
		if iface != want[i] {
			t.Errorf("disk interface %d = %q, want %q", i, iface, want[i])
		}
		if seen[iface] {
			t.Errorf("duplicate disk interface %q", iface)
		}
		seen[iface] = true
	}
}

func TestDriveInterfaceSchemaAllowsDocumentedValues(t *testing.T) {
	vmResource := NewVMResource()
	resp := &resource.SchemaResponse{}
	vmResource.Schema(context.Background(), resource.SchemaRequest{}, resp)

	attr := nestedStringAttr(t, resp.Schema.Blocks, "vergeio_drive", "interface")
	if len(attr.Validators) == 0 {
		t.Fatal("vergeio_drive.interface should validate against the documented interface list")
	}

	ctx := context.Background()
	var described strings.Builder
	for _, check := range attr.Validators {
		described.WriteString(check.Description(ctx))
		described.WriteByte('\n')
		described.WriteString(check.MarkdownDescription(ctx))
	}
	text := described.String()
	for _, iface := range diskInterfaces() {
		if !strings.Contains(text, iface) {
			t.Errorf("interface validator description missing %q: %s", iface, text)
		}
		if diags := validateDriveInterface(ctx, attr.Validators, iface); diags != "" {
			t.Errorf("interface %q rejected: %s", iface, diags)
		}
	}

	if diags := validateDriveInterface(ctx, attr.Validators, "sata"); diags == "" {
		t.Fatal("interface sata should be rejected")
	} else if !strings.Contains(diags, "usb") || !strings.Contains(diags, "tpm_state") {
		t.Fatalf("rejection should list the expanded allow-list, got %s", diags)
	}

	for _, unset := range []types.String{types.StringNull(), types.StringUnknown()} {
		for _, check := range attr.Validators {
			result := &validator.StringResponse{}
			check.ValidateString(ctx, validator.StringRequest{ConfigValue: unset}, result)
			if result.Diagnostics.HasError() {
				t.Fatalf("unset interface rejected: %s", result.Diagnostics)
			}
		}
	}
}

func TestValidateDiskInterfaceUsesAPIList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/version.json" {
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/v4/machine_drives/$table" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
			return
		}
		list := make(map[string]string, len(diskInterfaces()))
		for _, iface := range diskInterfaces() {
			list[iface] = iface
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"fields": map[string]any{
				"interface": map[string]any{"type": "string", "list": list},
			},
		}); err != nil {
			t.Errorf("encode schema: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	vmResource := &VMResource{diskApi: NewDiskApi(vergeio.NewClient(server.URL, "user", "pass", true))}
	ctx := context.Background()
	for _, iface := range diskInterfaces() {
		if err := vmResource.validateDiskInterface(ctx, types.StringValue(iface)); err != nil {
			t.Errorf("interface %q rejected by API list: %v", iface, err)
		}
	}
	if err := vmResource.validateDiskInterface(ctx, types.StringNull()); err != nil {
		t.Fatalf("null interface: %v", err)
	}
	if err := vmResource.validateDiskInterface(ctx, types.StringValue("sata")); err == nil {
		t.Fatal("sata should be rejected when the API list omits it")
	}
}

func TestValidateDiskInterfaceRejectsValueMissingFromAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/version.json" {
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
			return
		}
		_, _ = w.Write([]byte(`{"fields":{"interface":{"type":"string","list":{"virtio":"Virtio","virtio-scsi":"Virtio-SCSI"}}}}`))
	}))
	t.Cleanup(server.Close)

	vmResource := &VMResource{diskApi: NewDiskApi(vergeio.NewClient(server.URL, "user", "pass", true))}
	err := vmResource.validateDiskInterface(context.Background(), types.StringValue("usb"))
	if err == nil {
		t.Fatal("usb should be rejected when this VergeOS system does not list it")
	}
	if !strings.Contains(err.Error(), "usb") {
		t.Fatalf("error %v should name usb", err)
	}
}

func TestDriveInterfaceDocsListAllowList(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	for _, rel := range []string{
		filepath.Join("templates", "resources", "vm.md.tmpl"),
		filepath.Join("docs", "resources", "vm.md"),
	} {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		section := driveDocSection(t, rel, string(body))
		for _, iface := range diskInterfaces() {
			if !strings.Contains(section, "`"+iface+"`") {
				t.Errorf("%s drive docs missing `%s`", rel, iface)
			}
		}
	}
}

func driveDocSection(t *testing.T, name, body string) string {
	t.Helper()
	const start = "### Nested Schema for `vergeio_drive`"
	const end = "### Nested Schema for `vergeio_nic`"
	from := strings.Index(body, start)
	to := strings.Index(body, end)
	if from < 0 || to < 0 || to <= from {
		t.Fatalf("%s is missing the drive nested schema", name)
	}
	return body[from:to]
}

func validateDriveInterface(ctx context.Context, validators []validator.String, value string) string {
	var diags strings.Builder
	for _, check := range validators {
		result := &validator.StringResponse{}
		check.ValidateString(ctx, validator.StringRequest{ConfigValue: types.StringValue(value)}, result)
		for _, diag := range result.Diagnostics.Errors() {
			if diags.Len() > 0 {
				diags.WriteByte('\n')
			}
			diags.WriteString(diag.Detail())
			diags.WriteByte(' ')
			diags.WriteString(diag.Summary())
		}
	}
	return diags.String()
}
