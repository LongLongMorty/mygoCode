package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"mygocode/internal/config"
	"mygocode/internal/conversation"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

const anthropicStreamIdleTimeout = 5 * time.Minute

func supportsAdaptiveThinking(model string) bool {
	// 例如 claude-opus-4-6, claude-opus-4-7, claude-sonnet-4-6 等
	// 但不包括 claude-sonnet-4-5（4.5 版本使用 enabled 模式）
	for _, family := range []string{"claude-opus-4-", "claude-sonnet-4-"} {
		if strings.HasPrefix(model, family) {
			rest := model[len(family):]
			if len(rest) > 0 && rest[0] >= '6' && rest[0] <= '9' {
				return true
			}
		}
	}
	return false
}

type anthropicClient struct {
	client          anthropic.Client
	model           string
	thinking        bool
	systemPrompt    string
	maxOutputTokens int
	contextWindow   int
}

func newAnthropicClient(cfg *config.ProviderConfig, systemPrompt string) (*anthropicClient, error) {
	apiKey := cfg.ResolveAPIKey()
	if apiKey == "" {
		return nil, &AuthenticationError{
			Message: "Anthropic API key not found. Set it in .mygocode/config.yaml or via ANTHROPIC_API_KEY env var.",
		}
	}

	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(cfg.BaseURL),
	)

	return &anthropicClient{
		client:          client,
		model:           cfg.Model,
		thinking:        cfg.Thinking,
		systemPrompt:    systemPrompt,
		maxOutputTokens: cfg.GetMaxOutputTokens(),
		contextWindow:   cfg.GetContextWindow(),
	}, nil
}

func (c *anthropicClient) SetSystemPrompt(prompt string) {
	c.systemPrompt = prompt
}

func (c *anthropicClient) SetMaxOutputTokens(tokens int) {
	c.maxOutputTokens = tokens
}

// anthropicModelFetchTimeout 限制自动拉取模型元数据的时间，防止缓慢或
// 无法访问的端点延迟启动过程。
const anthropicModelFetchTimeout = 3 * time.Second

