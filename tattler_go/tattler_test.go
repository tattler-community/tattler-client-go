package tattler_go

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Common API base to use in tests
const api_base_test string = "http://127.0.0.1:11503/"

func TestInvalidConfiguration(t *testing.T) {
	params := make(map[string]string)
	vectors := []string{"email"}

	n := TattlerClientHTTP{
		Endpoint: "  ",
		Scope:    "testScope",
		Mode:     "debug",
	}
	err := n.SendNotification("456", "my_important_event", params, vectors, "corrid123")
	if err == nil {
		t.Fatalf("SendNotification unexpectedly accepted invalid Endpoint config")
	}

	n = TattlerClientHTTP{
		Endpoint: "http://localhost:11503",
		Scope:    " ",
		Mode:     "debug",
	}
	err = n.SendNotification("456", "my_important_event", params, vectors, "corrid123")
	if err == nil {
		t.Fatalf("SendNotification unexpectedly accepted invalid Scope config")
	}

	n = TattlerClientHTTP{
		Endpoint: "http://localhost:11503",
		Scope:    "validscope",
		Mode:     " ",
	}
	err = n.SendNotification("456", "my_important_event", params, vectors, "corrid123")
	if err == nil {
		t.Fatalf("SendNotification unexpectedly accepted invalid Mode config (empty)")
	}

	n = TattlerClientHTTP{
		Endpoint: "http://localhost:11503",
		Scope:    "validscope",
		Mode:     "unknown_mode",
	}
	err = n.SendNotification("456", "my_important_event", params, vectors, "corrid123")
	if err == nil {
		t.Fatalf("SendNotification unexpectedly accepted invalid Mode config (unknown)")
	}
}

func TestPrepareNotificationBasic(t *testing.T) {
	n := TattlerClientHTTP{
		Endpoint: api_base_test,
		Scope:    "testScope",
		Mode:     "debug",
	}

	params := make(map[string]string)
	params["foo"] = "bar"

	vectors := []string{"eMAIl"}
	url_have, body_have, taskname, err := n.PrepareNotification("456", "my_important_event", params, vectors, "corrid123")
	if err != nil {
		t.Fatalf("SendNotification unexpectedly returned error %v", err)
	}
	if taskname != "" {
		t.Fatalf("PrepareNotification() expected empty taskname when PersistencyDir is empty, but returned '%v'", taskname)
	}
	if !strings.HasPrefix(url_have, n.Endpoint) {
		t.Fatalf("PrepareNotification() expected URL to start with '%v', got '%v'", n.Endpoint, url_have)
	}
	mode_want := fmt.Sprintf("mode=%v", n.Mode)
	if !strings.Contains(url_have, mode_want) {
		t.Fatalf("PrepareNotification() expected URL to contain '%v', got '%v'", mode_want, url_have)
	}
	vectors_want := fmt.Sprintf("vectors=%v", strings.ToLower(strings.Join(vectors, ",")))
	if !strings.Contains(url_have, vectors_want) {
		t.Fatalf("PrepareNotification() expected to request vectors with '%v', instead '%v'", vectors_want, url_have)
	}
	body_want, _ := json.Marshal(params)
	if len(body_have) != len(body_want) {
		t.Fatalf("PrepareNotification() expected Body=%v bytes, got %v: '%v'", len(body_want), len(body_have), body_have)
	}
}

func TestCorrelationIdPassedOrGenerated(t *testing.T) {
	n := TattlerClientHTTP{
		Endpoint: api_base_test + "/",
		Scope:    "testScope",
	}

	corrid := "corrid987456"
	url_have, _, _, err := n.PrepareNotification("789", "other_event", map[string]string{}, []string{}, corrid)
	if err != nil {
		t.Fatalf("PrepareNotification() unexpectedly failed with valid input; error was '%v'", err)
	}
	corrid_want := fmt.Sprintf("correlationId=%v", corrid)
	if !strings.Contains(url_have, corrid_want) {
		t.Fatalf("PrepareNotification() expected to include '%v', was '%v' instead", corrid_want, url_have)
	}

	// defaults to random
	url_have2, _, _, err2 := n.PrepareNotification("789", "other_event", map[string]string{}, []string{}, "")
	if err2 != nil {
		t.Fatalf("PrepareNotification() unexpectedly failed with valid input; error was '%v'", err2)
	}
	expre := regexp.MustCompile(`\bcorrelationId=[a-fA-F0-9]+`)
	if !expre.MatchString(url_have2) {
		t.Fatalf("PrepareNotification() expected to auto-generate correlationId when empty one provided, but URL was '%v'", url_have2)
	}
}

