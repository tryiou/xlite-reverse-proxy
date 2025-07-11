// test script to evaluate dynamic servers  ADD/REMOVE.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
)

var (
	testMu      sync.Mutex
	testServer  *httptest.Server
	testResults = struct {
		sync.Mutex
		results map[string]bool
	}{results: make(map[string]bool)}
)

// Initialize global servers for tests
var servers = &Servers{}

// Record test result and log immediately
func recordTestResult(testName string, fail bool) {
	testResults.Lock()
	defer testResults.Unlock()
	testResults.results[testName] = !fail
	status := "PASS"
	if fail {
		status = "FAIL"
	}
	log.Printf("TEST_UNIT: RESULTS: [%s] %s", testName, status)
}

// Print final test summary after all tests are completed
func PrintFinalSummary() {
	testResults.Lock()
	defer testResults.Unlock()

	log.Println("TEST_UNIT: ========== FINAL TEST SUMMARY ==========")
	passed, failed := 0, 0
	for test, success := range testResults.results {
		if success {
			log.Printf("TEST_UNIT: PASS: %s", test)
			passed++
		} else {
			log.Printf("TEST_UNIT: FAIL: %s", test)
			failed++
		}
	}
	log.Printf("TEST_UNIT: ---")
	log.Printf("TEST_UNIT: %d passed, %d failed", passed, failed)
	log.Println("TEST_UNIT: ========================================")
}

// TestMain to facilitate summary and exit
func TestMain(m *testing.M) {
	// Run all tests
	code := m.Run()
	PrintFinalSummary()
	os.Exit(code)
}

func TestServerIDConsistency(t *testing.T) {
	defer recordTestResult("TestServerIDConsistency", t.Failed())
	log.Print("TEST_UNIT: Starting enhanced server ID consistency test")
	testServer = createMockProvider([]string{
		"http://server1:80",
		"http://server2:80",
		"http://server3:80",
		"http://server4:80",
	})
	defer testServer.Close()

	config = &Config{
		DynlistServersProviders: []string{testServer.URL},
		RateLimit:               10,
		HttpTimeout:             5,
		MaxStoredBlocks:         5,
		ConsensusThreshold:      0.66,
		AcceptedMethods:         []string{"heights", "fees"},
	}

	servers = &Servers{}

	// Phase 1: Initial population with 5 servers (4 remotes + provider)
	t.Run("BulkAddition", func(t *testing.T) {
		log.Print("TEST_UNIT: --- Phase1: Initial bulk addition (5 servers) ---")
		updateServersFromProviders(servers)
		logServerIDs("After initial population")
		verifyUniqueIDs(t)
		verifyIDAssignments(t, []string{
			testServer.URL,
			"http://server1:80",
			"http://server2:80",
			"http://server3:80",
			"http://server4:80",
		})
	})

	// Phase 2: Remove 3 servers and add 2 new
	t.Run("BulkRemovalAndAddition", func(t *testing.T) {
		log.Print("TEST_UNIT: --- Phase2: Bulk removal and addition ---")
		testServer.Config.Handler.(*mockHandler).RefreshRoutes(toJsonElements([]string{
			"http://server1:80",
			"http://server4:80",
			"http://server5:80",
			"http://server6:80",
		}))
		updateServersFromProviders(servers)
		logServerIDs("After bulk update")
		verifyUniqueIDs(t)
		verifyServerAbsence(t, []string{"http://server2:80", "http://server3:80"})
	})

	// Phase 3: Re-add removed servers and new ones
	t.Run("ReAddMixedServers", func(t *testing.T) {
		log.Print("TEST_UNIT: --- Phase3: Re-add removed and new servers ---")
		testServer.Config.Handler.(*mockHandler).RefreshRoutes(toJsonElements([]string{
			"http://server1:80",
			"http://server2:80",
			"http://server3:80",
			"http://server5:80",
			"http://server7:80",
			"http://server8:80",
		}))
		updateServersFromProviders(servers)
		logServerIDs("After re-adding mixed servers")
		verifyUniqueIDs(t)
		verifyReaddedIDs(t, []idUrlPair{
			{id: 3, url: "http://server2:80"},
			{id: 4, url: "http://server3:80"},
		})
	})

	// Phase 4: Test duplicate URL handling
	t.Run("DuplicateURLHandling", func(t *testing.T) {
		log.Print("TEST_UNIT: --- Phase4: Duplicate URL handling ---")
		// Record IDs before duplicate test
		preDuplicateIDForServer1 := getServerID("http://server1:80")
		testServer.Config.Handler.(*mockHandler).RefreshRoutes(toJsonElements([]string{
			"http://server1:80",
			"http://server1:80", // Intentional duplicate
			"http://server2:80",
		}))
		updateServersFromProviders(servers)
		logServerIDs("After duplicate URL test")
		verifyUniqueServers(t, []string{testServer.URL, "http://server1:80", "http://server2:80"})
		verifyIDConsistency(t, "http://server1:80", preDuplicateIDForServer1)
	})
	log.Print("TEST_UNIT: Enhanced test completed successfully!")
}