// FetchModelContextWindow 向兼容 Anthropic 的 /v1/models/{model} 端点查询
// 模型的 max_input_tokens。这是尽力而为的操作：任何错误（非 anthropic
// 端点、网络故障、超时、字段缺失）都会返回 0，并且不会 panic 或阻塞超过
// anthropicModelFetchTimeout。调用者将 0 视为"未知"，并回退到下一层
// 上下文窗口配置。
func (c *anthropicClient) FetchModelContextWindow(ctx context.Context) (window int) {
	// 硬性保护：此函数在启动时运行，因此 SDK 内部的 panic 或格式错误的
	// 响应必须静默降级，而不能导致进程崩溃。
	defer func() {
		if recover() != nil {
			window = 0
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, anthropicModelFetchTimeout)
	defer cancel()

	// 尽力而为的启动调用：禁用重试，让不稳定/失败的端点在超时内快速
	// 失败，而不是触发重试风暴。
	info, err := c.client.Models.Get(ctx, c.model, anthropic.ModelGetParams{}, option.WithMaxRetries(0))
	if err != nil || info == nil || info.MaxInputTokens <= 0 {
		return 0
	}
	return int(info.MaxInputTokens)
}

func (c *anthropicClient) Stream(ctx context.Context, conv *conversation.Manager, toolSchemas []map[string]any) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent, 64)
	errs := make(chan error, 1)

	msgs := buildAnthropicMessages(conv.GetMessages())

	var sdkTools []anthropic.ToolUnionParam
	for _, s := range toolSchemas {
		inputSchema, _ := s["input_schema"].(map[string]any)
		props, _ := inputSchema["properties"]
		required, _ := inputSchema["required"].([]string)
		desc, _ := s["description"].(string)
		sdkTools = append(sdkTools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        s["name"].(string),
				Description: param.NewOpt(desc),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: props,
					Required:   required,
				},
			},
		})
	}

	go func() {
		defer close(events)
		defer close(errs)

		maxTokens := int64(c.maxOutputTokens)
		// 将提示词缓存锚定在最长稳定前缀上：即系统提示词。这里标记一次，
		// 加上下面工具列表标记一次以及最终用户消息尾部标记一次 —— Anthropic
		// 会缓存到每个断点为止的内容，并在下次请求时重新校验字节一致性。
		// toolresult 包中的 ContentReplacementState 用于保持 tool_result
		// 内容跨这些断点的字节稳定性。
		params := anthropic.MessageNewParams{
			Model:     c.model,
			MaxTokens: maxTokens,
			System: []anthropic.TextBlockParam{{
				Text:         c.systemPrompt,
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			}},
			Messages: msgs,
		}
		markLastUserTailForCache(params.Messages)
		if c.thinking {
			if supportsAdaptiveThinking(c.model) {
				params.Thinking = anthropic.ThinkingConfigParamUnion{
					OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
				}
			} else {
				params.Thinking = anthropic.ThinkingConfigParamUnion{
					OfEnabled: &anthropic.ThinkingConfigEnabledParam{
						BudgetTokens: maxTokens - 1,
					},
				}
			}
		}
		if len(sdkTools) > 0 {
			// 工具 schema 在多轮对话中是稳定的，因此通过标记最后一个工具
			// 来缓存整个工具块基本没有额外成本。
			if last := sdkTools[len(sdkTools)-1].OfTool; last != nil {
				last.CacheControl = anthropic.NewCacheControlEphemeralParam()
			}
			params.Tools = sdkTools
		}

		stream := c.client.Messages.NewStreaming(ctx, params)
		defer stream.Close()

		var currentToolName, currentToolID, jsonAccum string
		var thinkingAccum, thinkingSignature string
		inThinking := false
		var accMessage anthropic.Message

		// 在单独的 goroutine 中读取 SSE 事件，以便能够响应 ctx 取消
		// 并检测静默的连接断开。如果底层连接在没有 FIN/RST 的情况下
		// 中断，SDK 的 stream.Next() 可能会无限阻塞。
		type sseResult struct {
			hasNext bool
		}
		nextCh := make(chan sseResult, 1)

		readNext := func() {
			nextCh <- sseResult{hasNext: stream.Next()}
		}

		idle := time.NewTimer(anthropicStreamIdleTimeout)
		defer idle.Stop()

		go readNext()
		for {
			var res sseResult
			select {
			case <-ctx.Done():
				errs <- &NetworkError{Message: fmt.Sprintf("context cancelled: %v", ctx.Err())}
				return
			case <-idle.C:
				errs <- &NetworkError{Message: fmt.Sprintf("stream idle timeout: no SSE events for %s", anthropicStreamIdleTimeout)}
				return
			case res = <-nextCh:
			}

			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(anthropicStreamIdleTimeout)

			if !res.hasNext {
				break
			}

			event := stream.Current()
			accMessage.Accumulate(event)
			// Anthropic SDK 的 Accumulate 只从 message_delta 中复制
			// OutputTokens，但某些服务提供商（如 MiniMax）也会在此
			// 报告 InputTokens 和缓存相关字段。这里手动补充。
			if mde, ok := event.AsAny().(anthropic.MessageDeltaEvent); ok {
				if mde.Usage.InputTokens > 0 {
					accMessage.Usage.InputTokens = mde.Usage.InputTokens
				}
				if mde.Usage.CacheReadInputTokens > 0 {
					accMessage.Usage.CacheReadInputTokens = mde.Usage.CacheReadInputTokens
				}
				if mde.Usage.CacheCreationInputTokens > 0 {
					accMessage.Usage.CacheCreationInputTokens = mde.Usage.CacheCreationInputTokens
				}
			}
			switch ev := event.AsAny().(type) {
			case anthropic.ContentBlockStartEvent:
				switch ev.ContentBlock.Type {
				case "thinking":
					inThinking = true
					thinkingAccum = ""
					thinkingSignature = ""
				case "tool_use":
					currentToolName = ev.ContentBlock.Name
					currentToolID = ev.ContentBlock.ID
					jsonAccum = ""
					events <- ToolCallStart{ToolName: currentToolName, ToolID: currentToolID}
				}
			case anthropic.ContentBlockDeltaEvent:
				switch delta := ev.Delta.AsAny().(type) {
				case anthropic.ThinkingDelta:
					thinkingAccum += delta.Thinking
					events <- ThinkingDelta{Text: delta.Thinking}
				case anthropic.SignatureDelta:
					thinkingSignature = delta.Signature
				case anthropic.TextDelta:
					events <- TextDelta{Text: delta.Text}
				case anthropic.InputJSONDelta:
					jsonAccum += delta.PartialJSON
					events <- ToolCallDelta{Text: delta.PartialJSON}
				}
			case anthropic.ContentBlockStopEvent:
				if inThinking {
					events <- ThinkingComplete{
						Thinking:  thinkingAccum,
						Signature: thinkingSignature,
					}
					inThinking = false
				}
				if currentToolName != "" {
					var args map[string]any
					if jsonAccum != "" {
						json.Unmarshal([]byte(jsonAccum), &args)
					}
					if args == nil {
						args = map[string]any{}
					}
					events <- ToolCallComplete{
						ToolID:    currentToolID,
						ToolName:  currentToolName,
						Arguments: args,
					}
					currentToolName = ""
					currentToolID = ""
					jsonAccum = ""
				}
			}

			go readNext()
		}

		if err := stream.Err(); err != nil {
			errs <- classifyAnthropicError(err)
			return
		}

		stopReason := string(accMessage.StopReason)
		if stopReason == "" {
			stopReason = "end_turn"
		}
		usage := UsageInfo{
			InputTokens:         int(accMessage.Usage.InputTokens),
			OutputTokens:        int(accMessage.Usage.OutputTokens),
			CacheReadTokens:     int(accMessage.Usage.CacheReadInputTokens),
			CacheCreationTokens: int(accMessage.Usage.CacheCreationInputTokens),
		}
		events <- StreamEnd{StopReason: stopReason, Usage: usage}
	}()

	return events, errs
}

