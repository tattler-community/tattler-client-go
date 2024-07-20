/*
tattler_go is a Go client for tattler.dev.
It marshals and delivers notification requests via tattler's REST API, see https://tattler.dev .
It supports local persistency to reliably occurrences where tattler server cannot be reached.

Basic usage:

	notifcli := TattlerClientHTTP{
		Scope:		"mybillingsystem",
		Timeout:	5000,
		Endpoint: 	"http://localhost:11503/notification",
		Mode:		"production",
	}
	myContext := make(map[string]string)
	myContext["amount"] = "10.20"
	myContext["invoice_number"] = "20230512"
	err := notifcli.SendNotification("7598", "new_invoice_created", myContext)

Notice that "Mode" defaults to "debug", so notifications are sent to the debug address
instead of the requested recipient, unless explicitly changed. Find details at
https://docs.tattler.dev/en/latest/keyconcepts/mode.html .

This module supports persistency. If enabled, then notification attempts are stored
in TattlerClientHTTP.PersistencyDir, and only removed if the notification succeeded.
This preserves notifications sent while the server was unreachable, and allows
replaying failed deliveries after the fact.

Persistency is organized as follows: each uncompleted notification attempt is stored
as a pair of files (cache keys), named:
- `{timestamp}_{randint}_url` -- whose content is the URL sent to tattler
- `{timestamp}_{randint}_body` -- whose content is the JSON body POSTed to tattler
*/
package tattler_go

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/kataras/golog"
	"github.com/tattler-community/tattler-client-go/fscache"
)

// TattlerClientHTTP configures communication with a Tattler server over the HTTP protocol.
// Its attributes should be set before calling any method.
// Attributes belong in this structure when they are session-dependent (e.g. a Timeout), and not delivery-dependent (e.g. which vectors to deliver to)
type TattlerClientHTTP struct {
	// name to present to Tattler server with; see Tattler server docs for its semantic.
	Scope string
	// Base URL to reach Tattler server at; actual notifications will be composed by suffixing paths to this base URL.
	Endpoint string
	// How long to wait for a request to Tattler server to complete.
	Timeout time.Duration
	// Operating mode to request to Tattler server; see Tattler server docs for "Modes" for its semantic.
	Mode string
	// Attempt to persist tasks in this folder before sending notifications; clear the task if the notification succeeded.
	PersistencyDir string
}

// Default timeout to use when none is given in TattlerClientHTTP structure
const DefaultTimeout time.Duration = 5 * time.Second

// List of supported notification modes; see docs of Tattler Server for their semantics
var NotificationModes = []string{"production", "staging", "debug"}

// Notification mode to use when no custom mode is requested
const DefaultMode string = "debug"

// Returns the position of an item in a slice, or -1 if not found
func find(haystack []string, needle string) int {
	for i, v := range haystack {
		if v == needle {
			return i
		}
	}
	return -1
}

func mkJSONContext(params map[string]string) ([]byte, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to make JSON for params: %v", err)
	}
	return data, nil
}

// normalize a vector name, if valid, else return false
func normalizeVectorName(vname string) (string, bool) {
	normalizedName := strings.ToLower(strings.TrimSpace(vname))
	matched, _ := regexp.MatchString("^[a-z0-9_-]+$", normalizedName)
	if !matched {
		return "", false
	}
	return normalizedName, true
}

/*
Validate configuration items set in TattlerClientHTTP structions, and set missing ones to default.

Return nil if configuration is valid; an error description otherwise.
*/
func (c *TattlerClientHTTP) ValidateConfiguration() error {
	c.Endpoint = strings.TrimSpace(c.Endpoint)
	c.Endpoint = strings.Trim(c.Endpoint, "/")
	c.Scope = strings.TrimSpace(c.Scope)
	c.Mode = strings.TrimSpace(c.Mode)
	if c.Timeout == time.Duration(0) {
		c.Timeout = DefaultTimeout
	} else if c.Timeout < 0 {
		return fmt.Errorf("client configuration has invalid Timeout=%v < 0", c.Timeout)
	}
	if c.Endpoint == "" {
		return fmt.Errorf("client configuration has invalid server endpoint; want http://foo.com:1234/path, have '%v'", c.Endpoint)
	} else if _, err := url.ParseRequestURI(c.Endpoint); err != nil {
		return fmt.Errorf("client configuration's server endpoint is not a valid URL, have '%v'", c.Endpoint)
	}
	if c.Scope == "" {
		return fmt.Errorf("client configuration has invalid scope; want http://foo.com:1234/path, have '%v'", c.Scope)
	}
	if c.Mode == "" {
		c.Mode = DefaultMode
	} else if find(NotificationModes, c.Mode) == -1 {
		return fmt.Errorf("invalid mode '%v' requested out of supported '%v'; giving up delivery altogether", c.Mode, NotificationModes)
	}
	return nil
}

