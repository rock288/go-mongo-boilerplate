package httpclient

import (
	"errors"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

func TestExternalClient_BlocksLinkLocalMetadata(t *testing.T) {
	c := NewExternalClient(config.HTTPClientConfig{})
	_, err := c.Get("http://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Fatal("expected dial error for link-local")
	}
	if !errors.Is(err, ErrBlockedAddress) && !strings.Contains(err.Error(), "blocked") {
		// Some platforms wrap inside net.OpError; check substring as fallback.
		t.Fatalf("expected ErrBlockedAddress, got %v", err)
	}
}

func TestExternalClient_BlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(nil)
	defer srv.Close()
	c := NewExternalClient(config.HTTPClientConfig{})
	_, err := c.Get(srv.URL)
	if err == nil {
		t.Fatal("expected dial error for loopback")
	}
}

func TestInternalClient_AllowsLoopback(t *testing.T) {
	srv := httptest.NewServer(nil)
	defer srv.Close()
	c := NewInternalClient(config.HTTPClientConfig{})
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("internal client should allow loopback: %v", err)
	}
	_ = resp.Body.Close()
}

func TestIsBlocked_TableDriven(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},
		{"10.1.2.3", true},
		{"192.168.1.1", true},
		{"172.16.1.1", true},
		{"169.254.169.254", true},
		{"::1", true},
		{"fe80::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("bad test fixture: %s", tc.ip)
			}
			if got := isBlocked(ip); got != tc.blocked {
				t.Fatalf("isBlocked(%s) = %v, want %v", tc.ip, got, tc.blocked)
			}
		})
	}
}