// --- Infrastructure Helpers ---
type idUrlPair struct {
	id  int
	url string
}

type mockHandler struct {
	mu      sync.Mutex
	servers []JsonElement
}

func (h *mockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	json.NewEncoder(w).Encode(JsonResponse{
		Result: string(marshalServers(h.servers)),
	})
}

func (h *mockHandler) RefreshRoutes(servers []JsonElement) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.servers = servers
}

func createMockProvider(initialServers []string) *httptest.Server {
	return httptest.NewServer(&mockHandler{
		servers: toJsonElements(initialServers),
	})
}

func toJsonElements(urls []string) []JsonElement {
	elements := make([]JsonElement, len(urls))
	for i, url := range urls {
		elements[i] = JsonElement{
			NodePubKey: fmt.Sprintf("pubkey-%d", i+1),
			Config:     makeMockConfig(url),
		}
	}
	return elements
}

func makeMockConfig(fullURL string) string {
	u, _ := url.Parse(fullURL)
	return fmt.Sprintf(`[Main]                                                                                                                                                   
host=%s                                                                                                                                                                          
port=%s                                                                                                                                                                          
plugins=heights,fees`, u.Hostname(), u.Port())
}

// --- Test Helpers ---
func logServerIDs(title string) {
	testMu.Lock()
	defer testMu.Unlock()

	log.Printf("TEST_UNIT: --- LOG %s ---", title)
	log.Print("TEST_UNIT: Servers count:", len(servers.Slice))
	log.Print("TEST_UNIT: Current server list:")
	for i, s := range servers.Slice {
		log.Printf("TEST_UNIT:  %d. ID=%d URL=%s", i+1, s.id, s.url)
	}
	log.Printf("TEST_UNIT: --- END %s ---", title)
}

func verifyUniqueIDs(t *testing.T) {
	testMu.Lock()
	defer testMu.Unlock()

	seen := make(map[int]bool)
	for _, s := range servers.Slice {
		if seen[s.id] {
			t.Fatalf("Duplicate ID detected: %d (%s)", s.id, s.url)
		}
		seen[s.id] = true
	}
}

func verifyIDAssignments(t *testing.T, expectedUrls []string) {
	testMu.Lock()
	defer testMu.Unlock()

	if len(servers.Slice) != len(expectedUrls) {
		t.Errorf("Expected %d servers, got %d", len(expectedUrls), len(servers.Slice))
	}

	urlSet := make(map[string]bool)
	for _, url := range expectedUrls {
		urlSet[url] = true
	}

	for _, server := range servers.Slice {
		if !urlSet[server.url] {
			t.Errorf("Unexpected server URL: %s", server.url)
		}
	}
}

func verifyServerAbsence(t *testing.T, absentUrls []string) {
	testMu.Lock()
	defer testMu.Unlock()

	urlSet := make(map[string]bool)
	for _, url := range absentUrls {
		urlSet[url] = true
	}

	for _, server := range servers.Slice {
		if urlSet[server.url] {
			t.Errorf("Server %s should be absent but is present (ID %d)", server.url, server.id)
		}
	}
}

func verifyReaddedIDs(t *testing.T, expectedPairs []idUrlPair) {
	testMu.Lock()
	defer testMu.Unlock()

	pairMap := make(map[string]int)
	for _, pair := range expectedPairs {
		pairMap[pair.url] = pair.id
	}

	for _, server := range servers.Slice {
		if expectedID, exists := pairMap[server.url]; exists {
			if server.id != expectedID {
				t.Errorf("Server %s got ID %d (expected %d)", server.url, server.id, expectedID)
			}
		}
	}
}

func verifyUniqueServers(t *testing.T, expectedUrls []string) {
	testMu.Lock()
	defer testMu.Unlock()

	// Build set of expected URLs
	expectedSet := make(map[string]bool)
	for _, url := range expectedUrls {
		expectedSet[url] = true
	}

	// Verify all live servers are in expected set
	urlCount := make(map[string]int)
	for _, server := range servers.Slice {
		urlCount[server.url]++
		if !expectedSet[server.url] {
			t.Errorf("Unexpected server URL: %s", server.url)
		}
	}

	// Verify no duplicates in live servers
	for url, count := range urlCount {
		if count > 1 {
			t.Errorf("Duplicate server URL in live servers: %s (found %d times)", url, count)
		}
	}
}

func verifyIDConsistency(t *testing.T, url string, expectedID int) {
	testMu.Lock()
	defer testMu.Unlock()

	for _, server := range servers.Slice {
		if server.url == url {
			if server.id != expectedID {
				t.Errorf("ID changed for %s: got %d (expected %d)", url, server.id, expectedID)
			}
			return
		}
	}
	t.Errorf("Server %s not found", url)
}

func getServerID(url string) int {
	testMu.Lock()
	defer testMu.Unlock()

	for _, server := range servers.Slice {
		if server.url == url {
			return server.id
		}
	}
	return -1
}

func marshalServers(servers []JsonElement) []byte {
	data, _ := json.Marshal(servers)
	return data
}
