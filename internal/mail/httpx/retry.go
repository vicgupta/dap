// Package httpx is the small HTTP helper the mail providers share.
package httpx

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Throttle is the latest rate-limit a provider reported.
type Throttle struct {
	Until  time.Time
	Status int
}

// Do sends req and, on 429 or 503, honors Retry-After once when the wait is short.
// Longer waits are returned as an error so a laptop agent does not sit blocked.
func Do(client *http.Client, req *http.Request) (*http.Response, Throttle, error) {
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, Throttle{}, err
	}
	if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusServiceUnavailable {
		return resp, Throttle{}, nil
	}
	wait := retryAfter(resp.Header.Get("Retry-After"))
	th := Throttle{Until: time.Now().Add(wait), Status: resp.StatusCode}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()
	if wait > 2*time.Second {
		return nil, th, fmt.Errorf("provider asked us to wait %s", wait.Round(time.Second))
	}
	if wait > 0 {
		time.Sleep(wait)
	}
	retry := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, th, err
		}
		retry.Body = body
	}
	resp, err = client.Do(retry)
	return resp, th, err
}

func retryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if sec, err := strconv.Atoi(v); err == nil {
		if sec < 0 {
			return 0
		}
		return time.Duration(sec) * time.Second
	}
	if when, err := http.ParseTime(v); err == nil {
		d := time.Until(when)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}
