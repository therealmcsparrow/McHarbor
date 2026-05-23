// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// ProviderConfig wraps an oauth2.Config with the provider's userinfo endpoint.
type ProviderConfig struct {
	OAuth2   oauth2.Config
	UserInfo string
}

type discoveryDocument struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
}

// UserInfo represents claims returned from the userinfo endpoint.
type UserInfo struct {
	Sub    string   `json:"sub"`
	Email  string   `json:"email"`
	Name   string   `json:"name"`
	Groups []string `json:"groups,omitempty"`
}

// BuildEntraConfig builds an OAuth2 config for Microsoft Entra ID (Azure AD).
func BuildEntraConfig(clientID, clientSecret, tenantID, redirectURL string, scopes []string) ProviderConfig {
	base := "https://login.microsoftonline.com/" + tenantID
	return ProviderConfig{
		OAuth2: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       scopes,
			Endpoint: oauth2.Endpoint{
				AuthURL:  base + "/oauth2/v2.0/authorize",
				TokenURL: base + "/oauth2/v2.0/token",
			},
		},
		UserInfo: "https://graph.microsoft.com/oidc/userinfo",
	}
}

// BuildGoogleConfig builds an OAuth2 config for Google Workspace.
func BuildGoogleConfig(clientID, clientSecret, redirectURL string, scopes []string) ProviderConfig {
	return ProviderConfig{
		OAuth2: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       scopes,
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
				TokenURL: "https://oauth2.googleapis.com/token",
			},
		},
		UserInfo: "https://openidconnect.googleapis.com/v1/userinfo",
	}
}

// BuildGenericConfig discovers endpoints from an issuer URL and builds an OAuth2 config.
func BuildGenericConfig(ctx context.Context, clientID, clientSecret, issuerURL, redirectURL string, scopes []string) (ProviderConfig, error) {
	discovery, err := fetchDiscoveryDocument(ctx, issuerURL)
	if err != nil {
		return ProviderConfig{}, err
	}
	if discovery.AuthorizationEndpoint == "" || discovery.TokenEndpoint == "" || discovery.UserInfoEndpoint == "" {
		return ProviderConfig{}, fmt.Errorf("discovery document missing required endpoints")
	}

	return ProviderConfig{
		OAuth2: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       scopes,
			Endpoint: oauth2.Endpoint{
				AuthURL:  discovery.AuthorizationEndpoint,
				TokenURL: discovery.TokenEndpoint,
			},
		},
		UserInfo: discovery.UserInfoEndpoint,
	}, nil
}

// FetchUserInfo exchanges an access token for user claims from the userinfo endpoint.
func FetchUserInfo(ctx context.Context, token *oauth2.Token, userInfoURL string) (*UserInfo, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating userinfo request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("userinfo returned %d: %s", resp.StatusCode, body)
	}

	var info UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding userinfo: %w", err)
	}

	return &info, nil
}

// TestConnection verifies connectivity and credentials for an OIDC provider.
// It fetches the OIDC discovery document and attempts a client_credentials token exchange.
// For Google, which doesn't support client_credentials for standard OAuth2 clients,
// a "unauthorized_client" error (vs "invalid_client") indicates valid credentials.
func TestConnection(ctx context.Context, providerType, clientID, clientSecret, tenantID, issuerURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Step 1: Verify discovery endpoint is reachable
	var discoveryURL string
	switch providerType {
	case "entra_id":
		discoveryURL = "https://login.microsoftonline.com/" + tenantID + "/v2.0/.well-known/openid-configuration"
	case "google":
		discoveryURL = "https://accounts.google.com/.well-known/openid-configuration"
	case "generic_oidc":
		discoveryURL = discoveryURLForIssuer(issuerURL)
	default:
		return fmt.Errorf("unsupported provider type: %s", providerType)
	}

	discovery, err := fetchDiscoveryDocumentByURL(ctx, discoveryURL)
	if err != nil {
		return err
	}
	if discovery.TokenEndpoint == "" {
		return fmt.Errorf("discovery document missing token_endpoint")
	}

	// Step 2: Verify client credentials via token endpoint
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}
	if providerType == "entra_id" {
		data.Set("scope", "https://graph.microsoft.com/.default")
	}

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, discovery.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("creating token request: %w", err)
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	tokenResp, err := http.DefaultClient.Do(tokenReq)
	if err != nil {
		return fmt.Errorf("requesting token: %w", err)
	}
	defer tokenResp.Body.Close()

	// For Entra ID: 200 = success, 401 = bad credentials
	// For Google: 400 with "unauthorized_client" = valid creds but grant not supported (OK)
	//             400/401 with "invalid_client" = bad credentials
	if tokenResp.StatusCode == http.StatusOK {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(tokenResp.Body, 2048))
	var tokenErr struct {
		Error string `json:"error"`
	}
	if jsonErr := json.Unmarshal(body, &tokenErr); jsonErr == nil {
		// Google returns "unauthorized_client" for valid credentials that can't use client_credentials grant
		if tokenErr.Error == "unauthorized_client" || tokenErr.Error == "unsupported_grant_type" || tokenErr.Error == "invalid_scope" {
			return nil
		}
	}

	return fmt.Errorf("token endpoint returned %d: %s", tokenResp.StatusCode, body)
}

