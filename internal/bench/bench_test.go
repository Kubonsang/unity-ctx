package bench

import (
	"encoding/json"
	"testing"
)

func TestEstimateTokensUsesCeilBytesDiv4(t *testing.T) {
	cases := []struct {
		name  string
		bytes int
		want  int
	}{
		{name: "zero", bytes: 0, want: 0},
		{name: "one", bytes: 1, want: 1},
		{name: "four", bytes: 4, want: 1},
		{name: "five", bytes: 5, want: 2},
		{name: "eight", bytes: 8, want: 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EstimateTokens(tc.bytes)
			if got != tc.want {
				t.Fatalf("EstimateTokens(%d) = %d, want %d", tc.bytes, got, tc.want)
			}
		})
	}
}

func TestBuildWithoutContextPack(t *testing.T) {
	got := Build(Input{
		RawBytes:       8,
		SummarizeBytes: 4,
	})

	if got.RawBytes != 8 {
		t.Fatalf("Build().RawBytes = %d, want 8", got.RawBytes)
	}
	if got.RawTokens != 2 {
		t.Fatalf("Build().RawTokens = %d, want 2", got.RawTokens)
	}

	want := Metric{
		Bytes:       4,
		Tokens:      1,
		Ratio:       0.5,
		SavedTokens: 1,
	}
	if got.Summarize != want {
		t.Fatalf("Build().Summarize = %+v, want %+v", got.Summarize, want)
	}
	if got.ContextPack != nil {
		t.Fatalf("Build().ContextPack = %+v, want nil", got.ContextPack)
	}
}

func TestBuildWithContextPack(t *testing.T) {
	got := Build(Input{
		RawBytes:         16,
		SummarizeBytes:   8,
		HasContextPack:   true,
		ContextPackBytes: 4,
	})

	if got.RawBytes != 16 {
		t.Fatalf("Build().RawBytes = %d, want 16", got.RawBytes)
	}
	if got.RawTokens != 4 {
		t.Fatalf("Build().RawTokens = %d, want 4", got.RawTokens)
	}

	wantSummarize := Metric{
		Bytes:       8,
		Tokens:      2,
		Ratio:       0.5,
		SavedTokens: 2,
	}
	if got.Summarize != wantSummarize {
		t.Fatalf("Build().Summarize = %+v, want %+v", got.Summarize, wantSummarize)
	}

	if got.ContextPack == nil {
		t.Fatal("Build().ContextPack = nil, want metric")
	}

	wantContextPack := Metric{
		Bytes:       4,
		Tokens:      1,
		Ratio:       0.25,
		SavedTokens: 3,
	}
	if *got.ContextPack != wantContextPack {
		t.Fatalf("Build().ContextPack = %+v, want %+v", *got.ContextPack, wantContextPack)
	}
}

func TestBuildSavedTokensNeverBelowZero(t *testing.T) {
	got := Build(Input{
		RawBytes:         4,
		SummarizeBytes:   8,
		HasContextPack:   true,
		ContextPackBytes: 8,
	})

	if got.Summarize.SavedTokens != 0 {
		t.Fatalf("Build().Summarize.SavedTokens = %d, want 0", got.Summarize.SavedTokens)
	}
	if got.ContextPack == nil {
		t.Fatal("Build().ContextPack = nil, want metric")
	}
	if got.ContextPack.SavedTokens != 0 {
		t.Fatalf("Build().ContextPack.SavedTokens = %d, want 0", got.ContextPack.SavedTokens)
	}
}

func TestBuildRatiosAreDeterministic(t *testing.T) {
	got := Build(Input{
		RawBytes:         16,
		SummarizeBytes:   16,
		HasContextPack:   true,
		ContextPackBytes: 8,
	})

	if got.Summarize.Ratio != 1.0 {
		t.Fatalf("Build().Summarize.Ratio = %v, want 1.0", got.Summarize.Ratio)
	}
	if got.ContextPack == nil {
		t.Fatal("Build().ContextPack = nil, want metric")
	}
	if got.ContextPack.Ratio != 0.5 {
		t.Fatalf("Build().ContextPack.Ratio = %v, want 0.5", got.ContextPack.Ratio)
	}
}

func TestBuildNormalizesNegativeBytesToZero(t *testing.T) {
	got := Build(Input{
		RawBytes:         -4,
		SummarizeBytes:   -8,
		HasContextPack:   true,
		ContextPackBytes: -12,
	})

	if got.RawBytes != 0 {
		t.Fatalf("Build().RawBytes = %d, want 0", got.RawBytes)
	}
	if got.RawTokens != 0 {
		t.Fatalf("Build().RawTokens = %d, want 0", got.RawTokens)
	}
	if got.Summarize.Bytes != 0 {
		t.Fatalf("Build().Summarize.Bytes = %d, want 0", got.Summarize.Bytes)
	}
	if got.Summarize.Tokens != 0 {
		t.Fatalf("Build().Summarize.Tokens = %d, want 0", got.Summarize.Tokens)
	}
	if got.ContextPack == nil {
		t.Fatal("Build().ContextPack = nil, want metric")
	}
	if got.ContextPack.Bytes != 0 {
		t.Fatalf("Build().ContextPack.Bytes = %d, want 0", got.ContextPack.Bytes)
	}
	if got.ContextPack.Tokens != 0 {
		t.Fatalf("Build().ContextPack.Tokens = %d, want 0", got.ContextPack.Tokens)
	}
}

