package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/yukimochi/Activity-Relay/models"
)

func resetAdminTestState(t *testing.T) {
	t.Helper()
	RelayState.RedisClient.FlushAll(context.TODO()).Result()
	RelayState = models.NewState(RelayState.RedisClient, false)
	viper.Set("ADMIN_USERNAME", "admin")
	viper.Set("ADMIN_PASSWORD", "secret")
	viper.Set("ADMIN_ENABLED", true)
}

func TestBuildServerRowsIncludesSubscribersFollowersAndDomainState(t *testing.T) {
	resetAdminTestState(t)
	RelayState.AddSubscriber(models.Subscriber{Domain: "mastodon.example", InboxURL: "https://mastodon.example/inbox"})
	RelayState.AddFollower(models.Follower{Domain: "pleroma.example", InboxURL: "https://pleroma.example/inbox", MutuallyFollow: true})
	RelayState.SetLimitedDomain("mastodon.example", true)
	RelayState.SetBlockedDomain("blocked.example", true)

	rows := buildServerRows()
	if len(rows) != 3 {
		t.Fatalf("Expected 3 rows, got %d: %+v", len(rows), rows)
	}

	byDomain := map[string]ServerRow{}
	for _, row := range rows {
		byDomain[row.Domain] = row
	}
	if byDomain["mastodon.example"].Status != "limited" || byDomain["mastodon.example"].Kind != "subscriber" {
		t.Fatalf("Expected mastodon.example to be limited subscriber, got %+v", byDomain["mastodon.example"])
	}
	if byDomain["pleroma.example"].Status != "active" || byDomain["pleroma.example"].Kind != "follower" {
		t.Fatalf("Expected pleroma.example to be active follower, got %+v", byDomain["pleroma.example"])
	}
	if byDomain["blocked.example"].Status != "blocked" {
		t.Fatalf("Expected blocked.example to be blocked-only row, got %+v", byDomain["blocked.example"])
	}
}

func TestHandleAPIServersReturnsJSONList(t *testing.T) {
	resetAdminTestState(t)
	RelayState.AddSubscriber(models.Subscriber{Domain: "mastodon.example", InboxURL: "https://mastodon.example/inbox"})

	req := httptest.NewRequest("GET", "/api/servers", nil)
	w := httptest.NewRecorder()
	handleAPIServers(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Expected application/json content type, got %q", ct)
	}
	var payload ServerListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Expected valid JSON response, got %v", err)
	}
	if payload.Total != 1 || payload.Servers[0].Domain != "mastodon.example" {
		t.Fatalf("Unexpected server list payload: %+v", payload)
	}
}

func TestAdminDisabledRejectsMutation(t *testing.T) {
	resetAdminTestState(t)
	viper.Set("ADMIN_ENABLED", false)
	RelayState.AddSubscriber(models.Subscriber{Domain: "mastodon.example", InboxURL: "https://mastodon.example/inbox"})

	req := httptest.NewRequest("POST", "/admin/servers/mastodon.example/delete", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	w := httptest.NewRecorder()
	handleAdminDeleteServer(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected 404 when admin is disabled, got %d", w.Code)
	}
	if RelayState.SelectSubscriber("mastodon.example") == nil {
		t.Fatalf("Disabled admin request deleted subscriber")
	}
}

func TestAdminDeleteRequiresAuthentication(t *testing.T) {
	resetAdminTestState(t)
	RelayState.AddSubscriber(models.Subscriber{Domain: "mastodon.example", InboxURL: "https://mastodon.example/inbox"})

	req := httptest.NewRequest("POST", "/admin/servers/mastodon.example/delete", nil)
	w := httptest.NewRecorder()
	handleAdminDeleteServer(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for unauthenticated admin mutation, got %d", w.Code)
	}
	if RelayState.SelectSubscriber("mastodon.example") == nil {
		t.Fatalf("Unauthenticated request deleted subscriber")
	}
}

func TestAdminMutationRejectsCrossOriginPost(t *testing.T) {
	resetAdminTestState(t)
	RelayState.AddSubscriber(models.Subscriber{Domain: "mastodon.example", InboxURL: "https://mastodon.example/inbox"})

	req := httptest.NewRequest("POST", "https://relay.example/admin/servers/mastodon.example/delete", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	handleAdminDeleteServer(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 for cross-origin admin mutation, got %d", w.Code)
	}
	if RelayState.SelectSubscriber("mastodon.example") == nil {
		t.Fatalf("Cross-origin request deleted subscriber")
	}
}

func TestAdminDeleteRemovesSubscriberWhenAuthenticated(t *testing.T) {
	resetAdminTestState(t)
	RelayState.AddSubscriber(models.Subscriber{Domain: "mastodon.example", InboxURL: "https://mastodon.example/inbox"})

	req := httptest.NewRequest("POST", "/admin/servers/mastodon.example/delete", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	w := httptest.NewRecorder()
	handleAdminDeleteServer(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("Expected 303 after authenticated delete, got %d body=%s", w.Code, w.Body.String())
	}
	if RelayState.SelectSubscriber("mastodon.example") != nil {
		t.Fatalf("Expected subscriber to be deleted")
	}
}

func TestAdminBlockRemovesActiveMembershipAndAddsBlockedDomain(t *testing.T) {
	resetAdminTestState(t)
	RelayState.AddSubscriber(models.Subscriber{Domain: "mastodon.example", InboxURL: "https://mastodon.example/inbox"})

	req := httptest.NewRequest("POST", "/admin/servers/mastodon.example/block", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	w := httptest.NewRecorder()
	handleAdminBlockServer(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("Expected 303 after authenticated block, got %d", w.Code)
	}
	if RelayState.SelectSubscriber("mastodon.example") != nil {
		t.Fatalf("Expected subscriber to be removed when blocked")
	}
	found := false
	for _, domain := range RelayState.BlockedDomains {
		if domain == "mastodon.example" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Expected mastodon.example to be in blocked domains: %+v", RelayState.BlockedDomains)
	}
}

func TestHandlePublicIndexShowsRelayUsage(t *testing.T) {
	resetAdminTestState(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handlePublicIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "https://"+GlobalConfig.ServerHostname().Host+"/inbox") {
		t.Fatalf("Expected public index to include relay inbox URL, got %s", body)
	}
}
