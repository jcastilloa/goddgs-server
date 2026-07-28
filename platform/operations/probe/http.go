package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operations "github.com/jcastilloa/goddgs-server/operations/domain"
	"golang.org/x/net/proxy"
)

type HTTPProber struct {
	timeout   time.Duration
	newClient func(string, time.Duration) (*http.Client, error)
}

func NewHTTPProber(timeout time.Duration) HTTPProber {
	return HTTPProber{timeout: timeout, newClient: newHTTPClient}
}

func (p HTTPProber) Probe(ctx context.Context, target operationsApplication.ProbeTarget, probeURL string) operationsApplication.ProbeObservation {
	startedAt := time.Now()
	observation := operationsApplication.ProbeObservation{ObservedAt: startedAt.UTC()}
	if p.timeout <= 0 {
		observation.ErrorCategory = operations.ErrorConfiguration
		return observation
	}
	client, err := p.newClient(target.TransportURL, p.timeout)
	if err != nil {
		observation.ErrorCategory = operations.ErrorConfiguration
		return observation
	}
	requestContext, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, probeURL, nil)
	if err != nil {
		observation.ErrorCategory = operations.ErrorConfiguration
		return observation
	}
	response, err := client.Do(request)
	observation.Duration = time.Since(startedAt)
	if err != nil {
		observation.ErrorCategory = classifyError(err)
		return observation
	}
	defer response.Body.Close()
	observation.HTTPStatus = response.StatusCode
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusBadRequest {
		observation.Success = true
		return observation
	}
	if response.StatusCode >= http.StatusInternalServerError {
		observation.ErrorCategory = operations.ErrorUpstream5xx
		return observation
	}
	observation.ErrorCategory = operations.ErrorInvalidResponse
	return observation
}

func newHTTPClient(transportURL string, timeout time.Duration) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if strings.TrimSpace(transportURL) != "" {
		if err := configureTransport(transport, transportURL, timeout); err != nil {
			return nil, err
		}
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

func configureTransport(transport *http.Transport, transportURL string, timeout time.Duration) error {
	parsedURL, err := url.Parse(transportURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return errors.New("invalid probe transport URL")
	}
	switch parsedURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsedURL)
		return nil
	case "socks5", "socks5h":
		return configureSOCKSTransport(transport, parsedURL, timeout)
	default:
		return fmt.Errorf("unsupported probe transport scheme %q", parsedURL.Scheme)
	}
}

func configureSOCKSTransport(transport *http.Transport, proxyURL *url.URL, timeout time.Duration) error {
	var authentication *proxy.Auth
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		authentication = &proxy.Auth{User: proxyURL.User.Username(), Password: password}
	}
	dialer, err := proxy.SOCKS5("tcp", proxyURL.Host, authentication, &net.Dialer{Timeout: timeout})
	if err != nil {
		return fmt.Errorf("create SOCKS probe transport: %w", err)
	}
	transport.DialContext = dialContext(dialer)
	return nil
}

func dialContext(dialer proxy.Dialer) func(context.Context, string, string) (net.Conn, error) {
	if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
		return contextDialer.DialContext
	}
	return func(_ context.Context, network, address string) (net.Conn, error) {
		return dialer.Dial(network, address)
	}
}

func classifyError(err error) operations.ErrorCategory {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return operations.ErrorTimeout
	case errors.Is(err, context.Canceled):
		return operations.ErrorCanceled
	}
	var netError net.Error
	var urlError *url.Error
	if errors.As(err, &netError) || errors.As(err, &urlError) {
		return operations.ErrorTransport
	}
	return operations.ErrorTransport
}
