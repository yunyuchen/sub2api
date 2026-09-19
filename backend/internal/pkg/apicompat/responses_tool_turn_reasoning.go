package apicompat

import "strings"

// responsesToolTurnReasoningPlaceholder 是补给缺失推理文本的占位内容。DeepSeek 只校验
// reasoning_text 非空，不校验内容（空串 400、单空格 200）；单空格与上游 #7313 在
// Chat Completions 回退路径上使用的 reasoning_content 占位保持一致。
const responsesToolTurnReasoningPlaceholder = " "

// EnsureResponsesToolTurnReasoning 让 Responses input 里每个「含工具调用的 assistant 连续段」
// 都以一条带非空 reasoning_text 的 reasoning 条目开头。
//
// DeepSeek 原生 /responses 在思考模式下（包括不带任何思考参数的默认情况，只有
// reasoning.effort=none 例外）对历史做如下校验，不满足即返回
// 400 "The `reasoning_text` in the thinking mode must be passed back to the API."：
// 发起过工具调用的 assistant 连续段，第一个条目必须是 reasoning，且其 content 里有
// 非空的 reasoning_text。该 400 既不换号也不重试，整段对话报废。
//
// 同一段对话跨上游账号续聊时，历史可能来自另一家上游：火山方舟的 reasoning 条目只有
// summary_text，带工具调用时还经常完全不产出 reasoning（输出为 message + function_call）；
// DeepSeek 官方自己在不带思考参数时也会产出不带 reasoning 的工具调用轮。
//
// 以下规则均由直连 api.deepseek.com/responses 实测得出（2026-09-19）：
//   - 连续段的边界是工具输出以及 user / developer / system 消息。developer 通知同样切断
//     连续段：[reasoning, developer, function_call] 400，[developer, reasoning, function_call] 200。
//   - reasoning 必须位于连续段最前：[message, reasoning, function_call] 400；
//     在段首再补一条合成 reasoning 后 200，同一段内出现多条 reasoning 无妨。
//   - 剔除 reasoning 条目无效；不含工具调用的连续段不受校验；effort=none 时多出的合成条目无害。
//
// 无需改动时返回原 input 与 false，调用方据此保持请求体逐字节不变。
func EnsureResponsesToolTurnReasoning(input any) (any, bool) {
	items, ok := input.([]any)
	if !ok || len(items) == 0 {
		return input, false
	}

	rewritten := make([]any, 0, len(items)+1)
	changed := false
	runStart := -1 // 当前连续段第一个条目在 rewritten 中的下标
	runHasToolCall := false

	flush := func() {
		start, hasToolCall := runStart, runHasToolCall
		runStart, runHasToolCall = -1, false
		if start < 0 || !hasToolCall {
			return
		}
		for index := start; index < len(rewritten); index++ {
			item, _ := rewritten[index].(map[string]any)
			if item == nil || responsesItemType(item) != "reasoning" || responsesReasoningItemHasText(item) {
				continue
			}
			rewritten[index] = withResponsesReasoningText(item)
			changed = true
		}
		if first, _ := rewritten[start].(map[string]any); first != nil && responsesItemType(first) == "reasoning" {
			return
		}
		rewritten = append(rewritten, nil)
		copy(rewritten[start+1:], rewritten[start:])
		rewritten[start] = syntheticResponsesReasoningItem()
		changed = true
	}

	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || isResponsesAssistantRunBoundary(item) {
			flush()
			rewritten = append(rewritten, raw)
			continue
		}
		if runStart < 0 {
			runStart = len(rewritten)
		}
		if isResponsesToolCallItem(item) {
			runHasToolCall = true
		}
		rewritten = append(rewritten, raw)
	}
	flush()

	if !changed {
		return input, false
	}
	return rewritten, true
}

func syntheticResponsesReasoningItem() map[string]any {
	return map[string]any{
		"type":    "reasoning",
		"summary": []any{},
		"content": []any{map[string]any{"type": "reasoning_text", "text": responsesToolTurnReasoningPlaceholder}},
	}
}

func responsesItemType(item map[string]any) string {
	return strings.TrimSpace(stringValue(item["type"]))
}

func isResponsesReasoningTextPart(raw any) (map[string]any, bool) {
	part, _ := raw.(map[string]any)
	if part == nil || strings.TrimSpace(stringValue(part["type"])) != "reasoning_text" {
		return nil, false
	}
	return part, true
}

func responsesReasoningItemHasText(item map[string]any) bool {
	parts, _ := item["content"].([]any)
	for _, raw := range parts {
		if part, ok := isResponsesReasoningTextPart(raw); ok && stringValue(part["text"]) != "" {
			return true
		}
	}
	return false
}

// withResponsesReasoningText 返回补了 reasoning_text 的条目副本，不修改调用方持有的原 map。
// 文本取自条目自己的 summary，没有则用占位；已有但为空串的 reasoning_text 分片就地换成该文本。
func withResponsesReasoningText(item map[string]any) map[string]any {
	text := ""
	if summary, ok := item["summary"].([]any); ok {
		segments := make([]string, 0, len(summary))
		for _, raw := range summary {
			part, _ := raw.(map[string]any)
			if part == nil {
				continue
			}
			if segment := stringValue(part["text"]); strings.TrimSpace(segment) != "" {
				segments = append(segments, segment)
			}
		}
		text = strings.Join(segments, "\n")
	}
	if text == "" {
		text = responsesToolTurnReasoningPlaceholder
	}

	cloned := make(map[string]any, len(item)+1)
	for key, value := range item {
		cloned[key] = value
	}
	existing, _ := item["content"].([]any)
	content := make([]any, 0, len(existing)+1)
	filled := false
	for _, raw := range existing {
		part, ok := isResponsesReasoningTextPart(raw)
		if !ok || filled {
			content = append(content, raw)
			continue
		}
		patched := make(map[string]any, len(part))
		for key, value := range part {
			patched[key] = value
		}
		patched["text"] = text
		content = append(content, patched)
		filled = true
	}
	if !filled {
		content = append(content, map[string]any{"type": "reasoning_text", "text": text})
	}
	cloned["content"] = content
	return cloned
}

// isResponsesAssistantRunBoundary 判断条目是否切断 assistant 连续段：工具输出，以及
// user / developer / system 消息（含省略 type、只写 role 的简写形式）。
// 工具输出按 *_output 后缀放宽识别：这里只需要知道「在此断开」，放宽是安全方向；
// isResponsesToolOutputItem 的白名单服务于会改写 output 字段的媒体抬升，必须保持精确。
func isResponsesAssistantRunBoundary(item map[string]any) bool {
	itemType := responsesItemType(item)
	if isResponsesToolOutputItem(item) || strings.HasSuffix(itemType, "_output") || itemType == "mcp_approval_response" {
		return true
	}
	if itemType != "" && itemType != "message" {
		return false
	}
	switch strings.TrimSpace(stringValue(item["role"])) {
	case "user", "developer", "system":
		return true
	default:
		return false
	}
}

// isResponsesToolCallItem 识别 assistant 发起、需要客户端回传输出的工具调用。
// web_search_call / image_generation_call 等服务端内建工具的结果内嵌在条目自身，不在此列。
func isResponsesToolCallItem(item map[string]any) bool {
	switch responsesItemType(item) {
	case "function_call", "custom_tool_call", "local_shell_call", "tool_search_call", "mcp_tool_call":
		return true
	default:
		return false
	}
}
