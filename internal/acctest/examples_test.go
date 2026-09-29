package acctest

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"terraform-provider-vergeio/internal/provider"
)

// TestExamplesCoverRegisteredObjects fails when a registered resource or data
// source has no example. Acceptance tests create objects with acctest.Name so
// the sweeper can find them; these files are the published examples.
func TestExamplesCoverRegisteredObjects(t *testing.T) {
	root := moduleRoot(t)
	ctx := context.Background()
	p := provider.New("test")()

	for _, factory := range p.Resources(ctx) {
		r := factory()
		resp := &resource.MetadataResponse{}
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
		dir := filepath.Join(root, "examples", "resources", resp.TypeName)
		requireFile(t, filepath.Join(dir, "resource.tf"))
		requireFile(t, filepath.Join(dir, "import.sh"))
	}
	for _, factory := range p.DataSources(ctx) {
		d := factory()
		resp := &datasource.MetadataResponse{}
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
		requireFile(t, filepath.Join(root, "examples", "data-sources", resp.TypeName, "data-source.tf"))
	}
}

func requireFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Errorf("missing example %s: %v", path, err)
		return
	}
	if info.Size() == 0 {
		t.Errorf("example %s is empty", path)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate acctest source file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}