func (c *TattlerClientHTTP) mkTattlerRequestURL(recipient string, event_name string, params map[string]string, vectors []string, correlationId string) (string, error) {
	if err := c.ValidateConfiguration(); err != nil {
		return "", fmt.Errorf("validating configuration failed: %v", err)
	}
	// process vectors
	var validVectors []string
	if len(vectors) > 0 {
		// some vectors requested. Validate them
		for _, v := range vectors {
			normvname, valid := normalizeVectorName(v)
			if valid {
				validVectors = append(validVectors, normvname)
			} else {
				golog.Warnf("SendNotification() of %v to %v requests invalid vector %v; ignoring", event_name, recipient, v)
			}
		}
	}
	queryParams := map[string]string{}
	queryParams["mode"] = c.Mode
	queryParams["user"] = recipient
	if len(validVectors) > 0 {
		queryParams["vector"] = strings.Join(validVectors, ",")
	}
	correlationId = strings.TrimSpace(correlationId)
	if correlationId != "" {
		queryParams["correlationId"] = correlationId
	} else {
		queryParams["correlationId"] = fmt.Sprintf("%x%x", rand.Uint64(), rand.Uint64())
	}
	var paramsPart []string
	for k, v := range queryParams {
		p := fmt.Sprintf("%v=%v", url.QueryEscape(k), url.QueryEscape(v))
		paramsPart = append(paramsPart, p)
	}
	paramstr := strings.Join(paramsPart, "&")
	finalURL := fmt.Sprintf("%v/notification/%v/%v/?%v", c.Endpoint, c.Scope, event_name, paramstr)
	return finalURL, nil
}

// PrepareNotification prepares URL and Body to send to Tattler over HTTP for sending a notification.
//
// PrepareNotification returns error if the underlying TattlerClientHTTP object is misconfigured
func (n *TattlerClientHTTP) PrepareNotification(recipient string, event_name string, params map[string]string, vectors []string, correlationId string) (string, []byte, string, error) {
	recipient = strings.TrimSpace(recipient)
	event_name = strings.TrimSpace(event_name)
	if recipient == "" || event_name == "" {
		return "", nil, "", fmt.Errorf("failed to send notification '%v' to '%v': empty recipient or event_name provided", event_name, recipient)
	}

	// URL
	urlstr, urlerr := n.mkTattlerRequestURL(recipient, event_name, params, vectors, correlationId)
	if urlerr != nil {
		return "", nil, "", fmt.Errorf("failed to assemble URL for notification server: %v", urlerr)
	}
	golog.Debugf("Prepared tattler URL=%v", urlstr)

	// Body
	body, err := mkJSONContext(params)
	if err != nil {
		return "", nil, "", fmt.Errorf("failed to encode params: %v", err)
	}
	golog.Debugf("Prepared body for notification server of %v bytes='%v'", len(body), body)

	taskname, persisterr := n.PersistTask(urlstr, body)
	if persisterr != nil {
		golog.Errorf("Error persisting task: '%v' (ignoring)", persisterr)
	}

	return urlstr, body, taskname, nil
}

func (n *TattlerClientHTTP) prepareHTTPRequest(urlstr string, body []byte) (*http.Request, *http.Client, error) {
	// request
	request, reqerr := http.NewRequest("POST", urlstr, bytes.NewBuffer(body))
	if reqerr != nil {
		return nil, nil, fmt.Errorf("failed to make tattler request with %v: %v", urlstr, reqerr)
	}
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	request.Header.Set("Accept", "application/json")

	client := &http.Client{}
	client.Timeout = n.Timeout

	return request, client, nil
}

