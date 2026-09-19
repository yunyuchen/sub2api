package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func decodeToolTurnInput(t *testing.T, raw string) []any {
	t.Helper()
	var items []any
	require.NoError(t, json.Unmarshal([]byte(raw), &items))
	return items
}

func toolTurnItems(t *testing.T, input any) []any {
	t.Helper()
	items, ok := input.([]any)
	require.True(t, ok)
	return items
}

func toolTurnItem(t *testing.T, input any, index int) map[string]any {
	t.Helper()
	items := toolTurnItems(t, input)
	require.Greater(t, len(items), index)
	item, ok := items[index].(map[string]any)
	require.True(t, ok)
	return item
}

func toolTurnItemTypes(t *testing.T, input any) []string {
	t.Helper()
	items := toolTurnItems(t, input)
	types := make([]string, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		itemType := stringValue(item["type"])
		if itemType == "message" || itemType == "" {
			itemType = "message:" + stringValue(item["role"])
		}
		types = append(types, itemType)
	}
	return types
}

var syntheticReasoningContent = []any{map[string]any{"type": "reasoning_text", "text": " "}}

// 火山方舟带工具调用时常见的输出形态：先说一句话再调工具，完全没有 reasoning 条目。
func TestEnsureResponsesToolTurnReasoning_InsertsAtRunFrontWhenMissing(t *testing.T) {
	input := decodeToolTurnInput(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"weather in Paris?"}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Let me check."}]},
		{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{\"city\":\"Paris\"}"},
		{"type":"function_call_output","call_id":"call_1","output":"{\"temp_c\":18}"}
	]`)

	got, changed := EnsureResponsesToolTurnReasoning(input)
	require.True(t, changed)
	// 实测：插在 function_call 正前方（assistant message 之后）仍然 400，必须在连续段最前。
	require.Equal(t, []string{"message:user", "reasoning", "message:assistant", "function_call", "function_call_output"}, toolTurnItemTypes(t, got))

	reasoning := toolTurnItem(t, got, 1)
	require.Equal(t, []any{}, reasoning["summary"])
	require.Equal(t, syntheticReasoningContent, reasoning["content"])
	require.NotContains(t, reasoning, "id")
	require.Len(t, input, 4, "原 input 不应被改动")
}

// 火山方舟的 reasoning 条目只有 summary_text，没有 content。
func TestEnsureResponsesToolTurnReasoning_FillsReasoningTextFromSummary(t *testing.T) {
	input := decodeToolTurnInput(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"weather?"}]},
		{"type":"reasoning","id":"rs_021","status":"completed","summary":[{"type":"summary_text","text":"Need the tool."},{"type":"summary_text","text":"City is Paris."}]},
		{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_1","output":"ok"}
	]`)
	original := toolTurnItem(t, input, 1)

	got, changed := EnsureResponsesToolTurnReasoning(input)
	require.True(t, changed)
	require.Equal(t, []string{"message:user", "reasoning", "function_call", "function_call_output"}, toolTurnItemTypes(t, got))

	reasoning := toolTurnItem(t, got, 1)
	require.Equal(t, "rs_021", reasoning["id"])
	require.Equal(t, original["summary"], reasoning["summary"])
	require.Equal(t, []any{map[string]any{"type": "reasoning_text", "text": "Need the tool.\nCity is Paris."}}, reasoning["content"])
	require.NotContains(t, original, "content", "不应修改调用方持有的原条目")
}

// 实测：reasoning_text 为空串 400、单空格 200。空串分片就地换成占位，不追加第二个分片。
func TestEnsureResponsesToolTurnReasoning_ReplacesEmptyReasoningText(t *testing.T) {
	input := decodeToolTurnInput(t, `[
		{"type":"reasoning","id":"rs_1","summary":[],"content":[{"type":"reasoning_text","text":""}],"encrypted_content":"opaque"},
		{"type":"function_call","call_id":"c","name":"f","arguments":"{}"}
	]`)
	original := toolTurnItem(t, input, 0)

	got, changed := EnsureResponsesToolTurnReasoning(input)
	require.True(t, changed)
	reasoning := toolTurnItem(t, got, 0)
	require.Equal(t, "opaque", reasoning["encrypted_content"])
	require.Equal(t, syntheticReasoningContent, reasoning["content"])
	require.Equal(t, []any{map[string]any{"type": "reasoning_text", "text": ""}}, original["content"], "不应修改调用方持有的原分片")
}

// 实测：[message, reasoning, function_call] 400；在段首再补一条合成 reasoning 后 200。
func TestEnsureResponsesToolTurnReasoning_InsertsWhenReasoningIsNotFirst(t *testing.T) {
	input := decodeToolTurnInput(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"weather?"}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hold on"}]},
		{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"thinking"}]},
		{"type":"function_call","call_id":"c1","name":"f","arguments":"{}"},
		{"type":"function_call_output","call_id":"c1","output":"ok"}
	]`)

	got, changed := EnsureResponsesToolTurnReasoning(input)
	require.True(t, changed)
	require.Equal(t, []string{"message:user", "reasoning", "message:assistant", "reasoning", "function_call", "function_call_output"}, toolTurnItemTypes(t, got))
	require.Equal(t, syntheticReasoningContent, toolTurnItem(t, got, 1)["content"])
	require.Equal(t, []any{map[string]any{"type": "reasoning_text", "text": "thinking"}}, toolTurnItem(t, got, 3)["content"])
}

// 实测：developer 通知会切断 assistant 连续段——[reasoning, developer, function_call] 400，
// [developer, reasoning, function_call] 200。所以 developer 之后的工具调用需要自己的段首 reasoning。
func TestEnsureResponsesToolTurnReasoning_DeveloperNoticeSplitsRun(t *testing.T) {
	input := decodeToolTurnInput(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"Paris then Rome"}]},
		{"type":"reasoning","id":"ok_1","summary":[],"content":[{"type":"reasoning_text","text":"first"}]},
		{"type":"message","role":"developer","content":[{"type":"input_text","text":"notice"}]},
		{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_1","output":"ok"}
	]`)

	got, changed := EnsureResponsesToolTurnReasoning(input)
	require.True(t, changed)
	require.Equal(t, []string{"message:user", "reasoning", "message:developer", "reasoning", "function_call", "function_call_output"}, toolTurnItemTypes(t, got))
	require.Equal(t, "ok_1", toolTurnItem(t, got, 1)["id"], "developer 之前的连续段不含工具调用，保持原样")
	require.Equal(t, syntheticReasoningContent, toolTurnItem(t, got, 3)["content"])
}