func TestPrepareNotificationDefaults(t *testing.T) {
	n := TattlerClientHTTP{
		Endpoint: api_base_test + "/",
		Scope:    "testScope",
	}

	if n.ValidateConfiguration() != nil {
		t.Fatalf("ValidateConfiguration() fails on valid TattlerClientHTTP object = %v", n)
	}

	if n.Mode != DefaultMode {
		t.Fatalf("ValidateConfiguration() sets 'Mode'=%v different from DefaultMode=%v", n.Mode, DefaultMode)
	}

	if n.Timeout != DefaultTimeout {
		t.Fatalf("ValidateConfiguration() sets 'Timeout'=%v different from DefaultTimeout=%v", n.Timeout, DefaultTimeout)
	}

	if strings.HasSuffix(n.Endpoint, "/") {
		t.Fatalf("ValidateConfiguration() does not trim trailing / from URL, leaves '%v'", n.Endpoint)
	}
}

func TestPrepareNotificationConnectionError(t *testing.T) {
	n := TattlerClientHTTP{
		Endpoint: api_base_test,
		Scope:    "testScope",
	}

	n.Timeout = time.Duration(-1) * time.Second
	if err := n.ValidateConfiguration(); err == nil || !strings.Contains(err.Error(), "imeout") {
		t.Fatalf("ValidateConfiguration() fails to raise error upon invalid Timeout=%v, or error fails to mention 'imeout' (err=%v)", n.Timeout, err.Error())
	}

	n.Timeout = DefaultTimeout
	n.Mode = strings.Join(NotificationModes, "") // some invalid mode name
	if err := n.ValidateConfiguration(); err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("ValidateConfiguration() fails to raise error upon invalid Mode=%v, or error fails to mention 'mode' (err=%v)", n.Mode, err)
	}

	n.Endpoint = "invalid_url"
	if err := n.ValidateConfiguration(); err == nil || !strings.Contains(err.Error(), "ndpoint") {
		t.Fatalf("ValidateConfiguration() fails to raise error upon invalid URL=%v, or fails to mention 'ndpoint' (err=%v)", n.Endpoint, err)
	}
}

func TestPrepareNotificationError(t *testing.T) {
	n := TattlerClientHTTP{
		Endpoint: api_base_test,
		Scope:    "testScope",
	}

	params := make(map[string]string)

	_, _, _, err := n.PrepareNotification("", "my_important_event", params, []string{}, "corrid123")
	if err == nil {
		t.Fatalf("PrepareNotification() failed to returned error when provided with empty recipient")
	}

	_, _, _, err2 := n.PrepareNotification("678", "", params, []string{}, "corrid123")
	if err2 == nil {
		t.Fatalf("PrepareNotification() failed to returned error when provided with empty event_name")
	}

	_, _, _, err3 := n.PrepareNotification("", "", params, []string{}, "corrid123")
	if err3 == nil {
		t.Fatalf("PrepareNotification() failed to returned error when provided with empty recipient and event_name")
	}
}

func TestPreparedClientTimeout(t *testing.T) {
	n := TattlerClientHTTP{
		Endpoint: api_base_test,
		Scope:    "testScope",
		Timeout:  2 * DefaultTimeout,
	}

	params := make(map[string]string)
	params["foo"] = "bar"
	nurl, nbody, _, _ := n.PrepareNotification("636", "ev", params, []string{}, "")
	req, cli, rerr := n.prepareHTTPRequest(nurl, nbody)
	if rerr != nil {
		t.Fatalf("prepareHTTPRequest() unexpectedly failed with '%v'", rerr)
	}
	cthead := req.Header.Get("Content-Type")
	if !strings.HasPrefix(cthead, "application/json") {
		t.Fatalf("prepareHTTPRequest() has wrong Content-Type header '%v' instead of 'application/json'", cthead)
	}
	if cli.Timeout != 2*DefaultTimeout {
		t.Fatalf("prepareHTTPRequest() sets wrong timeout = '%v'", cli.Timeout)
	}
}

