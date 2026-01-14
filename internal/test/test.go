package test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

func WaitForServer(tcpAddr *net.TCPAddr) error {
	var conn *net.TCPConn
	var err error
	for i := 0; i < 10; i++ {
		conn, err = net.DialTCP("tcp", nil, tcpAddr)
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

// WaitForHealthz waits for the /healthz endpoint to return a non-404 response
func WaitForHealthz(tcpAddr *net.TCPAddr) error {
	url := fmt.Sprintf("http://%s/healthz", tcpAddr.String())
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
