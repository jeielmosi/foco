package filetypes

import (
	"testing"

	"foco/internal/enums"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		path string
		data []byte
		want enums.FileCategory
	}{
		{"main.go", nil, enums.FileCategoryCode},
		{"README.md", nil, enums.FileCategoryDocs},
		{"package.json", nil, enums.FileCategoryConfig},
		{"data.json", nil, enums.FileCategoryData},
		{"trace.log.1.gz", nil, enums.FileCategoryLogs},
		{"main.go~", nil, enums.FileCategoryTemp},
		{"song.mp3", nil, enums.FileCategoryAudio},
		{"clip.mp4", nil, enums.FileCategoryVideo},
		{"bundle.zip", nil, enums.FileCategoryArchives},
		{"script", []byte("#!/usr/bin/env python3\nprint(1)\n"), enums.FileCategoryCode},
		{"tool", []byte("#!/usr/bin/env node\n"), enums.FileCategoryCode},
		{"blob.bin", nil, enums.FileCategoryUnknown},
	}
	for _, tc := range cases {
		got, _ := Classify(tc.path, tc.data)
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestAllReturnsUpstreamOrder(t *testing.T) {
	want := []enums.FileCategory{
		enums.FileCategoryCode, enums.FileCategoryConfig, enums.FileCategoryDocs,
		enums.FileCategoryData, enums.FileCategoryAudio, enums.FileCategoryVideo,
		enums.FileCategoryArchives, enums.FileCategoryLogs, enums.FileCategoryTemp,
		enums.FileCategoryUnknown,
	}
	got := All()
	if len(got) != len(want) {
		t.Fatalf("All() = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("All() = %v, want %v", got, want)
		}
	}
}

func TestFileCategoryUnknownIsValid(t *testing.T) {
	if !enums.FileCategoryUnknown.IsValid() {
		t.Fatal("FileCategoryUnknown should be valid")
	}
}