func TestFailedResponseProducesError(t *testing.T) {
	n := TattlerClientHTTP{
		Endpoint: api_base_test,
		Scope:    "testScope",
	}

	if n.processResponse(200, "200 OK", api_base_test, []byte{}, "") != nil {
		t.Fatalf("processResponse() returns failure upon successful run")
	}

	if n.processResponse(400, "200 OK", api_base_test, []byte{}, "") == nil {
		t.Fatalf("processResponse() returns no error upon failed run, if status description is '200' but status code is not")
	}
}

func TestPersist(t *testing.T) {
	fpath, err := os.MkdirTemp("", "test.*")
	if err != nil {
		t.Fatalf("Could not create tmpdir to test fscache: %v", err)
	}
	defer os.RemoveAll(fpath)

	n := TattlerClientHTTP{
		Endpoint:       api_base_test,
		Scope:          "testScope",
		PersistencyDir: fpath,
	}

	params := make(map[string]string)
	params["foo"] = "bar"
	urlstr, _, taskname, _ := n.PrepareNotification("636", "ev", params, []string{}, "correlId")
	if taskname == "" {
		t.Fatalf("PrepareNotification() failed to return non-empty taskname")
	}

	for _, exppart := range []string{"url", "body"} {
		fname := fmt.Sprintf("%v_%v", taskname, exppart)
		expfname := path.Join(n.PersistencyDir, fname)
		_, err = os.Stat(expfname)
		if err != nil {
			t.Fatalf("PrepareNotification() claims to have persisted task %v but file %v", taskname, expfname)
		}
	}

	n.processResponse(400, "200 OK", urlstr, []byte{}, taskname)
	for _, exppart := range []string{"url", "body"} {
		fname := fmt.Sprintf("%v_%v", taskname, exppart)
		expfname := path.Join(n.PersistencyDir, fname)
		_, err = os.Stat(expfname)
		if err != nil {
			t.Fatalf("processResponse() removes persisted task %v despite HTTP error response (%v)", taskname, expfname)
		}
	}

	n.processResponse(200, "200 OK", urlstr, []byte{}, taskname)
	for _, exppart := range []string{"url", "body"} {
		fname := fmt.Sprintf("%v_%v", taskname, exppart)
		expfname := path.Join(n.PersistencyDir, fname)
		_, err = os.Stat(expfname)
		if err == nil {
			t.Fatalf("processResponse() fails to remove persisted task %v despite HTTP success response (%v)", taskname, expfname)
		}
	}
}

