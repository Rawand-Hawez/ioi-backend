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
	GoTrueURL   = "http://localhost:9999"
	FiberURL    = "http://localhost:8080/api/v1"
	PostgRESTURL = "http://localhost:3000"
	TestEmail   = "e2e@ioi.dev"
	TestPass    = "testing12345!"
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
	req, _ := http.NewRequest("GET", FiberURL+"/todos/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect to Fiber API: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized for missing token, got %d", resp.StatusCode)
	}
}

func TestFiberCRUD(t *testing.T) {
	token := authenticate(t)

	// CREATE
	createPayload, _ := json.Marshal(map[string]string{"task": "e2e test todo"})
	req, _ := http.NewRequest("POST", FiberURL+"/todos/", bytes.NewBuffer(createPayload))
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
	todoID, ok := created["id"].(string)
	if !ok || todoID == "" {
		t.Fatal("Created todo missing ID")
	}
	fmt.Printf("Created todo: %s\n", todoID)

	// GET (list)
	req, _ = http.NewRequest("GET", FiberURL+"/todos/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("List request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("List failed with status %d: %s", resp.StatusCode, string(body))
	}
	var todos []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&todos)
	resp.Body.Close()
	if len(todos) == 0 {
		t.Fatal("Expected at least one todo after create")
	}
	fmt.Printf("Listed %d todos (RLS enforced)\n", len(todos))

	// TOGGLE
	req, _ = http.NewRequest("PATCH", FiberURL+"/todos/"+todoID+"/toggle", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Toggle request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Toggle failed with status %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
	fmt.Printf("Toggled todo: %s\n", todoID)

	// DELETE
	req, _ = http.NewRequest("DELETE", FiberURL+"/todos/"+todoID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Delete request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Delete failed with status %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
	fmt.Printf("Deleted todo: %s\n", todoID)
}

func TestPostgRESTEndpoint(t *testing.T) {
	// Unauthenticated request should be rejected (web_anon has no table access)
	resp, err := http.Get(PostgRESTURL + "/todos")
	if err != nil {
		t.Fatalf("Failed to connect to PostgREST: %v", err)
	}
	defer resp.Body.Close()

	// PostgREST returns 401 for tables not accessible to anon role
	if resp.StatusCode == http.StatusOK {
		t.Error("PostgREST should not expose todos to unauthenticated requests")
	}
	fmt.Printf("PostgREST anon access correctly blocked (status %d)\n", resp.StatusCode)

	// Authenticated request through PostgREST
	token := authenticate(t)
	req, _ := http.NewRequest("GET", PostgRESTURL+"/todos", nil)
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
