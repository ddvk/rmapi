package hwr

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
)

const apiURL = "https://cloud.myscript.com/api/v4.0/iink/batch"

// SendRequest sends a request to MyScript HWR API
func SendRequest(key, hmackey string, data []byte, mimeType string) ([]byte, error) {
	fullkey := key + hmackey
	mac := hmac.New(sha512.New, []byte(fullkey))
	mac.Write(data)
	result := hex.EncodeToString(mac.Sum(nil))

	client := http.Client{}

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", mimeType+", application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("applicationKey", key)
	req.Header.Set("hmac", result)

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: Status %d, Response: %s", res.StatusCode, string(body))
	}

	return body, nil
}

