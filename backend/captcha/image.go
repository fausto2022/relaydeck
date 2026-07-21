package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type imageTaskCreateResp struct {
	ErrorID          int             `json:"errorId"`
	ErrorCode        string          `json:"errorCode"`
	ErrorDescription string          `json:"errorDescription"`
	TaskID           json.RawMessage `json:"taskId"`
	Status           string          `json:"status"`
	Solution         imageSolution   `json:"solution"`
}

type imageTaskResultResp struct {
	ErrorID          int           `json:"errorId"`
	ErrorCode        string        `json:"errorCode"`
	ErrorDescription string        `json:"errorDescription"`
	Status           string        `json:"status"`
	Solution         imageSolution `json:"solution"`
}

type imageSolution struct {
	Text string `json:"text"`
}

func solveImageTask(
	ctx context.Context,
	httpClient *resty.Client,
	baseURL, apiKey, providerName, imageBase64 string,
	includeModule bool,
) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", fmt.Errorf("%s: api key is empty", providerName)
	}
	body := stripImageDataPrefix(imageBase64)
	if body == "" {
		return "", fmt.Errorf("%s: captcha image is empty", providerName)
	}
	task := map[string]any{
		"type":      "ImageToTextTask",
		"body":      body,
		"case":      true,
		"minLength": 4,
		"maxLength": 8,
	}
	if includeModule {
		task["module"] = "common"
	}
	resp, err := httpClient.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]any{"clientKey": apiKey, "task": task}).
		Post(strings.TrimRight(baseURL, "/") + "/createTask")
	if err != nil {
		return "", fmt.Errorf("%s image createTask http: %w", providerName, err)
	}
	var created imageTaskCreateResp
	if err := json.Unmarshal(resp.Body(), &created); err != nil {
		return "", fmt.Errorf("%s image createTask decode: %w", providerName, err)
	}
	if created.ErrorID != 0 {
		return "", fmt.Errorf("%s image createTask: %s %s", providerName, created.ErrorCode, created.ErrorDescription)
	}
	if text := normalizeCaptchaText(created.Solution.Text); text != "" {
		return text, nil
	}
	taskID := normalizeTaskID(created.TaskID)
	if taskID == "" {
		return "", fmt.Errorf("%s image createTask: empty taskId", providerName)
	}

	deadline := time.Now().Add(120 * time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return "", fmt.Errorf("%s image: timed out waiting for solution", providerName)
			}
			text, ready, err := fetchImageTaskResult(ctx, httpClient, baseURL, apiKey, providerName, taskID)
			if err != nil {
				return "", err
			}
			if ready {
				return text, nil
			}
		}
	}
}

func fetchImageTaskResult(
	ctx context.Context,
	httpClient *resty.Client,
	baseURL, apiKey, providerName, taskID string,
) (string, bool, error) {
	resp, err := httpClient.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]any{"clientKey": apiKey, "taskId": taskID}).
		Post(strings.TrimRight(baseURL, "/") + "/getTaskResult")
	if err != nil {
		return "", false, fmt.Errorf("%s image getTaskResult http: %w", providerName, err)
	}
	var result imageTaskResultResp
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return "", false, fmt.Errorf("%s image getTaskResult decode: %w", providerName, err)
	}
	if result.ErrorID != 0 {
		return "", false, fmt.Errorf("%s image getTaskResult: %s %s", providerName, result.ErrorCode, result.ErrorDescription)
	}
	if result.Status != "ready" {
		return "", false, nil
	}
	text := normalizeCaptchaText(result.Solution.Text)
	if text == "" {
		return "", false, errors.New(providerName + " image: ready but empty text")
	}
	return text, true, nil
}

func stripImageDataPrefix(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.Index(value, ","); strings.HasPrefix(value, "data:image/") && index >= 0 {
		return strings.TrimSpace(value[index+1:])
	}
	return value
}

func normalizeTaskID(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String()
	}
	return ""
}

func normalizeCaptchaText(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
