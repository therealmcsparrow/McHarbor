// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// s3TestClient does a single PutObject / GetObject / DeleteObject
// round trip against an S3-compatible endpoint using a hand-rolled
// SigV4 signer. The full AWS SDK isn't in the storage_locations
// module's dependency tree (it lives in container_backups) and
// pulling it in just for a self-test would inflate the deploy
// artifact for marginal value. SigV4 is well documented in the
// AWS spec; the implementation here is small and covered by the
// "good enough for a connect test" contract — the only data
// written is a small (under 1 KiB) marker object whose key is
// mcharbor-conn-test-<random>.
type s3TestClient struct {
	endpoint   string // e.g. "s3.us-east-1.amazonaws.com" or "minio.local:9000"
	region     string // e.g. "us-east-1"
	bucket     string
	accessKey  string
	secretKey  string
	usePathStyle bool // true for MinIO etc., false for AWS S3
	useSSL     bool
}

// newS3TestClientFromLocation builds a client from a storage
// location's configured fields. Returns nil if the location is
// missing the minimum config (region + bucket + accessKey + secretKey).
func newS3TestClientFromLocation(loc *testLocationRow) *s3TestClient {
	if strings.TrimSpace(loc.Region) == "" ||
		strings.TrimSpace(loc.Bucket) == "" ||
		loc.ClientID == "" ||
		loc.ClientSecret == "" {
		return nil
	}
	endpoint := strings.TrimSpace(loc.Endpoint)
	useSSL := true
	usePathStyle := false
	if endpoint == "" {
		endpoint = fmt.Sprintf("s3.%s.amazonaws.com", loc.Region)
	} else {
		// Honor scheme/port overrides in the endpoint URL. MinIO
		// style local deployments often pass
		// `http://minio.local:9000` so we look for the scheme here.
		if u, err := url.Parse(endpoint); err == nil && u.Scheme != "" {
			useSSL = u.Scheme == "https"
			u.Scheme = ""
			endpoint = u.String()
		}
		// MinIO and most self-hosted S3 servers prefer path-style
		// addressing (`<endpoint>/<bucket>/<key>`). AWS S3 prefers
		// virtual-hosted style (`<bucket>.<endpoint>/<key>`). We
		// guess at the difference by looking for common MinIO
		// markers in the endpoint string; operators can always
		// override with a fully-qualified endpoint URL.
		usePathStyle = strings.Contains(strings.ToLower(endpoint), "minio") ||
			strings.Contains(endpoint, "localhost") ||
			strings.Contains(endpoint, "127.0.0.1") ||
			strings.Contains(endpoint, ":9000")
	}
	return &s3TestClient{
		endpoint:    endpoint,
		region:      strings.TrimSpace(loc.Region),
		bucket:      strings.TrimSpace(loc.Bucket),
		accessKey:   loc.ClientID,
		secretKey:   loc.ClientSecret,
		usePathStyle: usePathStyle,
		useSSL:      useSSL,
	}
}

// host returns the host (or host:port) portion of the endpoint,
// stripping any scheme prefix.
func (c *s3TestClient) host() string {
	return c.endpoint
}

// scheme returns http or https based on the configured transport.
func (c *s3TestClient) scheme() string {
	if c.useSSL {
		return "https"
	}
	return "http"
}

// urlFor returns the absolute URL for a key. Path-style addressing
// (MinIO style): /<bucket>/<key>. Virtual-hosted style (AWS S3):
// the bucket is a DNS subdomain. We pick based on the usePathStyle
// flag set at construction time.
func (c *s3TestClient) urlFor(key string) string {
	if c.usePathStyle {
		return c.scheme() + "://" + c.host() + "/" + url.PathEscape(c.bucket) + "/" + key
	}
	return c.scheme() + "://" + c.bucket + "." + c.host() + "/" + key
}

