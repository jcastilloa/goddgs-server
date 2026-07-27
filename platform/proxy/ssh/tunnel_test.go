package ssh

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestTunnelServesSOCKS5ConnectThroughRemoteDialer(t *testing.T) {
	remote := newRecordingRemote(t)
	tunnel := newTunnelForTest(remote)
	client, server := net.Pipe()
	defer client.Close()

	done := make(chan struct{})
	go func() {
		tunnel.serveSOCKS(server)
		close(done)
	}()

	writeAll(t, client, []byte{5, 1, 0})
	if got := readExactly(t, client, 2); string(got) != string([]byte{5, 0}) {
		t.Fatalf("greeting response = %v, want [5 0]", got)
	}

	writeAll(t, client, append([]byte{5, 1, 0, 3, byte(len("example.com"))}, append([]byte("example.com"), 0, 80)...))
	if got := readExactly(t, client, 10); string(got) != string([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("connect response = %v, want success", got)
	}

	call := <-remote.calls
	if call.network != "tcp" || call.address != "example.com:80" {
		t.Errorf("remote dial = (%q, %q), want (tcp, example.com:80)", call.network, call.address)
	}

	writeAll(t, client, []byte("ping"))
	if got := readExactly(t, remote.peer, 4); string(got) != "ping" {
		t.Errorf("remote peer read = %q, want ping", got)
	}
	writeAll(t, remote.peer, []byte("pong"))
	if got := readExactly(t, client, 4); string(got) != "pong" {
		t.Errorf("client read = %q, want pong", got)
	}

	client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SOCKS handler did not return")
	}
}

func TestTunnelRejectsSOCKS5WhenNoAuthenticationMethodIsAccepted(t *testing.T) {
	tunnel := newTunnelForTest(newRecordingRemote(t))
	client, server := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() {
		tunnel.serveSOCKS(server)
		close(done)
	}()

	writeAll(t, client, []byte{5, 1, 2})
	if got := readExactly(t, client, 2); string(got) != string([]byte{5, 255}) {
		t.Errorf("greeting response = %v, want [5 255]", got)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SOCKS handler did not return")
	}
}

func TestConfigRejectsIncompleteTunnel(t *testing.T) {
	tests := []Config{
		{},
		{Host: "proxy.example.com", User: "deploy", PrivateKeyPath: "/key"},
	}
	for _, config := range tests {
		if err := config.validate(); err == nil {
			t.Errorf("validate(%#v) error = nil, want error", config)
		}
	}
}

func TestReconnectDelayBacksOffWithLimit(t *testing.T) {
	if got := nextReconnectDelay(time.Second); got != 2*time.Second {
		t.Errorf("nextReconnectDelay(1s) = %v, want 2s", got)
	}
	if got := nextReconnectDelay(maximumReconnectDelay); got != maximumReconnectDelay {
		t.Errorf("nextReconnectDelay(max) = %v, want %v", got, maximumReconnectDelay)
	}
}

type recordingRemote struct {
	calls chan dialCall
	peer  net.Conn
}

type dialCall struct {
	network string
	address string
}

func newRecordingRemote(t *testing.T) *recordingRemote {
	t.Helper()
	return &recordingRemote{calls: make(chan dialCall, 1)}
}

func (r *recordingRemote) DialContext(_ context.Context, network, address string) (net.Conn, error) {
	server, peer := net.Pipe()
	r.peer = peer
	r.calls <- dialCall{network: network, address: address}
	return server, nil
}

func writeAll(t *testing.T, conn net.Conn, value []byte) {
	t.Helper()
	if _, err := conn.Write(value); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readExactly(t *testing.T, conn net.Conn, length int) []byte {
	t.Helper()
	value := make([]byte, length)
	if _, err := io.ReadFull(conn, value); err != nil {
		t.Fatalf("read: %v", err)
	}
	return value
}
