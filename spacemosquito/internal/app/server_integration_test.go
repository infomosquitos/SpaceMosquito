//go:build integration

package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vkh/spacemosquito/internal/app"
	"github.com/vkh/spacemosquito/internal/config"
	"github.com/vkh/spacemosquito/internal/paths"
	"github.com/vkh/spacemosquito/internal/search"
	"github.com/vkh/spacemosquito/internal/store"
	"github.com/vkh/spacemosquito/internal/testutil"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	paths.SetDataDir(dir)
	t.Cleanup(func() { paths.SetDataDir("") })

	if err := paths.EnsureLayout(dir); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			Path:   "spacemosquito.db",
		},
		Storage: config.StorageConfig{
			BasePath: filepath.Join(dir, "saved"),
		},
		Session: config.SessionConfig{
			EncryptionKey: "test-key-32-bytes-minimum-padded!!",
			FilePath:      filepath.Join(dir, "session.enc"),
		},
		MCP: config.MCPConfig{
			Host:    "127.0.0.1",
			Port:    56403,
			Timeout: 3600,
		},
		Cron: config.CronConfig{
			FullCrawl:   &config.CronJobConfig{Enabled: false},
			Incremental: &config.CronJobConfig{Enabled: false},
		},
	}
	if err := paths.NormalizeConfig(cfg); err != nil {
		t.Fatalf("NormalizeConfig: %v", err)
	}
	return cfg
}

func bootSeededServer(t *testing.T) (*app.TestServer, *testutil.SeedData) {
	t.Helper()
	ts := app.NewTestServer(t, testConfig(t))
	seed, err := testutil.SeedFixtures(context.Background(), ts.Store)
	if err != nil {
		t.Fatalf("SeedFixtures: %v", err)
	}
	return ts, seed
}

func TestServerBoot_SQLite(t *testing.T) {
	ts := app.NewTestServer(t, testConfig(t))

	if code, body := testutil.GETBody(t, ts.URL+"/health"); code != http.StatusOK {
		t.Fatalf("health status = %d", code)
	} else if string(body) != "ok" {
		t.Fatalf("health body = %q", body)
	}
}

func TestServerBoot_embeddedMigrations(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	ts := app.NewTestServer(t, testConfig(t))
	if code := testutil.GETJSON(t, nil, ts.URL+"/health", nil); code != http.StatusOK {
		t.Fatalf("health status = %d", code)
	}
}

func TestREST_Search_returnsSeededPage(t *testing.T) {
	ts, seed := bootSeededServer(t)

	var resp struct {
		Count   int                 `json:"count"`
		Results []search.SearchHit  `json:"results"`
	}
	url := ts.URL + "/api/search?q=" + seed.SearchTerm
	if code := testutil.GETJSON(t, nil, url, &resp); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if resp.Count < 1 {
		t.Fatalf("count = %d, want >= 1", resp.Count)
	}
	found := false
	for _, hit := range resp.Results {
		if hit.ConfluenceID == seed.SearchPageID && hit.SpaceKey == seed.SpaceKey {
			found = true
			if hit.Title != seed.PageTitles[seed.SearchPageID] {
				t.Errorf("title = %q", hit.Title)
			}
		}
	}
	if !found {
		t.Fatalf("results missing confluence_id %d: %+v", seed.SearchPageID, resp.Results)
	}
}

func TestREST_Search_spaceFilter(t *testing.T) {
	ts, seed := bootSeededServer(t)

	var resp struct {
		Results []search.SearchHit `json:"results"`
	}
	url := ts.URL + "/api/search?q=" + seed.SearchTerm + "&space_key=" + seed.SpaceKey
	if code := testutil.GETJSON(t, nil, url, &resp); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	for _, hit := range resp.Results {
		if hit.SpaceKey != seed.SpaceKey {
			t.Fatalf("unexpected space_key %q", hit.SpaceKey)
		}
	}
}

func TestREST_Search_unknownSpace(t *testing.T) {
	ts, seed := bootSeededServer(t)

	var resp struct {
		Count   int                `json:"count"`
		Results []search.SearchHit `json:"results"`
	}
	url := ts.URL + "/api/search?q=" + seed.SearchTerm + "&space_key=NOPE"
	if code := testutil.GETJSON(t, nil, url, &resp); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if resp.Count != 0 || len(resp.Results) != 0 {
		t.Fatalf("expected empty results, got %+v", resp)
	}
}

func TestREST_Stats_matchSeed(t *testing.T) {
	ts, _ := bootSeededServer(t)

	var stats struct {
		TotalPages  int `json:"TotalPages"`
		TotalSpaces int `json:"TotalSpaces"`
	}
	if code := testutil.GETJSON(t, nil, ts.URL+"/api/search/stats", &stats); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if stats.TotalSpaces < 1 {
		t.Errorf("total_spaces = %d", stats.TotalSpaces)
	}
	if stats.TotalPages < 3 {
		t.Errorf("total_pages = %d, want >= 3", stats.TotalPages)
	}
}

