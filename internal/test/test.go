package test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func ReadFile(path ...string) string {
	_, file, _, _ := runtime.Caller(1)
	filePath := filepath.Join(append([]string{filepath.Dir(file)}, path...)...)
	fileBytes := Must(os.ReadFile(filePath))
	return string(fileBytes)
}

func RandomPortAddress() (*net.TCPAddr, error) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("failed to find random port for HTTP server: %w", err)
	}
	defer func() { _ = ln.Close() }()
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("failed to cast listener address to TCPAddr")
	}
	return tcpAddr, nil
}

// reachableAddr is the address to dial. Listening on "0.0.0.0:0" is dual-stack on
// Darwin and Linux, so the listener address is "[::]:port". Connecting to that
// wildcard fails on hosts that can bind "::" but cannot connect to it (Go issue
// 18806), which showed up as "dial tcp [::]:port: connect: connection refused"
// on macos-15-intel. Loopback reaches the same listener.
func reachableAddr(tcpAddr *net.TCPAddr) string {
	ip := tcpAddr.IP
	if ip == nil || ip.IsUnspecified() {
		ip = net.IPv4(127, 0, 0, 1)
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(tcpAddr.Port))
}

// WaitForServer waits up to 5 seconds for the server to accept TCP connections.
func WaitForServer(tcpAddr *net.TCPAddr) error {
	addr := reachableAddr(tcpAddr)
	deadline := time.Now().Add(5 * time.Second)
	var err error
	for {
		var conn net.Conn
		conn, err = net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("server at %s did not start: %w", addr, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// WaitForHealthz waits for the /healthz endpoint to return a non-404 response
func WaitForHealthz(tcpAddr *net.TCPAddr) error {
	url := fmt.Sprintf("http://%s/healthz", reachableAddr(tcpAddr))
	var resp *http.Response
	var err error
	for i := 0; i < 100; i++ {
		resp, err = http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("healthz endpoint returned 404 after retries")
}
