package xfyun

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func BuildAuthURL(baseURL, host, path, apiKey, apiSecret string, now time.Time) (string, error) {
	if baseURL == "" || host == "" || path == "" {
		return "", fmt.Errorf("xfyun auth url requires baseURL, host, and path")
	}

	date := now.UTC().Format(http.TimeFormat)
	requestLine := fmt.Sprintf("GET %s HTTP/1.1", path)
	signatureOrigin := fmt.Sprintf("host: %s\ndate: %s\n%s", host, date, requestLine)

	mac := hmac.New(sha256.New, []byte(apiSecret))
	_, _ = mac.Write([]byte(signatureOrigin))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	authorizationOrigin := fmt.Sprintf(
		`api_key="%s", algorithm="hmac-sha256", headers="host date request-line", signature="%s"`,
		apiKey,
		signature,
	)
	authorization := base64.StdEncoding.EncodeToString([]byte(authorizationOrigin))

	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	query := u.Query()
	query.Set("authorization", authorization)
	query.Set("date", date)
	query.Set("host", host)
	u.RawQuery = query.Encode()
	return u.String(), nil
}
