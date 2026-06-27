package proxy

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
)

func WriteJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func PipeConns(a, b net.Conn) (int64, int64) {
	type pipeResult struct {
		name  string
		bytes int64
	}
	done := make(chan pipeResult, 2)
	go func() {
		n, _ := io.Copy(a, b)
		_ = a.Close()
		_ = b.Close()
		done <- pipeResult{name: "egress", bytes: n}
	}()
	go func() {
		n, _ := io.Copy(b, a)
		_ = b.Close()
		_ = a.Close()
		done <- pipeResult{name: "ingress", bytes: n}
	}()
	var ingressBytes, egressBytes int64
	for i := 0; i < 2; i++ {
		result := <-done
		if result.name == "ingress" {
			ingressBytes = result.bytes
		} else {
			egressBytes = result.bytes
		}
	}
	return ingressBytes, egressBytes
}

func CopyResponse(w http.ResponseWriter, resp *http.Response) int64 {
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	n, _ := io.Copy(w, resp.Body)
	return n
}
