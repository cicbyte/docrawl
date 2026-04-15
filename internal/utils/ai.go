package utils

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cicbyte/docrawl/internal/models"
)

// AIRequestMode AI请求模式
type AIRequestMode int

const (
	// AIRequestModeSync 同步（非流式）请求
	AIRequestModeSync AIRequestMode = iota
	// AIRequestModeStream 流式请求
	AIRequestModeStream
)

// AIRequestConfig AI请求配置
type AIRequestConfig struct {
	SystemPrompt string        // 系统提示词
	UserPrompt   string        // 用户输入内容
	Mode         AIRequestMode // 请求模式
	Streaming    bool          // 是否启用流式输出显示
}

// AIResponse AI响应结果
type AIResponse struct {
	Content  string        // AI返回的内容
	Duration time.Duration // 请求耗时
	Success  bool          // 是否成功
	Error    error         // 错误信息
}

// AIClient AI客户端封装
type AIClient struct {
	config     *models.AIConfig
	httpClient *http.Client
}

// NewAIClient 创建AI客户端
func NewAIClient(config *models.AIConfig) (*AIClient, error) {
	if config == nil {
		return nil, fmt.Errorf("AI配置不能为空")
	}

	if config.BaseURL == "" {
		return nil, fmt.Errorf("AI BaseURL不能为空")
	}

	if config.Model == "" {
		return nil, fmt.Errorf("AI Model不能为空")
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	return &AIClient{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}, nil
}

// openAIRequest OpenAI兼容API请求格式
type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

// openAIMessage OpenAI消息格式
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponse OpenAI兼容API响应格式
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// CallAI 调用AI接口的统一入口
func (c *AIClient) CallAI(ctx context.Context, reqConfig *AIRequestConfig) *AIResponse {
	startTime := time.Now()

	response := &AIResponse{
		Success: true,
	}

	var content string
	var err error

	// 根据模式执行请求
	switch reqConfig.Mode {
	case AIRequestModeStream:
		content, err = c.callAIStream(ctx, reqConfig.SystemPrompt, reqConfig.UserPrompt, reqConfig.Streaming)
	case AIRequestModeSync:
		content, err = c.callAISync(ctx, reqConfig.SystemPrompt, reqConfig.UserPrompt)
	default:
		response.Success = false
		response.Error = fmt.Errorf("不支持的AI请求模式: %v", reqConfig.Mode)
		response.Duration = time.Since(startTime)
		return response
	}

	if err != nil {
		response.Success = false
		response.Error = fmt.Errorf("AI处理失败: %w", err)
	} else {
		response.Content = content
	}

	response.Duration = time.Since(startTime)
	return response
}

// callAISync 同步（非流式）AI请求
func (c *AIClient) callAISync(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := openAIRequest{
		Model: c.config.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	if c.config.MaxTokens > 0 {
		reqBody.MaxTokens = c.config.MaxTokens
	}
	if c.config.Temperature > 0 {
		reqBody.Temperature = c.config.Temperature
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	// 构建请求URL
	url := c.config.BaseURL
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	url += "chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.ApiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API返回错误 %d: %s", resp.StatusCode, string(body))
	}

	var openaiResp openAIResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if openaiResp.Error != nil {
		return "", fmt.Errorf("API错误: %s", openaiResp.Error.Message)
	}

	if len(openaiResp.Choices) == 0 {
		return "", fmt.Errorf("API未返回任何选择")
	}

	return openaiResp.Choices[0].Message.Content, nil
}

// callAIStream 流式AI请求
func (c *AIClient) callAIStream(ctx context.Context, systemPrompt, userPrompt string, enableStreaming bool) (string, error) {
	reqBody := openAIRequest{
		Model: c.config.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: true,
	}

	if c.config.MaxTokens > 0 {
		reqBody.MaxTokens = c.config.MaxTokens
	}
	if c.config.Temperature > 0 {
		reqBody.Temperature = c.config.Temperature
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	// 构建请求URL
	url := c.config.BaseURL
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	url += "chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.ApiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API返回错误 %d: %s", resp.StatusCode, string(body))
	}

	var result strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	// 实时显示流式输出
	if enableStreaming {
		fmt.Print("🤖 AI回复: ")
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行
		if line == "" {
			continue
		}

		// 检查是否是SSE数据行
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		// 移除"data: "前缀
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var streamResp struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			continue
		}

		if streamResp.Error != nil {
			return "", fmt.Errorf("API流式错误: %s", streamResp.Error.Message)
		}

		if len(streamResp.Choices) > 0 {
			content := streamResp.Choices[0].Delta.Content
			result.WriteString(content)

			// 实时显示（如果启用）
			if enableStreaming {
				fmt.Print(content)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取流式响应失败: %w", err)
	}

	if enableStreaming {
		fmt.Println() // 换行
	}

	return result.String(), nil
}

// CallAISync 简化的同步AI调用接口
func (c *AIClient) CallAISync(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqConfig := &AIRequestConfig{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Mode:         AIRequestModeSync,
		Streaming:    false,
	}

	response := c.CallAI(ctx, reqConfig)
	if !response.Success {
		return "", response.Error
	}

	return response.Content, nil
}

// CallAIStream 简化的流式AI调用接口
func (c *AIClient) CallAIStream(ctx context.Context, systemPrompt, userPrompt string, enableStreaming bool) (string, error) {
	reqConfig := &AIRequestConfig{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Mode:         AIRequestModeStream,
		Streaming:    enableStreaming,
	}

	response := c.CallAI(ctx, reqConfig)
	if !response.Success {
		return "", response.Error
	}

	return response.Content, nil
}

// ValidateAIConfig 验证AI配置是否完整
func ValidateAIConfig(config *models.AIConfig) error {
	if config.BaseURL == "" {
		return fmt.Errorf("AI BaseURL不能为空")
	}
	if config.Model == "" {
		return fmt.Errorf("AI Model不能为空")
	}
	if config.Provider == "" {
		return fmt.Errorf("AI Provider不能为空")
	}

	// Ollama通常不需要API密钥，其他提供商需要
	if strings.ToLower(config.Provider) != "ollama" && config.ApiKey == "" {
		return fmt.Errorf("AI ApiKey不能为空")
	}

	if config.Timeout <= 0 {
		return fmt.Errorf("AI Timeout必须大于0")
	}
	if config.MaxTokens <= 0 {
		return fmt.Errorf("AI MaxTokens必须大于0")
	}

	return nil
}