func TestREST_ListSpaces(t *testing.T) {
	ts, seed := bootSeededServer(t)

	var spaces []map[string]any
	if code := testutil.GETJSON(t, nil, ts.URL+"/api/spaces", &spaces); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	found := false
	for _, s := range spaces {
		if s["space_key"] == seed.SpaceKey {
			found = true
			if pages, ok := s["pages_crawled"].(float64); !ok || pages < 3 {
				t.Errorf("pages_crawled = %v", s["pages_crawled"])
			}
		}
	}
	if !found {
		t.Fatalf("space %q not in list: %+v", seed.SpaceKey, spaces)
	}
}

func TestREST_SpacePages(t *testing.T) {
	ts, seed := bootSeededServer(t)

	var result search.ListSpaceResult
	url := ts.URL + "/api/spaces/" + seed.SpaceKey + "/pages"
	if code := testutil.GETJSON(t, nil, url, &result); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if result.SpaceKey != seed.SpaceKey {
		t.Errorf("space_key = %q", result.SpaceKey)
	}
	if result.Count < 3 {
		t.Fatalf("count = %d, want >= 3", result.Count)
	}
	ids := map[int]bool{}
	for _, p := range result.Pages {
		ids[p.ConfluenceID] = true
	}
	for _, id := range seed.PageIDs {
		if !ids[id] {
			t.Errorf("missing page id %d", id)
		}
	}
}

func TestREST_SpacePages_pagination(t *testing.T) {
	ts, seed := bootSeededServer(t)

	var page1 search.ListSpaceResult
	url := ts.URL + "/api/spaces/" + seed.SpaceKey + "/pages?limit=2"
	if code := testutil.GETJSON(t, nil, url, &page1); code != http.StatusOK {
		t.Fatalf("page1 status = %d", code)
	}
	if page1.Count != 2 || !page1.HasMore || page1.NextAfterConfluenceID == nil {
		t.Fatalf("page1 = %+v", page1)
	}

	var page2 search.ListSpaceResult
	url = ts.URL + "/api/spaces/" + seed.SpaceKey + "/pages?limit=2&after_confluence_id=" + strconv.Itoa(*page1.NextAfterConfluenceID)
	if code := testutil.GETJSON(t, nil, url, &page2); code != http.StatusOK {
		t.Fatalf("page2 status = %d", code)
	}
	if page2.Count < 1 {
		t.Fatalf("page2 count = %d", page2.Count)
	}
	if page2.Pages[0].ConfluenceID <= *page1.NextAfterConfluenceID {
		t.Fatalf("cursor not exclusive: %+v", page2.Pages[0])
	}
}

func TestREST_SpaceByKey(t *testing.T) {
	ts, seed := bootSeededServer(t)

	var space map[string]any
	url := ts.URL + "/api/spaces/" + seed.SpaceKey
	if code := testutil.GETJSON(t, nil, url, &space); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if space["space_key"] != seed.SpaceKey {
		t.Errorf("space_key = %v", space["space_key"])
	}
	if pages, ok := space["pages_crawled"].(float64); !ok || pages < 3 {
		t.Errorf("pages_crawled = %v", space["pages_crawled"])
	}
}

