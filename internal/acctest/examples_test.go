package acctest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	eschema "github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	pschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/zclconf/go-cty/cty"

	tfprovider "terraform-provider-vergeio/internal/provider"
)

// TestExamplesCoverRegisteredObjects fails when a registered resource or data
// source has no example. Acceptance tests create objects with acctest.Name so
// the sweeper can find them; these files are the published examples.
func TestExamplesCoverRegisteredObjects(t *testing.T) {
	root := moduleRoot(t)
	ctx := context.Background()
	p := tfprovider.New("test")()

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
	withEphemeral, ok := p.(provider.ProviderWithEphemeralResources)
	if !ok {
		t.Fatal("provider does not register ephemeral resources")
	}
	for _, factory := range withEphemeral.EphemeralResources(ctx) {
		e := factory()
		resp := &ephemeral.MetadataResponse{}
		e.Metadata(ctx, ephemeral.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
		requireFile(t, filepath.Join(root, "examples", "ephemeral-resources", resp.TypeName, "ephemeral-resource.tf"))
	}
}

// TestExamplesMatchSchema parses every example configuration and rejects
// arguments the provider schema does not define. The registry pages render
// these files, so a name such as dhcp_end fails here instead of in a user's
// terraform validate.
func TestExamplesMatchSchema(t *testing.T) {
	root := moduleRoot(t)
	schemas := providerExampleSchemas()
	var files []string
	err := filepath.WalkDir(filepath.Join(root, "examples"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".tf") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no example files found")
	}
	sort.Strings(files)
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		for _, msg := range exampleSchemaErrors(rel, src, schemas) {
			t.Errorf("%s: %s", rel, msg)
		}
	}
}

func TestExampleSchemaRejectsUnknownNetworkArguments(t *testing.T) {
	src := []byte(`
resource "vergeio_network" "web_network" {
  name            = "web-internal-network"
  network_address = "192.168.10.0/24"
  dns_server_list = ["8.8.8.8", "8.8.4.4"]
  dhcp_enabled    = true
  dhcp_start      = "192.168.10.100"
  dhcp_end        = "192.168.10.200"
}
`)
	errs := exampleSchemaErrors("network.tf", src, providerExampleSchemas())
	joined := strings.Join(errs, "\n")
	for _, name := range []string{"network_address", "dns_server_list", "dhcp_end"} {
		if !strings.Contains(joined, name) {
			t.Errorf("missing unsupported-argument error for %s\n%s", name, joined)
		}
	}
}

func TestExampleSchemaRejectsExtraBrace(t *testing.T) {
	src := []byte(`
resource "vergeio_network" "example" {
  name     = "example-internal-network"
  network  = "192.168.1.0/24"
  dhcp_end = "192.168.1.200"
  }
}
`)
	errs := exampleSchemaErrors("network.tf", src, providerExampleSchemas())
	if len(errs) == 0 || !strings.Contains(strings.Join(errs, "\n"), "parse") {
		t.Fatalf("extra closing brace was accepted: %v", errs)
	}
}

func TestExampleSchemaRejectsUnregisteredCloudinitType(t *testing.T) {
	src := []byte(`
data "vergeio_cloudinitfiles" "all" {
}
`)
	errs := exampleSchemaErrors("cloudinit.tf", src, providerExampleSchemas())
	if len(errs) == 0 || !strings.Contains(strings.Join(errs, "\n"), "vergeio_cloudinitfiles") {
		t.Fatalf("unregistered data source was accepted: %v", errs)
	}
}

