package nex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Request structure for testing
type TestRequest struct {
	ClientID string `json:"client_id"`
	Message  string `json:"message"`
}

// Response structure for testing
type TestResponse struct {
	ReceivedClientID string `json:"received_client_id"`
	ProcessedBy      string `json:"processed_by"`
}

// Mock handler function
func testHandler(req *TestRequest) (*TestResponse, error) {
	// Add some processing time to increase the chance of race conditions
	time.Sleep(1 * time.Millisecond)

	return &TestResponse{
		ReceivedClientID: req.ClientID,
		ProcessedBy:      "server",
	}, nil
}

func TestConcurrentInvoke(t *testing.T) {
	// Create a Nex handler
	handler := Handler(testHandler)

	// Start a test HTTP server
	server := httptest.NewServer(handler)
	defer server.Close()

	const numClients = 100
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Slice to collect all responses
	responses := make([]TestResponse, 0, numClients)
	// Slice to collect errors
	errors := make([]error, 0)

	// Launch multiple concurrent clients
	for i := 1; i <= numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			// Prepare request data
			reqData := TestRequest{
				ClientID: fmt.Sprintf("client%d", clientID),
				Message:  fmt.Sprintf("message from client %d", clientID),
			}

			jsonData, err := json.Marshal(reqData)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("client%d marshal error: %v", clientID, err))
				mu.Unlock()
				return
			}

			// Send HTTP POST request
			resp, err := http.Post(server.URL, "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("client%d request error: %v", clientID, err))
				mu.Unlock()
				return
			}
			defer resp.Body.Close()

			// Parse response
			var response TestResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("client%d decode error: %v", clientID, err))
				mu.Unlock()
				return
			}

			// Save response
			mu.Lock()
			responses = append(responses, response)
			mu.Unlock()
		}(i)
	}

	// Wait for all requests to complete
	wg.Wait()

	// Check for errors
	if len(errors) > 0 {
		for _, err := range errors {
			t.Errorf("Request error: %v", err)
		}
	}

	// Check number of responses
	if len(responses) != numClients {
		t.Errorf("Expected %d responses, got %d", numClients, len(responses))
	}

	// Check for duplicate client IDs
	clientIDMap := make(map[string][]int)
	for i, resp := range responses {
		clientIDMap[resp.ReceivedClientID] = append(clientIDMap[resp.ReceivedClientID], i)
	}

	// Find duplicates and missing IDs
	duplicates := make(map[string][]int)
	missing := make([]string, 0)

	for i := 1; i <= numClients; i++ {
		expectedClientID := fmt.Sprintf("client%d", i)
		if indices, exists := clientIDMap[expectedClientID]; exists {
			if len(indices) > 1 {
				duplicates[expectedClientID] = indices
			}
		} else {
			missing = append(missing, expectedClientID)
		}
	}

	// Report findings
	t.Logf("Total responses received: %d", len(responses))
	t.Logf("Unique client IDs: %d", len(clientIDMap))

	if len(duplicates) > 0 {
		t.Errorf("Found duplicate client IDs:")
		for clientID, indices := range duplicates {
			t.Errorf("  %s appeared %d times at indices: %v", clientID, len(indices), indices)
		}
	}

	if len(missing) > 0 {
		t.Errorf("Missing client IDs: %v", missing)
	}

	// Print first 10 responses for debugging
	t.Logf("First 10 responses:")
	for i := 0; i < 10 && i < len(responses); i++ {
		t.Logf("  Response %d: %s", i, responses[i].ReceivedClientID)
	}
}

// High concurrency stress test version
func TestHighConcurrentInvoke(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high concurrent test in short mode")
	}

	// Create a Nex handler
	handler := Handler(testHandler)

	// Start a test HTTP server
	server := httptest.NewServer(handler)
	defer server.Close()

	const numClients = 1000
	const concurrency = 50 // Maximum number of concurrent requests

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Collect all responses and errors
	responses := make([]TestResponse, 0, numClients)
	errors := make([]error, 0)

	// Use a semaphore to control concurrency
	semaphore := make(chan struct{}, concurrency)

	// Launch multiple concurrent clients
	for i := 1; i <= numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Prepare request data
			reqData := TestRequest{
				ClientID: fmt.Sprintf("client%d", clientID),
				Message:  fmt.Sprintf("message from client %d", clientID),
			}

			jsonData, err := json.Marshal(reqData)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("client%d marshal error: %v", clientID, err))
				mu.Unlock()
				return
			}

			// Send HTTP POST request with timeout
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Post(server.URL, "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("client%d request error: %v", clientID, err))
				mu.Unlock()
				return
			}
			defer resp.Body.Close()

			// Parse response
			var response TestResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("client%d decode error: %v", clientID, err))
				mu.Unlock()
				return
			}

			// Save response
			mu.Lock()
			responses = append(responses, response)
			mu.Unlock()
		}(i)
	}

	// Wait for all requests to complete
	wg.Wait()

	// Analyze results
	clientIDCount := make(map[string]int)
	for _, resp := range responses {
		clientIDCount[resp.ReceivedClientID]++
	}

	duplicateCount := 0
	for clientID, count := range clientIDCount {
		if count > 1 {
			t.Logf("Client ID %s appeared %d times", clientID, count)
			duplicateCount++
		}
	}

	t.Logf("Total responses: %d", len(responses))
	t.Logf("Unique client IDs: %d", len(clientIDCount))
	t.Logf("Duplicate client IDs: %d", duplicateCount)
	t.Logf("Errors: %d", len(errors))

	if duplicateCount > 0 {
		t.Errorf("Found %d duplicate client IDs in concurrent test", duplicateCount)
	}
}