// markLastUserTailForCache 为最后一条 user 角色消息的最后一个内容块
// 附加一个 ephemeral cache_control 标记。Anthropic 会缓存到该块（含）
// 之前的前缀；后续请求只要前缀字节完全一致就能命中缓存。toolresult
// 包中的 ContentReplacementState 用于保证 tool_result 内容跨该断点
// 的字节稳定性。
//
// 就地修改 `messages`。如果没有 user 消息或最后一条 user 消息没有
// 可标记的内容块，则不做任何操作。
func markLastUserTailForCache(messages []anthropic.MessageParam) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != anthropic.MessageParamRoleUser {
			continue
		}
		blocks := messages[i].Content
		if len(blocks) == 0 {
			return
		}
		last := &blocks[len(blocks)-1]
		switch {
		case last.OfText != nil:
			last.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()
		case last.OfToolResult != nil:
			last.OfToolResult.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		return
	}
}

func buildAnthropicMessages(messages []conversation.Message) []anthropic.MessageParam {
	var result []anthropic.MessageParam
	for _, m := range messages {
		if m.Role == "assistant" {
			var blocks []anthropic.ContentBlockParamUnion
			for _, tb := range m.ThinkingBlocks {
				blocks = append(blocks, anthropic.NewThinkingBlock(tb.Signature, tb.Thinking))
			}
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}
			for _, tu := range m.ToolUses {
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    tu.ToolUseID,
						Name:  tu.ToolName,
						Input: tu.Arguments,
					},
				})
			}
			if len(blocks) == 0 {
				blocks = append(blocks, anthropic.NewTextBlock(""))
			}
			result = append(result, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleAssistant,
				Content: blocks,
			})
		} else if len(m.ToolResults) > 0 {
			var blocks []anthropic.ContentBlockParamUnion
			for _, tr := range m.ToolResults {
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: tr.ToolUseID,
						IsError:   param.NewOpt(tr.IsError),
						Content: []anthropic.ToolResultBlockParamContentUnion{{
							OfText: &anthropic.TextBlockParam{Text: tr.Content},
						}},
					},
				})
			}
			result = append(result, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleUser,
				Content: blocks,
			})
		} else {
			// 合并连续的 user 文本消息，以保持角色交替。
			canMerge := false
			if n := len(result); n > 0 {
				prev := result[n-1]
				if prev.Role == anthropic.MessageParamRoleUser && len(prev.Content) > 0 && prev.Content[0].OfToolResult == nil {
					canMerge = true
				}
			}
			if canMerge {
				result[len(result)-1].Content = append(result[len(result)-1].Content, anthropic.NewTextBlock(m.Content))
			} else {
				result = append(result, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
			}
		}
	}
	return result
}

func classifyAnthropicError(err error) error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == 413 || strings.Contains(apiErr.Error(), "prompt is too long") {
			return &ContextTooLongError{Message: fmt.Sprintf("Context too long: %s", apiErr.Error())}
		}
		switch apiErr.Type() {
		case anthropic.ErrorTypeAuthenticationError:
			return &AuthenticationError{Message: fmt.Sprintf("Invalid API key: %s", apiErr.Error())}
		case anthropic.ErrorTypeRateLimitError:
			retry := ""
			if apiErr.Response != nil {
				retry = apiErr.Response.Header.Get("Retry-After")
			}
			msg := "Rate limited."
			if retry != "" {
				msg += fmt.Sprintf(" Retry after %ss.", retry)
			} else {
				msg += " Please wait."
			}
			return &RateLimitError{Message: msg, RetryAfter: retry}
		default:
			return &LLMError{Message: fmt.Sprintf("API error (%d): %s", apiErr.StatusCode, apiErr.Error())}
		}
	}
	return &NetworkError{Message: fmt.Sprintf("Network error: %s", err.Error())}
}