func TestBuildWithZeroRawBytesHasZeroRatioAndNonNegativeSavedTokens(t *testing.T) {
	got := Build(Input{
		RawBytes:         0,
		SummarizeBytes:   4,
		HasContextPack:   true,
		ContextPackBytes: 8,
	})

	if got.Summarize.Ratio != 0 {
		t.Fatalf("Build().Summarize.Ratio = %v, want 0", got.Summarize.Ratio)
	}
	if got.Summarize.SavedTokens < 0 {
		t.Fatalf("Build().Summarize.SavedTokens = %d, want non-negative", got.Summarize.SavedTokens)
	}
	if got.ContextPack == nil {
		t.Fatal("Build().ContextPack = nil, want metric")
	}
	if got.ContextPack.Ratio != 0 {
		t.Fatalf("Build().ContextPack.Ratio = %v, want 0", got.ContextPack.Ratio)
	}
	if got.ContextPack.SavedTokens < 0 {
		t.Fatalf("Build().ContextPack.SavedTokens = %d, want non-negative", got.ContextPack.SavedTokens)
	}
}

func TestBuildLeavesContextPackNilWhenBoolIsFalse(t *testing.T) {
	got := Build(Input{
		RawBytes:         16,
		SummarizeBytes:   8,
		ContextPackBytes: 4,
	})

	if got.ContextPack != nil {
		t.Fatalf("Build().ContextPack = %+v, want nil", got.ContextPack)
	}
}

func TestResultJSONUsesSnakeCaseAndOmitsNilContextPack(t *testing.T) {
	withoutContextPack := Build(Input{
		RawBytes:       16,
		SummarizeBytes: 8,
	})

	withoutJSON, err := json.Marshal(withoutContextPack)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var withoutMap map[string]any
	if err := json.Unmarshal(withoutJSON, &withoutMap); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if _, ok := withoutMap["raw_bytes"]; !ok {
		t.Fatalf("json keys = %v, want raw_bytes", withoutMap)
	}
	if _, ok := withoutMap["raw_tokens"]; !ok {
		t.Fatalf("json keys = %v, want raw_tokens", withoutMap)
	}
	summarizeValue, ok := withoutMap["summarize"]
	if !ok {
		t.Fatalf("json keys = %v, want summarize", withoutMap)
	}
	if _, ok := withoutMap["context_pack"]; ok {
		t.Fatalf("json keys = %v, want context_pack omitted", withoutMap)
	}
	if _, ok := withoutMap["rawBytes"]; ok {
		t.Fatalf("json keys = %v, want snake_case only", withoutMap)
	}

	summarizeMap, ok := summarizeValue.(map[string]any)
	if !ok {
		t.Fatalf("summarize = %T, want object", summarizeValue)
	}
	for _, key := range []string{"bytes", "tokens", "ratio", "saved_tokens"} {
		if _, ok := summarizeMap[key]; !ok {
			t.Fatalf("summarize keys = %v, want %q", summarizeMap, key)
		}
	}
	if _, ok := summarizeMap["savedTokens"]; ok {
		t.Fatalf("summarize keys = %v, want snake_case only", summarizeMap)
	}

	withContextPack := Build(Input{
		RawBytes:         16,
		SummarizeBytes:   8,
		HasContextPack:   true,
		ContextPackBytes: 4,
	})

	withJSON, err := json.Marshal(withContextPack)
	if err != nil {
		t.Fatalf("json.Marshal() with context_pack error = %v", err)
	}

	var withMap map[string]any
	if err := json.Unmarshal(withJSON, &withMap); err != nil {
		t.Fatalf("json.Unmarshal() with context_pack error = %v", err)
	}

	contextPackValue, ok := withMap["context_pack"]
	if !ok {
		t.Fatalf("json keys = %v, want context_pack", withMap)
	}

	contextPackMap, ok := contextPackValue.(map[string]any)
	if !ok {
		t.Fatalf("context_pack = %T, want object", contextPackValue)
	}
	for _, key := range []string{"bytes", "tokens", "ratio", "saved_tokens"} {
		if _, ok := contextPackMap[key]; !ok {
			t.Fatalf("context_pack keys = %v, want %q", contextPackMap, key)
		}
	}
	if _, ok := contextPackMap["savedTokens"]; ok {
		t.Fatalf("context_pack keys = %v, want snake_case only", contextPackMap)
	}
}
