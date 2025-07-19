package nex

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A simple handler function for benchmarking
func benchmarkHandler(req *TestRequest) (*TestResponse, error) {
	return &TestResponse{
		ReceivedClientID: req.ClientID,
		ProcessedBy:      "server",
	}, nil
}

// Benchmark for sequential handler invocation
func BenchmarkSequentialInvoke(b *testing.B) {
	handler := Handler(benchmarkHandler)
	server := httptest.NewServer(handler)
	defer server.Close()

	reqData := TestRequest{
		ClientID: "benchmark_client",
		Message:  "benchmark message",
	}

	jsonData, _ := json.Marshal(reqData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Post(server.URL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}

// Benchmark for concurrent handler invocation
func BenchmarkConcurrentInvoke(b *testing.B) {
	handler := Handler(benchmarkHandler)
	server := httptest.NewServer(handler)
	defer server.Close()

	reqData := TestRequest{
		ClientID: "benchmark_client",
		Message:  "benchmark message",
	}

	jsonData, _ := json.Marshal(reqData)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := http.Post(server.URL, "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
		}
	})
}
