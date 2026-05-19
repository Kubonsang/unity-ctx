package bench

type Input struct {
	RawBytes         int
	SummarizeBytes   int
	HasContextPack   bool
	ContextPackBytes int
}

type Metric struct {
	Bytes       int
	Tokens      int
	Ratio       float64
	SavedTokens int
}

type Result struct {
	RawBytes    int
	RawTokens   int
	Summarize   Metric
	ContextPack *Metric
}

func EstimateTokens(utf8Bytes int) int {
	if utf8Bytes <= 0 {
		return 0
	}
	return (utf8Bytes + 3) / 4
}

func Build(in Input) Result {
	rawBytes := normalizeBytes(in.RawBytes)
	summarizeBytes := normalizeBytes(in.SummarizeBytes)
	contextPackBytes := normalizeBytes(in.ContextPackBytes)
	rawTokens := EstimateTokens(rawBytes)
	result := Result{
		RawBytes:  rawBytes,
		RawTokens: rawTokens,
		Summarize: buildMetric(summarizeBytes, rawTokens),
	}
	if in.HasContextPack {
		metric := buildMetric(contextPackBytes, rawTokens)
		result.ContextPack = &metric
	}
	return result
}

func buildMetric(bytes int, rawTokens int) Metric {
	tokens := EstimateTokens(bytes)
	return Metric{
		Bytes:       bytes,
		Tokens:      tokens,
		Ratio:       ratio(tokens, rawTokens),
		SavedTokens: savedTokens(rawTokens, tokens),
	}
}

func ratio(tokens int, rawTokens int) float64 {
	if rawTokens <= 0 {
		return 0
	}
	return float64(tokens) / float64(rawTokens)
}

func savedTokens(rawTokens int, tokens int) int {
	saved := rawTokens - tokens
	if saved < 0 {
		return 0
	}
	return saved
}

func normalizeBytes(bytes int) int {
	if bytes < 0 {
		return 0
	}
	return bytes
}
