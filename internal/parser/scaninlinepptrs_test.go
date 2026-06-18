package parser

import "testing"

func TestScanInlinePPtrs(t *testing.T) {
	type ref struct {
		fileID  int64
		guid    string
		hasGUID bool
	}
	cases := []struct {
		name string
		raw  string
		want []ref
	}{
		{"plain inline", `m_X: {fileID: 4001, guid: abcGUID, type: 3}`, []ref{{4001, "abcGUID", true}}},
		{"guidless", `m_X: {fileID: 4002}`, []ref{{4002, "", false}}},
		{"flow list single", `m_X: [{fileID: 4001, guid: G, type: 3}]`, []ref{{4001, "G", true}}},
		{"flow list multi", `m_X: [{fileID: 1, guid: G}, {fileID: 2, guid: G}]`, []ref{{1, "G", true}, {2, "G", true}}},
		{"multiline flow item", "m_X:\n  - {fileID: 4001,\n      guid: G, type: 3}", []ref{{4001, "G", true}}},
		{"nested sub-brace", `m_X: [{fileID: 4001, m_Range: {x: 0, y: 1}, guid: G, type: 3}]`, []ref{{4001, "G", true}}},
		{"guid before fileID", `m_X: {guid: G, fileID: 4001, type: 3}`, []ref{{4001, "G", true}}},
		{"quoted guid value", `m_X: {fileID: 4001, guid: 'G', type: 3}`, []ref{{4001, "G", true}}},
		{"quoted key", `m_X: {"fileID": 4001, guid: G, type: 3}`, []ref{{4001, "G", true}}},
		{"flow with extra field", `m_X: [{fileID: 4002, type: 0}]`, []ref{{4002, "", false}}},
		{"apostrophe in plain scalar before pptr", "m_Name: Player's Gun\nm_Ref: {fileID: 4001, guid: G, type: 3}", []ref{{4001, "G", true}}},
		{"double-quote scalar before pptr", "m_Name: \"a\\\"b\"\nm_Ref: {fileID: 4001, guid: G}", []ref{{4001, "G", true}}},
		{"two apostrophes then pptr", "m_A: don't\nm_B: can't\nm_Ref: {fileID: 4001, guid: G}", []ref{{4001, "G", true}}},
		{"duplicate fileID keys both emitted", `m_X: {fileID: 4001, fileID: 9999, guid: G}`, []ref{{4001, "G", true}, {9999, "G", true}}},
		{"no pptr", `m_X: {x: 0, y: 1}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []ref
			ScanInlinePPtrs(tc.raw, func(fileID int64, guid string, hasGUID bool) {
				got = append(got, ref{fileID, guid, hasGUID})
			})
			if len(got) != len(tc.want) {
				t.Fatalf("got %d refs %+v, want %d %+v", len(got), got, len(tc.want), tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("ref[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
