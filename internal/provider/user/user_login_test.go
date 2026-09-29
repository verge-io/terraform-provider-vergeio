package user_test

import "testing"

func TestTokenLoginOutcomeAcceptsCreated(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		rejected bool
		token    string
		wantErr  bool
	}{
		{name: "201", status: 201, body: `{"$key":"tok-201"}`, token: "tok-201"},
		{name: "200", status: 200, body: `{"$key":"tok-200"}`, token: "tok-200"},
		{name: "299 numeric key", status: 299, body: `{"$key":42}`, token: "42"},
		{name: "204 empty", status: 204, body: ``, wantErr: true},
		{name: "401", status: 401, body: `{"error":"unauthorized"}`, rejected: true},
		{name: "403", status: 403, body: `forbidden`, rejected: true},
		{name: "500", status: 500, body: `error`, rejected: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rejected, token, err := tokenLoginOutcome(tc.status, []byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("status %d: got token %q, want error", tc.status, token)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if rejected != tc.rejected || token != tc.token {
				t.Fatalf("status %d: rejected=%v token=%q, want rejected=%v token=%q", tc.status, rejected, token, tc.rejected, tc.token)
			}
		})
	}
}
