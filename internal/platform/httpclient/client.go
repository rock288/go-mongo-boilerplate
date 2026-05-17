package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

// rfc1918 / loopback / link-local CIDRs blocked by the external dialer.
var blockedCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",
		"::1/128",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"fc00::/7",
		"fe80::/10",
		"0.0.0.0/8",
	}
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}()

// ErrBlockedAddress is returned by the external client when the resolved
// remote IP belongs to a blocked CIDR (SSRF defense).
var ErrBlockedAddress = errors.New("httpclient: address blocked by external client policy")

// NewInternalClient is for service-to-service calls inside a trust boundary.
// No SSRF restrictions; suitable for hardcoded internal URLs.
func NewInternalClient(cfg config.HTTPClientConfig) *http.Client {
	t := baseTransport(cfg, (&net.Dialer{Timeout: 5 * time.Second}).DialContext)
	return &http.Client{
		Transport:     t,
		Timeout:       timeoutOr(cfg.Timeout, 10*time.Second),
		CheckRedirect: capRedirects(maxOr(cfg.MaxRedirects, 5)),
	}
}

// NewExternalClient is for calls that may touch user-supplied URLs.
// Blocks loopback / RFC1918 / link-local destinations to prevent SSRF.
func NewExternalClient(cfg config.HTTPClientConfig) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	dialCtx := func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if isBlocked(ip) {
				return nil, &net.OpError{Op: "dial", Net: network, Err: ErrBlockedAddress}
			}
		}
		// Use the first allowed IP, sidestepping CGO resolver re-resolution.
		for _, ip := range ips {
			conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if derr == nil {
				return conn, nil
			}
			if errors.Is(derr, syscall.ECONNREFUSED) {
				continue
			}
			return nil, derr
		}
		return nil, errors.New("httpclient: all addresses failed")
	}
	t := baseTransport(cfg, dialCtx)
	return &http.Client{
		Transport:     t,
		Timeout:       timeoutOr(cfg.Timeout, 10*time.Second),
		CheckRedirect: capRedirects(maxOr(cfg.MaxRedirects, 5)),
	}
}

func isBlocked(ip net.IP) bool {
	if ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	for _, n := range blockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func baseTransport(cfg config.HTTPClientConfig, dialCtx func(context.Context, string, string) (net.Conn, error)) *http.Transport {
	return &http.Transport{
		DialContext:           dialCtx,
		MaxIdleConns:          maxOr(cfg.MaxIdleConns, 100),
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
}

func capRedirects(max int) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= max {
			return http.ErrUseLastResponse
		}
		return nil
	}
}

func timeoutOr(d, fallback time.Duration) time.Duration {
	if d <= 0 {
		return fallback
	}
	return d
}

func maxOr(n, fallback int) int {
	if n <= 0 {
		return fallback
	}
	return n
}
