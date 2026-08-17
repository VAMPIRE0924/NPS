package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	beecontext "github.com/astaxie/beego/context"
)

func TestUnauthenticatedProtectedPostRequiresAPIAuth(t *testing.T) {
	post := httptest.NewRequest(http.MethodPost, "/client/list/", nil)
	if !unauthenticatedRequestRequiresAPIAuth(post) {
		t.Fatal("expected an unauthenticated protected POST to require API authentication")
	}
	get := httptest.NewRequest(http.MethodGet, "/client/list/", nil)
	if unauthenticatedRequestRequiresAPIAuth(get) {
		t.Fatal("expected an unauthenticated page GET to retain the login redirect flow")
	}
}

func TestClientScopeIgnoresCallerSuppliedClientID(t *testing.T) {
	if got := selectEffectiveClientID(false, 7, 99); got != 7 {
		t.Fatalf("client request escaped its session scope: %d", got)
	}
	if got := selectEffectiveClientID(true, 0, 99); got != 99 {
		t.Fatalf("admin request lost its explicit client scope: %d", got)
	}
}

func TestClientMutationModesDoNotAllocateDedicatedNPSPorts(t *testing.T) {
	for _, mode := range []string{"secret", "p2p"} {
		if !clientMayMutateTask("add", mode) {
			t.Fatalf("client should retain %s creation capability", mode)
		}
	}
	for _, mode := range []string{"portForward", "httpProxy", "file"} {
		if clientMayMutateTask("add", mode) || clientMayMutateTask("start", mode) {
			t.Fatalf("client must not allocate or activate dedicated NPS port mode %s", mode)
		}
	}
	if !clientMayMutateTask("start", "socks5") {
		t.Fatal("client must retain managed SOCKS start/stop capability")
	}
	if clientMayMutateTaskRequest("edit", "secret", "portForward") {
		t.Fatal("client must not convert a shared mode into a dedicated NPS port mode")
	}
	if !clientMayMutateTaskRequest("edit", "secret", "p2p") {
		t.Fatal("client should retain edits between permitted shared modes")
	}
}

func TestRejectForbiddenCommitsHTTPStatusAndJSONBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := beecontext.NewContext()
	ctx.Reset(recorder, httptest.NewRequest(http.MethodGet, "/client/edit?id=1", nil))
	controller := new(BaseController)
	controller.Init(ctx, "BaseController", "Test", controller)

	var recovered interface{}
	func() {
		defer func() { recovered = recover() }()
		controller.rejectForbidden()
	}()
	if recovered == nil {
		t.Fatal("rejectForbidden must stop controller execution")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("forbidden response status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("forbidden response Content-Type = %q", got)
	}
	var response map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("forbidden response is not JSON: %v", err)
	}
	if response["status"] != float64(0) || response["msg"] != "permission denied" {
		t.Fatalf("unexpected forbidden response: %#v", response)
	}
}
