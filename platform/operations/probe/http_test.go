package probe

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operations "github.com/jcastilloa/goddgs-server/operations/domain"
)

func TestHTTPProberClassifiesResponsesErrorsAndClosesBodies(t *testing.T) {
	tests := []struct {
		name         string
		responseCode int
		roundTripErr error
		wantSuccess  bool
		wantCategory operations.ErrorCategory
	}{
		{name: "2xx", responseCode: http.StatusNoContent, wantSuccess: true},
		{name: "3xx", responseCode: http.StatusTemporaryRedirect, wantSuccess: true},
		{name: "4xx", responseCode: http.StatusBadRequest, wantCategory: operations.ErrorInvalidResponse},
		{name: "5xx", responseCode: http.StatusBadGateway, wantCategory: operations.ErrorUpstream5xx},
		{name: "transport", roundTripErr: &url.Error{Op: "Get", URL: "https://probe.example.com", Err: errors.New("unreachable")}, wantCategory: operations.ErrorTransport},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			body := &trackingBody{}
			prober := HTTPProber{timeout: time.Second, newClient: func(string, time.Duration) (*http.Client, error) {
				return &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
					if testCase.roundTripErr != nil {
						return nil, testCase.roundTripErr
					}
					return &http.Response{StatusCode: testCase.responseCode, Body: body, Header: make(http.Header)}, nil
				})}, nil
			}}
			result := prober.Probe(context.Background(), operationsApplication.ProbeTarget{Name: "proxy"}, "https://probe.example.com/health")
			if result.Success != testCase.wantSuccess {
				t.Errorf("success = %v, want %v", result.Success, testCase.wantSuccess)
			}
			if result.ErrorCategory != testCase.wantCategory {
				t.Errorf("error category = %q, want %q", result.ErrorCategory, testCase.wantCategory)
			}
			if testCase.roundTripErr == nil && !body.closed {
				t.Error("response body was not closed")
			}
		})
	}
}

func TestHTTPProberClassifiesTimeout(t *testing.T) {
	prober := HTTPProber{timeout: 10 * time.Millisecond, newClient: func(string, time.Duration) (*http.Client, error) {
		return &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		})}, nil
	}}
	result := prober.Probe(context.Background(), operationsApplication.ProbeTarget{Name: "proxy"}, "https://probe.example.com/health")
	if result.Success || result.ErrorCategory != operations.ErrorTimeout {
		t.Errorf("timeout result = %#v", result)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type trackingBody struct{ closed bool }

func (b *trackingBody) Read([]byte) (int, error) { return 0, nil }

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}