func TestMCP_InitializeAndToolsList(t *testing.T) {
	ts, _ := bootSeededServer(t)
	client := testutil.ConnectMCP(t, ts.URL)
	defer client.Close()

	initResp := client.Call(t, "initialize", map[string]any{}, 1)
	if initResp.Error != nil {
		t.Fatalf("initialize error: %+v", initResp.Error)
	}

	listResp := client.Call(t, "tools/list", map[string]any{}, 2)
	if listResp.Error != nil {
		t.Fatalf("tools/list error: %+v", listResp.Error)
	}
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(listResp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 4 {
		t.Fatalf("tool count = %d", len(result.Tools))
	}
}

func TestMCP_ListSpaces(t *testing.T) {
	ts, seed := bootSeededServer(t)
	client := testutil.ConnectMCP(t, ts.URL)
	defer client.Close()
	client.Call(t, "initialize", map[string]any{}, 1)

	var spaces []struct {
		Key string `json:"key"`
	}
	client.ToolCall(t, "confluence_list_spaces", map[string]any{}, 3, &spaces)
	found := false
	for _, s := range spaces {
		if s.Key == seed.SpaceKey {
			found = true
		}
	}
	if !found {
		t.Fatalf("space %q not found in %+v", seed.SpaceKey, spaces)
	}
}

func TestMCP_ListSpace(t *testing.T) {
	ts, seed := bootSeededServer(t)
	client := testutil.ConnectMCP(t, ts.URL)
	defer client.Close()
	client.Call(t, "initialize", map[string]any{}, 1)

	var result search.ListSpaceResult
	client.ToolCall(t, "confluence_list_space", map[string]any{
		"space_key": seed.SpaceKey,
		"limit":     10,
	}, 4, &result)
	if result.Count < 3 {
		t.Fatalf("count = %d", result.Count)
	}
}

func TestMCP_Search(t *testing.T) {
	ts, seed := bootSeededServer(t)
	client := testutil.ConnectMCP(t, ts.URL)
	defer client.Close()
	client.Call(t, "initialize", map[string]any{}, 1)

	var hits []search.SearchHit
	client.ToolCall(t, "confluence_search", map[string]any{
		"query": seed.SearchTerm,
	}, 5, &hits)
	found := false
	for _, h := range hits {
		if h.ConfluenceID == seed.SearchPageID {
			found = true
		}
	}
	if !found {
		t.Fatalf("search missing id %d: %+v", seed.SearchPageID, hits)
	}
}

func TestMCP_GetPage(t *testing.T) {
	ts, seed := bootSeededServer(t)
	client := testutil.ConnectMCP(t, ts.URL)
	defer client.Close()
	client.Call(t, "initialize", map[string]any{}, 1)

	var page search.PageDetail
	client.ToolCall(t, "confluence_get_page", map[string]any{
		"space_key":     seed.SpaceKey,
		"confluence_id": float64(seed.SearchPageID),
	}, 6, &page)
	if page.ConfluenceID != seed.SearchPageID {
		t.Errorf("confluence_id = %d", page.ConfluenceID)
	}
	if page.Title != seed.PageTitles[seed.SearchPageID] {
		t.Errorf("title = %q", page.Title)
	}
	if !strings.Contains(page.Content, seed.SearchTerm) {
		t.Errorf("content = %q", page.Content)
	}
}

func TestMCP_GetPage_notFound(t *testing.T) {
	ts, seed := bootSeededServer(t)
	client := testutil.ConnectMCP(t, ts.URL)
	defer client.Close()
	client.Call(t, "initialize", map[string]any{}, 1)

	resp := client.ToolCall(t, "confluence_get_page", map[string]any{
		"space_key":     seed.SpaceKey,
		"confluence_id": float64(99999),
	}, 7, nil)
	var envelope struct {
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.IsError {
		t.Fatalf("expected tool error, got %s", string(resp.Result))
	}
}

func TestREST_GetPage_byConfluenceID(t *testing.T) {
	ts, seed := bootSeededServer(t)

	var page search.PageDetail
	url := ts.URL + "/api/pages/" + strconv.Itoa(seed.SearchPageID)
	if code := testutil.GETJSON(t, nil, url, &page); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if page.ConfluenceID != seed.SearchPageID || page.SpaceKey != seed.SpaceKey {
		t.Fatalf("page = %+v", page)
	}
}

func TestMCP_GetPage_withoutSpaceKey(t *testing.T) {
	ts, seed := bootSeededServer(t)
	client := testutil.ConnectMCP(t, ts.URL)
	defer client.Close()
	client.Call(t, "initialize", map[string]any{}, 1)

	var page search.PageDetail
	client.ToolCall(t, "confluence_get_page", map[string]any{
		"confluence_id": float64(seed.SearchPageID),
	}, 8, &page)
	if page.ConfluenceID != seed.SearchPageID {
		t.Errorf("confluence_id = %d", page.ConfluenceID)
	}
}

func TestREST_GetPage_ambiguous(t *testing.T) {
	ts := app.NewTestServer(t, testConfig(t))
	ctx := context.Background()
	id1, _ := ts.Store.CreateSpace(ctx, "AAA", "A", "https://example/spaces/AAA")
	id2, _ := ts.Store.CreateSpace(ctx, "BBB", "B", "https://example/spaces/BBB")
	_ = ts.Store.UpsertPage(ctx, &store.Page{SpaceID: id1, ConfluenceID: 77, Title: "A page", Content: "a"})
	_ = ts.Store.UpsertPage(ctx, &store.Page{SpaceID: id2, ConfluenceID: 77, Title: "B page", Content: "b"})

	code, body := testutil.GETBody(t, ts.URL+"/api/pages/77")
	if code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", code, body)
	}
	if !strings.Contains(string(body), "AAA") || !strings.Contains(string(body), "BBB") {
		t.Fatalf("body = %s", body)
	}
}

func TestMCP_GETMethodNotAllowed(t *testing.T) {
	ts, _ := bootSeededServer(t)

	resp, err := http.Get(ts.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}
