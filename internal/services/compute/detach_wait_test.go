package compute

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func shortenDetachWait(t *testing.T, timeout, interval time.Duration) {
	t.Helper()
	origTimeout, origInterval := deviceDetachTimeout, deviceDetachInterval
	t.Cleanup(func() {
		deviceDetachTimeout = origTimeout
		deviceDetachInterval = origInterval
	})
	deviceDetachTimeout = timeout
	deviceDetachInterval = interval
}

func TestWaitUntilDetachedReportsLastStatus(t *testing.T) {
	shortenDetachWait(t, 0, time.Hour)

	reads := 0
	err := waitUntilDetached(t.Context(), func() (string, error) {
		reads++
		return "hotplug", nil
	}, "offline", "drive", "data", "46", "key")
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if reads != 1 {
		t.Fatalf("reads = %d, want one read before giving up", reads)
	}
	msg := err.Error()
	for _, want := range []string{"data", "46", "hotplug", "offline"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestWaitUntilDetachedPollsUntilOffline(t *testing.T) {
	shortenDetachWait(t, time.Second, 0)

	reads := 0
	err := waitUntilDetached(t.Context(), func() (string, error) {
		reads++
		if reads < 3 {
			return "hotplug", nil
		}
		return "offline", nil
	}, "offline", "drive", "data", "46", "key")
	if err != nil {
		t.Fatal(err)
	}
	if reads != 3 {
		t.Fatalf("reads = %d, want polls until offline", reads)
	}
}

func TestDeleteDiskUnplugsOnceThenWaits(t *testing.T) {
	shortenDetachWait(t, time.Second, 0)

	var unplugBodies []string
	statusReads := 0
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"version":"26.0.0"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/46":
			statusReads++
			payload := `{"powerstate":"online"}`
			if statusReads > 1 {
				payload = `{"powerstate":"hotplug"}`
			}
			if statusReads > 3 {
				payload = `{"powerstate":"offline"}`
			}
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(payload)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			unplugBodies = append(unplugBodies, string(body))
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/machine_drives/46":
			deleted = true
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	err := api.deleteDisk(t.Context(), &diskResourceModel{
		Key:  types.StringValue("46"),
		Name: types.StringValue("data"),
	}, types.StringValue("7"))
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("drive was not deleted after it went offline")
	}
	if statusReads < 4 {
		t.Fatalf("status reads = %d, want polls while the drive is still hotplug", statusReads)
	}
	if len(unplugBodies) != 1 {
		t.Fatalf("unplug calls = %d, want 1: %#v", len(unplugBodies), unplugBodies)
	}
	body := unplugBodies[0]
	if !strings.Contains(body, `"action":"hotplugdrive"`) || !strings.Contains(body, `"unplug":true`) || !strings.Contains(body, `"device":"46"`) {
		t.Fatalf("unplug body = %s, want one hotplugdrive with unplug true", body)
	}
}

func TestDeleteDiskTimeoutNamesDriveAndStatus(t *testing.T) {
	shortenDetachWait(t, 0, time.Hour)

	unplugs := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/46":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"powerstate":"hotplug"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			unplugs++
			t.Errorf("unplug was sent while the drive was already hotplug")
			http.Error(w, "unexpected unplug", http.StatusInternalServerError)
		case r.Method == http.MethodDelete:
			t.Errorf("drive was deleted before it went offline")
			http.Error(w, "unexpected delete", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	err := api.deleteDisk(t.Context(), &diskResourceModel{
		Key:  types.StringValue("46"),
		Name: types.StringValue("data"),
	}, types.StringValue("7"))
	if err == nil {
		t.Fatal("expected a timeout")
	}
	msg := err.Error()
	for _, want := range []string{"data", "46", "hotplug"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if unplugs != 0 {
		t.Fatalf("unplug calls = %d, want 0 while already hotplug", unplugs)
	}
}

func TestDeleteDiskWaitsWhenUnplugAlreadyInProgress(t *testing.T) {
	shortenDetachWait(t, time.Second, 0)

	unplugs := 0
	statusReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/46":
			statusReads++
			payload := `{"powerstate":"online"}`
			if statusReads > 1 {
				payload = `{"powerstate":"offline"}`
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(payload))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			unplugs++
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"err":"The specified drive is already hotplugging"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/machine_drives/46":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	if err := api.deleteDisk(t.Context(), &diskResourceModel{
		Key:  types.StringValue("46"),
		Name: types.StringValue("data"),
	}, types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	if unplugs != 1 {
		t.Fatalf("unplug calls = %d, want 1", unplugs)
	}
}

func TestDeleteNICUnplugsOnceThenWaits(t *testing.T) {
	shortenDetachWait(t, time.Second, 0)

	var unplugBodies []string
	statusReads := 0
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_nics/98":
			statusReads++
			payload := `{"powerstate":"up"}`
			if statusReads > 1 {
				payload = `{"powerstate":"hotplug"}`
			}
			if statusReads > 3 {
				payload = `{"powerstate":"down"}`
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(payload))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			unplugBodies = append(unplugBodies, string(body))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/machine_nics/98":
			deleted = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &NICApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	err := api.deleteNIC(t.Context(), &nicResourceModel{
		Id:   types.StringValue("98"),
		Name: types.StringValue("lan"),
	}, types.StringValue("7"))
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("NIC was not deleted after it went down")
	}
	if statusReads < 4 {
		t.Fatalf("status reads = %d, want polls while the NIC is still hotplug", statusReads)
	}
	if len(unplugBodies) != 1 {
		t.Fatalf("unplug calls = %d, want 1: %#v", len(unplugBodies), unplugBodies)
	}
	body := unplugBodies[0]
	if !strings.Contains(body, `"action":"hotplugnic"`) || !strings.Contains(body, `"unplug":true`) || !strings.Contains(body, `"device":"98"`) {
		t.Fatalf("unplug body = %s, want one hotplugnic with unplug true", body)
	}
}

func TestDeleteNICTimeoutNamesNICAndStatus(t *testing.T) {
	shortenDetachWait(t, 0, time.Hour)

	unplugs := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_nics/98":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"powerstate":"up"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			unplugs++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodDelete:
			t.Errorf("NIC was deleted before it went down")
			http.Error(w, "unexpected delete", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &NICApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	err := api.deleteNIC(t.Context(), &nicResourceModel{
		Id:   types.StringValue("98"),
		Name: types.StringValue("lan"),
	}, types.StringValue("7"))
	if err == nil {
		t.Fatal("expected a timeout")
	}
	msg := err.Error()
	for _, want := range []string{"lan", "98", "up", "down"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if unplugs != 1 {
		t.Fatalf("unplug calls = %d, want 1", unplugs)
	}
}