func (n *TattlerClientHTTP) processResponse(statusCode int, statusText string, urlstr string, body []byte, taskname string) error {
	if statusCode != 200 {
		var extraPersistMsg string
		if n.PersistencyDir != "" {
			extraPersistMsg = " (keeping persistent task)"
		}
		return fmt.Errorf("tattler req '%v' failed with %v%v: %v", urlstr, extraPersistMsg, statusCode, statusText)
	}

	if taskname != "" {
		n.ClearTask(taskname)
	}
	golog.Infof("Notification -> %v sent: %v %v", urlstr, statusCode, string(body))
	return nil
}

/*
Send a notification about an event to a recipient.

Validate the undelying connection settings and send the notification. If vectors are omitted, they default to all available vectors for the user.
If a non-empty correlationId is provided, it is passed on in the request to the Tattler server, else a new one is auto-generated.
*/
func (n *TattlerClientHTTP) SendNotification(recipient string, event_name string, params map[string]string, vectors []string, correlationId string) error {
	urlstr, body, taskname, berr := n.PrepareNotification(recipient, event_name, params, vectors, correlationId)
	if berr != nil {
		return fmt.Errorf("failed to prepare tattler request: %v", berr)
	}

	request, client, rerr := n.prepareHTTPRequest(urlstr, body)
	if rerr != nil {
		return fmt.Errorf("failed to prepare request: %v", rerr)
	}
	resp, resperr := client.Do(request)
	if resperr != nil {
		return fmt.Errorf("failed to request tattler %v: %v", urlstr, resperr)
	}
	defer resp.Body.Close()

	respbody, _ := io.ReadAll(resp.Body)
	return n.processResponse(resp.StatusCode, resp.Status, urlstr, respbody, taskname)
}

func (n *TattlerClientHTTP) PersistTask(requrl string, reqbody []byte) (string, error) {
	if n.PersistencyDir == "" {
		golog.Debug("Not persisting task because PersistencyDir empty.")
		return "", nil
	}
	cache, err := fscache.GetInstance(n.PersistencyDir)
	if err != nil {
		return "", fmt.Errorf("failed to load cache to persist task: %v", err)
	}
	taskname := fmt.Sprintf("%v_%x", time.Now().Unix(), rand.Uint32())
	urlkname := fmt.Sprintf("%v_url", taskname)
	urlerr := cache.Set(urlkname, []byte(requrl))
	if urlerr != nil {
		return "", fmt.Errorf("failed to persist request URL part into %v: %v", urlkname, urlerr)
	}
	bodykname := fmt.Sprintf("%v_body", taskname)
	bodyerr := cache.Set(bodykname, []byte(reqbody))
	if bodyerr != nil {
		return "", fmt.Errorf("failed to persist request body part into %v: %v", bodykname, urlerr)
	}
	golog.Infof("Task journalled successfully with keys=%v_{url, body}", taskname)
	return taskname, nil
}

func (n *TattlerClientHTTP) ClearTask(taskname string) error {
	if taskname == "" {
		golog.Debugf("Omitting clearing empty taskname.")
		return nil
	}
	if n.PersistencyDir == "" {
		golog.Warnf("Requested to ClearTask() when PersistencyDir disabled")
		return fmt.Errorf("cannot ClearTask(%v) because PersistencyDir is disabled", taskname)
	}
	for _, part := range []string{"url", "body"} {
		rpath := fmt.Sprintf("%v_%v", taskname, part)
		os.Remove(rpath)
	}
	cache, err := fscache.GetInstance(n.PersistencyDir)
	if err != nil {
		return fmt.Errorf("failed to load cache to clear task %v: %v", taskname, err)
	}
	for _, part := range []string{"url", "body"} {
		cache.Unset(fmt.Sprintf("%v_%v", taskname, part))
	}
	golog.Infof("Task %v successfully cleared from journal.")
	return nil
}
