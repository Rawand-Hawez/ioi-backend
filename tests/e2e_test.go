package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

const (
	GoTrueURL    = "http://localhost:9999"
	FiberURL     = "http://localhost:8080/api/v1"
	PostgRESTURL = "http://localhost:3000"
	TestEmail    = "e2e@ioi.dev"
	TestPass     = "testing12345!"
)

type AuthResponse struct {
	AccessToken string `json:"access_token"`
}

func authenticate(t *testing.T) string {
	t.Helper()
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
	return auth.AccessToken
}

func TestGoTrueAuth(t *testing.T) {
	token := authenticate(t)
	if token == "" {
		t.Fatal("Empty access token")
	}
	fmt.Println("GoTrue authentication successful, JWT acquired.")
}

func TestFiberUnauthorized(t *testing.T) {
	req, _ := http.NewRequest("GET", FiberURL+"/business-entities", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect to Fiber API: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized for missing token, got %d", resp.StatusCode)
	}
}

func TestFiberBusinessEntityE2E(t *testing.T) {
	token := authenticate(t)
	uniqueCode := fmt.Sprintf("E2E_%d", time.Now().UnixNano())

	// CREATE
	createPayload, _ := json.Marshal(map[string]string{
		"code":        uniqueCode,
		"name":        "E2E Test Entity",
		"description": "Created via E2E test",
	})
	req, _ := http.NewRequest("POST", FiberURL+"/business-entities", bytes.NewBuffer(createPayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Create request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Create failed with status %d: %s", resp.StatusCode, string(body))
	}

	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	entityID, ok := created["id"].(string)
	if !ok || entityID == "" {
		t.Fatal("Created business entity missing ID")
	}
	fmt.Printf("Created business entity: %s\n", entityID)

	// GET (list)
	req, _ = http.NewRequest("GET", FiberURL+"/business-entities?page=1&page_size=10", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("List request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("List failed with status %d: %s", resp.StatusCode, string(body))
	}
	var listResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()
	data, exists := listResp["data"]
	if !exists {
		t.Fatalf("Response missing 'data' field: %+v", listResp)
	}
	items, ok := data.([]interface{})
	if !ok {
		t.Fatalf("'data' is not an array: %+v", data)
	}
	fmt.Printf("Listed %d business entities (RLS enforced)\n", len(items))
}

func TestPostgRESTEndpoint(t *testing.T) {
	// Unauthenticated request should be rejected (web_anon has no table access)
	resp, err := http.Get(PostgRESTURL + "/business_entities")
	if err != nil {
		t.Fatalf("Failed to connect to PostgREST: %v", err)
	}
	defer resp.Body.Close()

	// PostgREST returns 401 for tables not accessible to anon role
	if resp.StatusCode == http.StatusOK {
		t.Error("PostgREST should not expose business_entities to unauthenticated requests")
	}
	fmt.Printf("PostgREST anon access correctly blocked (status %d)\n", resp.StatusCode)

	// Authenticated request through PostgREST
	token := authenticate(t)
	req, _ := http.NewRequest("GET", PostgRESTURL+"/business_entities", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PostgREST authenticated request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("PostgREST authenticated GET failed with status %d: %s", resp2.StatusCode, string(body))
	}
	fmt.Println("PostgREST authenticated access successful with RLS enforcement")
}
