// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package openai // import "miniflux.app/v2/internal/integration/openai"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"miniflux.app/v2/internal/version"
)

const (
	defaultClientTimeout       = 60 * time.Second
	defaultModel               = "gpt-4o"
	defaultReasoningEffort     = "medium"
	defaultSystemPrompt        = "You are an assistant that summarizes articles. Summarize the text in one sentence focusing only on the main point followed by a list of key points separated by an empty line. Format the summary as HTML. If the article is written in English, summarize it in English. If it is written in any other language, summarize it in German."
)

type Client struct {
	apiKey          string
	model           string
	reasoningEffort string
	systemPrompt    string
}

func NewClient(apiKey, model, reasoningEffort, systemPrompt string) *Client {
	if model == "" {
		model = defaultModel
	}
	if reasoningEffort == "" {
		reasoningEffort = defaultReasoningEffort
	}
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}
	return &Client{
		apiKey:          apiKey,
		model:           model,
		reasoningEffort: reasoningEffort,
		systemPrompt:    systemPrompt,
	}
}

type ResponsesRequest struct {
	Model        string      `json:"model"`
	Input        []InputItem `json:"input"`
	Instructions string      `json:"instructions,omitempty"`
	Reasoning    *Reasoning  `json:"reasoning,omitempty"`
}

type InputItem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Reasoning struct {
	Effort string `json:"effort"`
}

func (c *Client) CreateChatCompletion(articleText string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("openai: missing api key")
	}

	input := []InputItem{
		{
			Role:    "user",
			Content: articleText,
		},
	}

	requestBody := &ResponsesRequest{
		Model:        c.model,
		Input:        input,
		Instructions: c.systemPrompt,
	}

	if c.reasoningEffort != "none" {
		requestBody.Reasoning = &Reasoning{
			Effort: c.reasoningEffort,
		}
	}

	requestBodyJson, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("openai: unable to encode request body: %v", err)
	}

	request, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(requestBodyJson))
	if err != nil {
		return "", fmt.Errorf("openai: unable to create request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Miniflux/"+version.Version)
	request.Header.Set("Authorization", "Bearer "+c.apiKey)

	httpClient := &http.Client{Timeout: defaultClientTimeout}
	response, err := httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("openai: unable to send request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		return "", fmt.Errorf("openai: unable to create response: status=%d", response.StatusCode)
	}

	var responsesResponse struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}

	err = json.NewDecoder(response.Body).Decode(&responsesResponse)
	if err != nil {
		return "", fmt.Errorf("openai: unable to decode response body: %v", err)
	}

	for _, output := range responsesResponse.Output {
		if output.Type == "message" {
			for _, content := range output.Content {
				if content.Type == "output_text" {
					return content.Text, nil
				}
			}
		}
	}

	return "", fmt.Errorf("openai: no output returned")
}