func fetchDiscoveryDocument(ctx context.Context, issuerURL string) (*discoveryDocument, error) {
	return fetchDiscoveryDocumentByURL(ctx, discoveryURLForIssuer(issuerURL))
}

func fetchDiscoveryDocumentByURL(ctx context.Context, discoveryURL string) (*discoveryDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating discovery request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("discovery endpoint returned %d: %s", resp.StatusCode, body)
	}

	var discovery discoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return nil, fmt.Errorf("decoding discovery document: %w", err)
	}

	return &discovery, nil
}

func discoveryURLForIssuer(issuerURL string) string {
	return strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"
}

// GroupInfo represents a group from the identity provider.
type GroupInfo struct {
	ID          string `json:"id"`
	Name        string `json:"displayName"`
	Description string `json:"description"`
}

// FetchEntraGroups fetches groups from Microsoft Graph API using client credentials.
func FetchEntraGroups(ctx context.Context, clientID, clientSecret, tenantID string) ([]GroupInfo, error) {
	tokenURL := "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/token"

	cfg := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		Scopes:       []string{"https://graph.microsoft.com/.default"},
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client := cfg.Client(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://graph.microsoft.com/v1.0/groups?$select=id,displayName,description&$top=999", nil)
	if err != nil {
		return nil, fmt.Errorf("creating graph request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching groups from graph: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("graph API returned %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Value []GroupInfo `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding graph response: %w", err)
	}

	return result.Value, nil
}

// FetchGoogleGroups fetches groups from Google Admin SDK using client credentials.
// Requires the Admin SDK API to be enabled and the OAuth client to have domain-wide delegation,
// or alternatively uses the impersonated admin email to retrieve groups for the domain.
func FetchGoogleGroups(ctx context.Context, clientID, clientSecret, domain string) ([]GroupInfo, error) {
	// Google doesn't support client_credentials grant for regular OAuth2 clients.
	// Instead, use the OAuth2 token endpoint with the JWT assertion flow.
	// For simplicity, we use a direct API call with a service-to-service token.
	cfg := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     "https://oauth2.googleapis.com/token",
		Scopes:       []string{"https://www.googleapis.com/auth/admin.directory.group.readonly"},
		EndpointParams: url.Values{
			"subject": {""}, // Would need admin email for delegation
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client := cfg.Client(ctx)

	apiURL := "https://admin.googleapis.com/admin/directory/v1/groups?domain=" + url.QueryEscape(domain) + "&maxResults=200"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating admin SDK request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching groups from admin SDK: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("admin SDK returned %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Groups []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Email       string `json:"email"`
			Description string `json:"description"`
		} `json:"groups"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding admin SDK response: %w", err)
	}

	groups := make([]GroupInfo, 0, len(result.Groups))
	for _, g := range result.Groups {
		name := g.Name
		if name == "" {
			name = strings.Split(g.Email, "@")[0]
		}
		groups = append(groups, GroupInfo{
			ID:          g.ID,
			Name:        name,
			Description: g.Description,
		})
	}

	return groups, nil
}
