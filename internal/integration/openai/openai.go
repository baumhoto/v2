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

const defaultClientTimeout = 30 * time.Second

type Client struct {
	apiKey string
}

func NewClient(apiKey string) *Client {
	return &Client{apiKey: apiKey}
}

type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *Client) CreateChatCompletion(articleText string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("openai: missing api key")
	}

	messages := []Message{
		{
			Role:    "system",
			Content: "you are an assistant that summarizes articles. Summarize the text in one sentence focusing only on the main point followed by a list of keypoints separated by an empty line. Format the summary as html. if the article is written in English summarize it in English. if it is written in any other language summarize it in German.",
		},
		{
			Role:    "user",
			Content: articleText,
		},
	}

	requestBody := &ChatCompletionRequest{
		Model:    "gpt-5-nano",
		Messages: messages,
	}

	requestBodyJson, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("openai: unable to encode request body: %v", err)
	}

	request, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(requestBodyJson))
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
		return "", fmt.Errorf("openai: unable to create chat completion: status=%d", response.StatusCode)
	}

	var chatCompletionResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	err = json.NewDecoder(response.Body).Decode(&chatCompletionResponse)
	if err != nil {
		return "", fmt.Errorf("openai: unable to decode response body: %v", err)
	}

	if len(chatCompletionResponse.Choices) > 0 {
		return chatCompletionResponse.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("openai: no choices returned")
}
