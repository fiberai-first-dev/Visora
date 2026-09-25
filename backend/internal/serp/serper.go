package serp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"net/url"
)

type SerpEntry struct {
	Position int
	Title    string
	URL      string
	Domain   string
	Snippet  string
}

type serperResponse struct {
	Organic []struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
		Position int   `json:"position"`
	} `json:"organic"`
}

// CheckQuery calls Serper.dev Google search API for Indian SERPs.
func CheckQuery(query string) ([]SerpEntry, error) {
	apiKey := os.Getenv("SERP_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("SERP_API_KEY is missing")
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"q":   query,
		"gl":  "in",
		"hl":  "en",
		"num": 20,
	})

	req, err := http.NewRequest("POST", "https://google.serper.dev/search", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("serper API returned status %d", resp.StatusCode)
	}

	var result serperResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var entries []SerpEntry
	for _, item := range result.Organic {
		u, _ := url.Parse(item.Link)
		domain := ""
		if u != nil {
			domain = u.Hostname()
		}
		
		entries = append(entries, SerpEntry{
			Position: item.Position,
			Title:    item.Title,
			URL:      item.Link,
			Domain:   domain,
			Snippet:  item.Snippet,
		})
	}

	return entries, nil
}