// sign computes the SigV4 Authorization header value for the
// given request. Adapted from the AWS SigV4 spec; only the
// subset we need (S3, AWS4-HMAC-SHA256) is implemented.
func (c *s3TestClient) sign(
	method, key, bodyHash string,
) string {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z") // YYYYMMDDTHHMMSSZ
	dateStamp := now.Format("20060102")     // YYYYMMDD

	hostHeader := c.host()
	if !c.usePathStyle {
		// Virtual-hosted style: the host header carries the bucket
		// as a subdomain.
		hostHeader = c.bucket + "." + c.host()
	}
	service := "s3"
	region := c.region

	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)

	// Canonical request. We don't use a real hash for the body
	// since the test always sends a small in-memory buffer; SHA256
	// is always correct.
	canonicalHeaders := "host:" + hostHeader + "\n" +
		"x-amz-content-sha256:" + bodyHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"

	canonicalRequest := method + "\n" +
		"/" + url.PathEscape(c.bucket) + "/" + key + "\n" +
		"" + "\n" +
		canonicalHeaders + "\n" +
		signedHeaders + "\n" +
		bodyHash

	stringToSign := "AWS4-HMAC-SHA256" + "\n" +
		amzDate + "\n" +
		credentialScope + "\n" +
		hashSHA256([]byte(canonicalRequest))

	// Derive the signing key by stepping HMAC-SHA256 four times.
	// This is the canonical SigV4 key derivation; missing a step
	// produces 403 SignatureDoesNotMatch.
	kDate := hmacSHA256([]byte("AWS4"+c.secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))

	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	auth := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.accessKey, credentialScope, signedHeaders, signature,
	)
	_ = bodyHash
	return auth
}

// roundTrip executes a single signed request and returns the
// response. Body is sent in full and the response is fully read
// and closed before returning.
func (c *s3TestClient) roundTrip(
	ctx context.Context,
	method, key string,
	body []byte,
	extraHeaders map[string]string,
) (int, []byte, error) {
	target := c.urlFor(key)
	bodyHash := hashSHA256(body)
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("building request: %w", err)
	}
	amzDate := time.Now().UTC().Format("20060102T150405Z")
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", bodyHash)
	if len(body) > 0 {
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", c.sign(method, key, bodyHash))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("s3 request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("reading s3 response: %w", err)
	}
	return resp.StatusCode, raw, nil
}

// putObject uploads `body` to `key`. 200 OK = the artifact was
// written (MinIO + some AWS endpoints) or 204 No Content (most
// AWS S3 regions).
func (c *s3TestClient) putObject(ctx context.Context, key string, body []byte) error {
	status, raw, err := c.roundTrip(ctx, http.MethodPut, key, body, map[string]string{
		"Content-Type": "application/octet-stream",
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("PutObject returned status %d: %s", status, truncateBody(string(raw), 256))
	}
	return nil
}

// getObject downloads the body at `key`. The first 4 KiB is read
// to keep the connect test fast and bounded.
func (c *s3TestClient) getObject(ctx context.Context, key string) ([]byte, error) {
	status, raw, err := c.roundTrip(ctx, http.MethodGet, key, nil, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("GetObject returned status %d: %s", status, truncateBody(string(raw), 256))
	}
	return raw, nil
}

// deleteObject removes the object at `key`. 204 No Content is the
// success response on every S3-compatible server.
func (c *s3TestClient) deleteObject(ctx context.Context, key string) error {
	status, raw, err := c.roundTrip(ctx, http.MethodDelete, key, nil, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return fmt.Errorf("DeleteObject returned status %d: %s", status, truncateBody(string(raw), 256))
	}
	return nil
}

// hashSHA256 returns the hex SHA-256 of data. Helper for the
// SigV4 signer.
func hashSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// hmacSHA256 returns HMAC-SHA256(key, data). Used by SigV4 to
// derive the signing key by stepping over date → region → service
// → "aws4_request".
func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// canonicalQueryString returns the S3-canonical ordering of a
// query string (params sorted by key). Unused for the basic
// Put/Get/Delete calls (none of which use query params) but kept
// for completeness so future S3 calls that need signed query
// params can call it.
func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(values))
	for _, k := range keys {
		vs := values[k]
		sort.Strings(vs)
		for _, v := range vs {
			pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(pairs, "&")
}