// TestDocTemplatesUseExampleFiles keeps resource and data source pages from
// pasting a second copy of HCL that can drift from examples/.
func TestDocTemplatesUseExampleFiles(t *testing.T) {
	root := moduleRoot(t)
	templates := filepath.Join(root, "templates")
	err := filepath.WalkDir(templates, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".tmpl") {
			return nil
		}
		rel, err := filepath.Rel(templates, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, "guides"+string(filepath.Separator)) {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(src)
		if strings.Contains(text, "vergeio_cloudinitfiles") {
			t.Errorf("%s uses vergeio_cloudinitfiles; the data source is vergeio_cloudinit_files", rel)
		}
		if strings.Contains(text, "storage resources") {
			t.Errorf("%s describes vergeio_user as storage", rel)
		}
		if rel == "index.md.tmpl" {
			if strings.Contains(text, "0.1.0") || strings.Contains(text, "2.X.X") {
				t.Errorf("%s pins a provider version that does not resolve", rel)
			}
			return nil
		}
		if strings.Contains(text, `resource "vergeio_`) || strings.Contains(text, `data "vergeio_`) {
			t.Errorf("%s embeds a vergeio configuration; render it with tffile from examples/", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPublishedNetworkExampleUsesSchemaNames(t *testing.T) {
	root := moduleRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "examples", "resources", "vergeio_network", "resource.tf"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	for _, name := range []string{"network_address", "dns_server_list", "dhcp_end"} {
		if strings.Contains(text, name) {
			t.Errorf("network example still uses %s", name)
		}
	}
	for _, name := range []string{"network ", "dhcp_stop", "dnslist", "description", "domain", "rate_limit"} {
		if !strings.Contains(text, name) {
			t.Errorf("network example is missing %s", strings.TrimSpace(name))
		}
	}
}

type schemaNode struct {
	children map[string]*schemaNode
}

type exampleSchemas struct {
	provider   *schemaNode
	resources  map[string]*schemaNode
	dataSource map[string]*schemaNode
	ephemeral  map[string]*schemaNode
}

func providerExampleSchemas() exampleSchemas {
	ctx := context.Background()
	p := tfprovider.New("test")()
	out := exampleSchemas{
		resources:  map[string]*schemaNode{},
		dataSource: map[string]*schemaNode{},
		ephemeral:  map[string]*schemaNode{},
	}

	presp := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, presp)
	out.provider = providerSchemaNode(presp.Schema)

	for _, factory := range p.Resources(ctx) {
		r := factory()
		meta := &resource.MetadataResponse{}
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
		sresp := &resource.SchemaResponse{}
		r.Schema(ctx, resource.SchemaRequest{}, sresp)
		out.resources[meta.TypeName] = resourceSchemaNode(sresp.Schema)
	}
	for _, factory := range p.DataSources(ctx) {
		d := factory()
		meta := &datasource.MetadataResponse{}
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
		sresp := &datasource.SchemaResponse{}
		d.Schema(ctx, datasource.SchemaRequest{}, sresp)
		out.dataSource[meta.TypeName] = datasourceSchemaNode(sresp.Schema)
	}
	if withEphemeral, ok := p.(provider.ProviderWithEphemeralResources); ok {
		for _, factory := range withEphemeral.EphemeralResources(ctx) {
			e := factory()
			meta := &ephemeral.MetadataResponse{}
			e.Metadata(ctx, ephemeral.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
			sresp := &ephemeral.SchemaResponse{}
			e.Schema(ctx, ephemeral.SchemaRequest{}, sresp)
			out.ephemeral[meta.TypeName] = ephemeralSchemaNode(sresp.Schema)
		}
	}
	return out
}

func ephemeralSchemaNode(s eschema.Schema) *schemaNode {
	children := map[string]*schemaNode{}
	for name := range s.Attributes {
		children[name] = &schemaNode{}
	}
	return &schemaNode{children: children}
}

func exampleSchemaErrors(filename string, src []byte, schemas exampleSchemas) []string {
	file, diags := hclsyntax.ParseConfig(src, filename, hcl.InitialPos)
	if diags.HasErrors() {
		return []string{"parse: " + diags.Error()}
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return []string{"parse: configuration body is not native HCL"}
	}
	var errs []string
	for _, block := range body.Blocks {
		switch block.Type {
		case "resource":
			if len(block.Labels) < 1 {
				continue
			}
			errs = append(errs, checkLabeledBlock(block, schemas.resources, "resource")...)
		case "data":
			if len(block.Labels) < 1 {
				continue
			}
			errs = append(errs, checkLabeledBlock(block, schemas.dataSource, "data source")...)
		case "provider":
			if len(block.Labels) < 1 || block.Labels[0] != "vergeio" {
				continue
			}
			errs = append(errs, checkBody(block.Labels[0], block.Body, schemas.provider)...)
		case "ephemeral":
			if len(block.Labels) < 1 {
				continue
			}
			errs = append(errs, checkLabeledBlock(block, schemas.ephemeral, "ephemeral resource")...)
		}
	}
	return errs
}

func checkLabeledBlock(block *hclsyntax.Block, schemas map[string]*schemaNode, kind string) []string {
	typeName := block.Labels[0]
	if !strings.HasPrefix(typeName, "vergeio_") {
		return nil
	}
	node, ok := schemas[typeName]
	if !ok {
		return []string{fmt.Sprintf("unsupported %s %q", kind, typeName)}
	}
	label := typeName
	if len(block.Labels) > 1 {
		label = typeName + "." + block.Labels[1]
	}
	return checkBody(label, block.Body, node)
}

func checkBody(path string, body *hclsyntax.Body, node *schemaNode) []string {
	if node == nil {
		node = &schemaNode{}
	}
	var errs []string
	names := make([]string, 0, len(body.Attributes))
	for name := range body.Attributes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if resourceMetaArgument(name) || (name == "alias" && path == "vergeio") {
			continue
		}
		child, ok := node.children[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: unsupported argument %q", path, name))
			continue
		}
		errs = append(errs, checkExpr(path+"."+name, body.Attributes[name].Expr, child)...)
	}
	for _, nested := range body.Blocks {
		if resourceMetaBlock(nested.Type) {
			continue
		}
		child, ok := node.children[nested.Type]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: unsupported block %q", path, nested.Type))
			continue
		}
		errs = append(errs, checkBody(path+"."+nested.Type, nested.Body, child)...)
	}
	return errs
}

func checkExpr(path string, expr hcl.Expression, node *schemaNode) []string {
	if node == nil {
		node = &schemaNode{}
	}
	switch e := expr.(type) {
	case *hclsyntax.ObjectConsExpr:
		if len(node.children) == 0 {
			return []string{fmt.Sprintf("%s: attribute does not accept nested arguments", path)}
		}
		var errs []string
		for _, item := range e.Items {
			key, ok := exprKey(item.KeyExpr)
			if !ok {
				continue
			}
			child, ok := node.children[key]
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: unsupported argument %q", path, key))
				continue
			}
			errs = append(errs, checkExpr(path+"."+key, item.ValueExpr, child)...)
		}
		return errs
	case *hclsyntax.TupleConsExpr:
		var errs []string
		for i, el := range e.Exprs {
			errs = append(errs, checkExpr(fmt.Sprintf("%s[%d]", path, i), el, node)...)
		}
		return errs
	default:
		return nil
	}
}

func exprKey(expr hcl.Expression) (string, bool) {
	if key := hcl.ExprAsKeyword(expr); key != "" {
		return key, true
	}
	switch e := expr.(type) {
	case *hclsyntax.ObjectConsKeyExpr:
		return exprKey(e.Wrapped)
	case *hclsyntax.LiteralValueExpr:
		if e.Val.Type() == cty.String {
			return e.Val.AsString(), true
		}
	case *hclsyntax.TemplateExpr:
		if e.IsStringLiteral() {
			val, diags := e.Value(nil)
			if !diags.HasErrors() && val.Type() == cty.String {
				return val.AsString(), true
			}
		}
	}
	return "", false
}

func resourceMetaArgument(name string) bool {
	switch name {
	case "count", "for_each", "depends_on", "provider", "lifecycle":
		return true
	default:
		return false
	}
}

func resourceMetaBlock(name string) bool {
	switch name {
	case "lifecycle", "provisioner", "connection", "dynamic":
		return true
	default:
		return false
	}
}

func providerSchemaNode(schema pschema.Schema) *schemaNode {
	children := map[string]*schemaNode{}
	for name, attr := range schema.Attributes {
		children[name] = providerAttrNode(attr)
	}
	return &schemaNode{children: children}
}

func providerAttrNode(attr pschema.Attribute) *schemaNode {
	switch a := attr.(type) {
	case pschema.SingleNestedAttribute:
		children := map[string]*schemaNode{}
		for name, nested := range a.Attributes {
			children[name] = providerAttrNode(nested)
		}
		return &schemaNode{children: children}
	default:
		return &schemaNode{}
	}
}

func resourceSchemaNode(schema rschema.Schema) *schemaNode {
	return &schemaNode{children: resourceNodes(schema.Attributes, schema.Blocks)}
}

func resourceNodes(attrs map[string]rschema.Attribute, blocks map[string]rschema.Block) map[string]*schemaNode {
	children := map[string]*schemaNode{}
	for name, attr := range attrs {
		children[name] = resourceAttrNode(attr)
	}
	for name, block := range blocks {
		children[name] = resourceBlockNode(block)
	}
	return children
}

func resourceAttrNode(attr rschema.Attribute) *schemaNode {
	switch a := attr.(type) {
	case rschema.ListNestedAttribute:
		return &schemaNode{children: resourceNodes(a.NestedObject.Attributes, nil)}
	case rschema.SetNestedAttribute:
		return &schemaNode{children: resourceNodes(a.NestedObject.Attributes, nil)}
	case rschema.MapNestedAttribute:
		return &schemaNode{children: resourceNodes(a.NestedObject.Attributes, nil)}
	case rschema.SingleNestedAttribute:
		return &schemaNode{children: resourceNodes(a.Attributes, nil)}
	default:
		return &schemaNode{}
	}
}

func resourceBlockNode(block rschema.Block) *schemaNode {
	switch b := block.(type) {
	case rschema.ListNestedBlock:
		return &schemaNode{children: resourceNodes(b.NestedObject.Attributes, b.NestedObject.Blocks)}
	case rschema.SetNestedBlock:
		return &schemaNode{children: resourceNodes(b.NestedObject.Attributes, b.NestedObject.Blocks)}
	case rschema.SingleNestedBlock:
		return &schemaNode{children: resourceNodes(b.Attributes, b.Blocks)}
	default:
		return &schemaNode{}
	}
}

func datasourceSchemaNode(schema dschema.Schema) *schemaNode {
	children := map[string]*schemaNode{}
	for name, attr := range schema.Attributes {
		children[name] = datasourceAttrNode(attr)
	}
	for name, block := range schema.Blocks {
		children[name] = datasourceBlockNode(block)
	}
	return &schemaNode{children: children}
}

func datasourceAttrNode(attr dschema.Attribute) *schemaNode {
	switch a := attr.(type) {
	case dschema.ListNestedAttribute:
		return &schemaNode{children: datasourceAttrMap(a.NestedObject.Attributes)}
	case dschema.SetNestedAttribute:
		return &schemaNode{children: datasourceAttrMap(a.NestedObject.Attributes)}
	case dschema.MapNestedAttribute:
		return &schemaNode{children: datasourceAttrMap(a.NestedObject.Attributes)}
	case dschema.SingleNestedAttribute:
		return &schemaNode{children: datasourceAttrMap(a.Attributes)}
	default:
		return &schemaNode{}
	}
}

func datasourceAttrMap(attrs map[string]dschema.Attribute) map[string]*schemaNode {
	children := map[string]*schemaNode{}
	for name, attr := range attrs {
		children[name] = datasourceAttrNode(attr)
	}
	return children
}

func datasourceBlockNode(block dschema.Block) *schemaNode {
	switch b := block.(type) {
	case dschema.ListNestedBlock:
		children := datasourceAttrMap(b.NestedObject.Attributes)
		for name, nested := range b.NestedObject.Blocks {
			children[name] = datasourceBlockNode(nested)
		}
		return &schemaNode{children: children}
	case dschema.SetNestedBlock:
		children := datasourceAttrMap(b.NestedObject.Attributes)
		for name, nested := range b.NestedObject.Blocks {
			children[name] = datasourceBlockNode(nested)
		}
		return &schemaNode{children: children}
	case dschema.SingleNestedBlock:
		children := datasourceAttrMap(b.Attributes)
		for name, nested := range b.Blocks {
			children[name] = datasourceBlockNode(nested)
		}
		return &schemaNode{children: children}
	default:
		return &schemaNode{}
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