// DeepSeek 官方自己的形态：content[reasoning_text] + encrypted_content。必须原样放行，
// 这样现有单账号流量的请求体逐字节不变。
func TestEnsureResponsesToolTurnReasoning_LeavesNativeDeepSeekHistoryUntouched(t *testing.T) {
	input := decodeToolTurnInput(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"weather?"}]},
		{"type":"reasoning","id":"cdf43f","summary":[],"content":[{"type":"reasoning_text","text":"thinking"}],"encrypted_content":"opaque"},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"checking"}]},
		{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{}"},
		{"type":"function_call","call_id":"call_2","name":"get_weather","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_1","output":"ok"},
		{"type":"function_call_output","call_id":"call_2","output":"ok"},
		{"type":"reasoning","id":"cdf440","summary":[],"content":[{"type":"reasoning_text","text":"done"}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"18C"}]}
	]`)

	got, changed := EnsureResponsesToolTurnReasoning(input)
	require.False(t, changed)
	gotItems := toolTurnItems(t, got)
	require.Same(t, &input[0], &gotItems[0], "无改动时必须返回原切片，调用方据此跳过重新序列化")
}

// 不含工具调用的连续段，DeepSeek 不校验 reasoning_text（实测纯文字历史原样 200），不动。
// web_search_call 等服务端内建工具不需要回传输出，同样不算工具调用。
func TestEnsureResponsesToolTurnReasoning_IgnoresRunsWithoutToolCalls(t *testing.T) {
	input := decodeToolTurnInput(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
		{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"greet"}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]},
		{"role":"user","content":"and now?"},
		{"type":"web_search_call","id":"ws_1","status":"completed"},
		{"role":"assistant","content":"found it"}
	]`)

	_, changed := EnsureResponsesToolTurnReasoning(input)
	require.False(t, changed)
}

// 同一个 user 轮里连续多次工具往返：每个工具输出都是边界，每段各自需要段首 reasoning。
// 未列入媒体抬升白名单的 *_output 类型同样是边界。连续多次插入会触发切片扩容。
func TestEnsureResponsesToolTurnReasoning_HandlesEachToolRoundTripSeparately(t *testing.T) {
	input := decodeToolTurnInput(t, `[
		{"role":"user","content":"three cities"},
		{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_1","output":"ok"},
		{"type":"local_shell_call","call_id":"call_2","action":{}},
		{"type":"local_shell_call_output","call_id":"call_2","output":"ok"},
		{"type":"function_call","call_id":"call_3","name":"get_weather","arguments":"{}"},
		{"type":"custom_tool_call","call_id":"call_4","name":"apply_patch","input":"x"},
		{"type":"function_call_output","call_id":"call_3","output":"ok"},
		{"type":"custom_tool_call_output","call_id":"call_4","output":"ok"}
	]`)

	got, changed := EnsureResponsesToolTurnReasoning(input)
	require.True(t, changed)
	require.Equal(t, []string{
		"message:user",
		"reasoning", "function_call", "function_call_output",
		"reasoning", "local_shell_call", "local_shell_call_output",
		"reasoning", "function_call", "custom_tool_call", "function_call_output", "custom_tool_call_output",
	}, toolTurnItemTypes(t, got))
}

// 400 自愈重试、invalid_encrypted_content 重试都会重新走一遍请求体归一化，函数必须是不动点，
// 否则每次重试都会叠加出新的合成 reasoning。
func TestEnsureResponsesToolTurnReasoning_IsIdempotent(t *testing.T) {
	input := decodeToolTurnInput(t, `[
		{"role":"user","content":"weather?"},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Let me check."}]},
		{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"late"}]},
		{"type":"function_call","call_id":"c1","name":"f","arguments":"{}"},
		{"type":"function_call_output","call_id":"c1","output":"ok"},
		{"type":"message","role":"developer","content":[{"type":"input_text","text":"notice"}]},
		{"type":"function_call","call_id":"c2","name":"f","arguments":"{}"},
		{"type":"function_call_output","call_id":"c2","output":"ok"}
	]`)

	once, changed := EnsureResponsesToolTurnReasoning(input)
	require.True(t, changed)
	twice, changedAgain := EnsureResponsesToolTurnReasoning(once)
	require.False(t, changedAgain)
	require.Equal(t, once, twice)
}

func TestEnsureResponsesToolTurnReasoning_NonArrayInput(t *testing.T) {
	got, changed := EnsureResponsesToolTurnReasoning("plain text input")
	require.False(t, changed)
	require.Equal(t, "plain text input", got)

	_, changed = EnsureResponsesToolTurnReasoning([]any{})
	require.False(t, changed)
	_, changed = EnsureResponsesToolTurnReasoning([]any{"not-an-object", 42})
	require.False(t, changed)
}
