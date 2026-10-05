package vm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-vergeio/internal/provider/vergeio"
)

func TestChangeVMPowerState_NeverRunningReturnsError(t *testing.T) {
	prevInterval, prevRetries := powerStatePollInterval, powerStateMaxRetries
	powerStatePollInterval = time.Millisecond
	powerStateMaxRetries = 2
	t.Cleanup(func() {
		powerStatePollInterval = prevInterval
		powerStateMaxRetries = prevRetries
	})

	var posts, gets int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/vm_actions"):
			atomic.AddInt32(&posts, 1)
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"$key":1}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/vms/"):
			atomic.AddInt32(&gets, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"powerstate":false}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "https://")
	client := vergeio.NewClient(host, "u", "p", true)
	va := NewVMApi(client)

	data := &VMResourceModel{Id: types.StringValue("42")}
	err := va.changeVMPowerState(context.Background(), data, "poweron")
	if err == nil {
		t.Fatalf("expected timeout error when VM never runs; posts=%d gets=%d", atomic.LoadInt32(&posts), atomic.LoadInt32(&gets))
	}
	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "power state") {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&posts) < 1 {
		t.Fatalf("expected POST /vm_actions, posts=%d", atomic.LoadInt32(&posts))
	}
}
