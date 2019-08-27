package proxyplease

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"

	ggp "github.com/rapid7/go-get-proxied/proxy"
)

// Proxy is a struct that can be passed to NewDialContext. All variables are optional. If a value is nil,
// a default will be assigned or inferred from the local system settings.
type Proxy struct {
	URL              *url.URL
	Username         string
	Password         string
	Domain           string
	TargetURL        *url.URL
	Headers          *http.Header
	TLSConfig        *tls.Config
	AuthSchemeFilter []string
}

// DialContext is the DialContext function that should be wrapped with a
// a supported authentication scheme.
type DialContext func(ctx context.Context, network, addr string) (net.Conn, error)

// NewDialContext returns a DialContext that can be used in a variety of network types.
// The function accepts an optional Proxy type parameter.
func NewDialContext(p Proxy) DialContext {
	// assign defaults
	if p.Headers == nil {
		p.Headers = &http.Header{}
	}
	if p.TargetURL == nil {
		p.TargetURL, _ = url.Parse("https://www.google.com")
	}
	// if no provided Proxy.URL, infer from system settings
	if p.URL == nil {
		debugf("proxy> No proxy provided. Attempting to infer from system.")
		p.URL = ggp.NewProvider("").GetProxy(p.TargetURL.Scheme, p.TargetURL.String()).URL()
		// if no Proxy.URL was provided and no URL could be determined from system,
		// then assume connection is direct.
		if p.URL == nil {
			debugf("proxy> No proxy could be determined. Assuming a direct connection.")
			d := net.Dialer{}
			return d.DialContext
		}
		// WinHTTP sometimes does not provide protocol. If nil, assume HTTP
		if p.URL.Scheme == "" {
			p.URL.Scheme = "http"
		}

		debugf("proxy> Inferred proxy from system: %s", p.URL.String())
	}

	// assign user:pass if defined in URL
	if p.URL.User.Username() != "" {
		p.Username = p.URL.User.Username()
	}
	if pass, _ := p.URL.User.Password(); pass != "" {
		p.Password = pass
	}

	// return DialContext function
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// first establish TLS if https
		dialProxy := func() (net.Conn, error) {
			dialer := &net.Dialer{}
			if p.URL.Scheme == "https" {
				return tls.DialWithDialer(dialer, "tcp", p.URL.Host, p.TLSConfig)
			}
			return dialer.DialContext(ctx, network, p.URL.Host)
		}
		// return a net.Conn with a establish and authenticated proxy session
		return getProxyConn(addr, p, dialProxy)
	}
}

func getProxyConn(addr string, p Proxy, baseDial func() (net.Conn, error)) (net.Conn, error) {
	// inspect Proxy.URL.Scheme and return appropriate function
	switch p.URL.Scheme {
	case "socks4", "socks4a", "socks5", "socks5h", "socks":
		return dialAndNegotiateSOCKS(p.URL, p.Username, p.Password, addr)
	case "http", "https":
		return dialAndNegotiateHTTP(p, addr, baseDial)
	default:
		debugf("get> Unsupported proxy URL scheme '%s'", p.URL.Scheme)
		return nil, errors.New("Unsupported proxy URL scheme")
	}
}