func TestSendNotificationWithBody(t *testing.T) {
	// prepare server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/notification/myscope/my_important_event") {
			t.Errorf("Expected to request configured endpoint '%v' got: %s", "/notification/myscope/my_important_event", r.URL.Path)
		}
		if !strings.Contains(r.URL.RawQuery, "user=456") {
			t.Errorf("Expected to request delivery to user=456 got: %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "mode=debug") {
			t.Errorf("Expected to request delivery to mode=debug got: %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			t.Errorf("Expected Content-Type header to contain 'application/json', got: %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Expected Accept: application/json header, got: %s", r.Header.Get("Accept"))
		}
		body, berr := io.ReadAll(r.Body)
		if berr != nil {
			t.Errorf("Expected some body, got nothing.")
		}
		var jbody map[string]interface{}
		jerr := json.Unmarshal(body, &jbody)
		if jerr != nil {
			t.Errorf("Failed to JSON-parse request: %v", jerr)
		}
		val, ok := jbody["foo"]
		if !ok {
			t.Errorf("Expected param 'foo' in request body, not found. %v", jbody)
		}
		if val != "string" {
			t.Errorf("Expected param 'foo'='string', got %v", val)
		}
		val, ok = jbody["bar"]
		if !ok {
			t.Errorf("Expected param 'bar' in request body, not found. %v", jbody)
		}
		if val != "2024-07-16T21:02:59Z" {
			t.Errorf("Expected param 'bar'='2024-07-16T21:02:59Z', got %v", val)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"email:49b99061-f5bc-4d58-9f79-fce37106877f","vector":"email","resultCode":0,"result":"success","detail":"OK"}`))
	}))
	defer server.Close()

	params := make(map[string]string)
	params["foo"] = "string"
	params["bar"] = "2024-07-16T21:02:59Z"
	vectors := []string{"email"}

	n := TattlerClientHTTP{
		Endpoint: server.URL,
		Scope:    "myscope",
		Mode:     "debug",
	}

	err := n.SendNotification("456", "my_important_event", params, vectors, "corrid123")
	if err != nil {
		t.Fatalf("SendNotification unexpectedly rejected valid request with body: %v", err)
	}
}

func TestDefaultMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		qrparams, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			t.Errorf("Failed to parse query '%v'", r.URL.RawQuery)
		}
		if qrparams.Has("mode") && qrparams.Get("mode") != "debug" {
			t.Errorf("Mode expected to either be omitted, or specified as 'debug', instead got '%v'", qrparams.Get("mode"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"email:49b99061-f5bc-4d58-9f79-fce37106877f","vector":"email","resultCode":0,"result":"success","detail":"OK"}`))
	}))
	defer server.Close()

	n := TattlerClientHTTP{
		Endpoint: server.URL,
		Scope:    "myscope",
	}

	params := make(map[string]string)
	vectors := []string{}

	err := n.SendNotification("456", "my_important_event", params, vectors, "corrid123")
	if err != nil {
		t.Fatalf("SendNotification unexpectedly rejected valid request with body: %v", err)
	}
}

func TestSendNotificationSkipsInvalidVectors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		qrparams, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			t.Errorf("Failed to parse query '%v'", r.URL.RawQuery)
		}
		if !qrparams.Has("vector") {
			t.Errorf("Expected param 'vector' missing from '%v'", r.URL.RawQuery)
		}
		if qrparams.Get("vector") != "valid1,v2,valid3" {
			t.Errorf("Param 'vector' expected 'valid1,v2,valid3' but got '%v'", qrparams.Get("vector"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"email:49b99061-f5bc-4d58-9f79-fce37106877f","vector":"email","resultCode":0,"result":"success","detail":"OK"}`))
	}))
	defer server.Close()

	n := TattlerClientHTTP{
		Endpoint: server.URL,
		Scope:    "myscope",
	}

	params := make(map[string]string)
	vectors := []string{"valid1", "v2", " valid3", "in valid"}

	err := n.SendNotification("456", "my_important_event", params, vectors, "corrid123")
	if err != nil {
		t.Fatalf("SendNotification unexpectedly rejected valid request with body: %v", err)
	}

}

func TestSendNotificationError(t *testing.T) {
	params := make(map[string]string)
	params["foo"] = "string"
	params["bar"] = "2024-07-16T21:02:59Z"
	vectors := []string{"email"}

	n := TattlerClientHTTP{
		Endpoint: " invalid  / url!",
		Scope:    "myscope",
	}

	err := n.SendNotification("456", "my_important_event", params, vectors, "corrid123")
	if err == nil {
		t.Fatalf("SendNotification unexpectedly accepted invalid API Endpoing '%v'", n.Endpoint)
	}
}

func TestSendNotificationServerError(t *testing.T) {
	params := make(map[string]string)
	vectors := []string{"email"}

	n := TattlerClientHTTP{
		Endpoint: "",
		Scope:    "myscope",
		Mode:     "debug",
	}

	for _, statusCode := range []int{
		http.StatusBadRequest,
		http.StatusBadGateway,
		http.StatusGatewayTimeout,
		http.StatusInternalServerError,
	} {
		// prepare server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(statusCode)
		}))

		n.Endpoint = server.URL

		err := n.SendNotification("456", "my_important_event", params, vectors, "corrid123")
		server.Close()
		if err == nil {
			t.Fatalf("SendNotification unexpectedly succeeded upon server error %v", statusCode)
		}
	}
}
