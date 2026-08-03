package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const apiBase = "https://api.github.com"

// Client is a minimal REST client for the two endpoints vigil needs:
// creating issues and checking whether one is still open.
type Client struct {
	Token      string
	Repo       string // "owner/repo"
	HTTPClient *http.Client
}

func NewClient(token, repo string) *Client {
	return &Client{
		Token:      token,
		Repo:       repo,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type Issue struct {
	Number  int    `json:"number"`
	State   string `json:"state"` // "open" or "closed"
	HTMLURL string `json:"html_url"`
}

// CreateIssue opens a new issue with the given title, body, and labels.
func (c *Client) CreateIssue(title, body string, labels []string) (Issue, error) {
	payload, err := json.Marshal(map[string]any{
		"title":  title,
		"body":   body,
		"labels": labels,
	})
	if err != nil {
		return Issue{}, fmt.Errorf("marshal issue: %w", err)
	}

	url := fmt.Sprintf("%s/repos/%s/issues", apiBase, c.Repo)
	var issue Issue
	if err := c.do(http.MethodPost, url, bytes.NewReader(payload), &issue); err != nil {
		return Issue{}, err
	}
	return issue, nil
}

// GetIssue fetches an issue's current state (open/closed).
func (c *Client) GetIssue(number int) (Issue, error) {
	url := fmt.Sprintf("%s/repos/%s/issues/%d", apiBase, c.Repo, number)
	var issue Issue
	if err := c.do(http.MethodGet, url, nil, &issue); err != nil {
		return Issue{}, err
	}
	return issue, nil
}

func (c *Client) do(method, url string, body io.Reader, out any) error {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("github request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("github %s %s: status %d: %s", method, url, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode github response: %w", err)
		}
	}
	return nil
}
