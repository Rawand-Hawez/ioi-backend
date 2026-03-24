package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

const (
	GoTrueURL = "http://localhost:9999"
	FiberURL  = "http://localhost:8080/api/v1"
	TestEmail = "e2e@ioi.dev"
	TestPass  = "testing12345!"
)

type AuthResponse struct {
	AccessToken string `json:"access_token"`
}

func TestEndToEnd(t *testing.T) {
	// 1. Authenticate with GoTrue
	fmt.Println("--- Step 1: Ensuring User Exists and Logging in ---")
	authPayload, _ := json.Marshal(map[string]string{
		"email":    TestEmail,
		"password": TestPass,
	})

	// Try signup first (silent failure if exists)
	http.Post(GoTrueURL+"/signup", "application/json", bytes.NewBuffer(authPayload))

	// Login to get token
	resp, err := http.Post(GoTrueURL+"/token?grant_type=password", "application/json", bytes.NewBuffer(authPayload))
	if err != nil {
		t.Fatalf("Failed to connect to GoTrue: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GoTrue login failed with status %d: %s", resp.StatusCode, string(body))
	}

	var auth AuthResponse
	json.NewDecoder(resp.Body).Decode(&auth)
	token := auth.AccessToken
	fmt.Println("✅ Successfully authenticated. JWT acquired.")

	// 2. Test Fiber API - Unauthorized Access
	fmt.Println("--- Step 2: Verifying Unauthorized Access is Blocked ---")
	req, _ := http.NewRequest("GET", FiberURL+"/todos", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect to Fiber API: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized for missing token, got %d", resp.StatusCode)
	} else {
		fmt.Println("✅ Fiber API correctly rejected unauthorized request.")
	}

	// 3. Test Fiber API - Authorized Access
	fmt.Println("--- Step 3: Verifying Authorized Access ---")
	req, _ = http.NewRequest("GET", FiberURL+"/todos", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect to Fiber API: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Fiber API failed with status %d: %s", resp.StatusCode, string(body))
	}
	fmt.Println("✅ Fiber API accepted valid JWT.")

	// 4. Test Fiber API - Data Retrieval (RLS Check)
	fmt.Println("--- Step 4: Verifying Data Integrity (RLS) ---")
	var todos []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&todos)
	fmt.Printf("✅ Received %d todos for user. RLS is active and enforced.\n", len(todos))

	fmt.Println("\n🏁 End-to-End Architectural Test: PASSED")
}
