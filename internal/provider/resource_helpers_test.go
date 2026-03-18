package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPlatformStringToList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty string returns empty list",
			input: "",
			want:  []string{},
		},
		{
			name:  "single platform",
			input: "darwin",
			want:  []string{"darwin"},
		},
		{
			name:  "multiple platforms comma-separated",
			input: "darwin,linux,windows",
			want:  []string{"darwin", "linux", "windows"},
		},
		{
			name:  "trims spaces around commas",
			input: "darwin, linux, windows",
			want:  []string{"darwin", "linux", "windows"},
		},
		{
			name:  "ignores empty segments from extra commas",
			input: "darwin,,linux",
			want:  []string{"darwin", "linux"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := platformStringToList(tc.input)
			if got.IsNull() || got.IsUnknown() {
				t.Fatalf("expected a known non-null list, got: %v", got)
			}
			elems := got.Elements()
			if len(elems) != len(tc.want) {
				t.Fatalf("expected %d elements, got %d: %v", len(tc.want), len(elems), elems)
			}
			for i, w := range tc.want {
				sv, ok := elems[i].(types.String)
				if !ok {
					t.Fatalf("element %d is not types.String: %T", i, elems[i])
				}
				if sv.ValueString() != w {
					t.Errorf("element %d: expected %q, got %q", i, w, sv.ValueString())
				}
			}
		})
	}
}

func TestPlatformListToString(t *testing.T) {
	ctx := context.Background()

	makeList := func(platforms ...string) types.List {
		vals := make([]attr.Value, len(platforms))
		for i, p := range platforms {
			vals[i] = types.StringValue(p)
		}
		return types.ListValueMust(types.StringType, vals)
	}

	tests := []struct {
		name  string
		input types.List
		want  string
	}{
		{
			name:  "null list returns empty string",
			input: types.ListNull(types.StringType),
			want:  "",
		},
		{
			name:  "empty list returns empty string",
			input: types.ListValueMust(types.StringType, []attr.Value{}),
			want:  "",
		},
		{
			name:  "single platform",
			input: makeList("darwin"),
			want:  "darwin",
		},
		{
			name:  "multiple platforms joined with comma",
			input: makeList("darwin", "linux", "windows"),
			want:  "darwin,linux,windows",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := platformListToString(ctx, tc.input)
			if got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestExtractLabels(t *testing.T) {
	ctx := context.Background()

	makeList := func(labels ...string) types.List {
		vals := make([]attr.Value, len(labels))
		for i, l := range labels {
			vals[i] = types.StringValue(l)
		}
		return types.ListValueMust(types.StringType, vals)
	}

	t.Run("null list does not modify target", func(t *testing.T) {
		target := []string{"existing"}
		diags := extractLabels(ctx, types.ListNull(types.StringType), &target)
		if diags.HasError() {
			t.Fatalf("unexpected error: %v", diags)
		}
		if len(target) != 1 || target[0] != "existing" {
			t.Errorf("expected target unchanged, got: %v", target)
		}
	})

	t.Run("unknown list does not modify target", func(t *testing.T) {
		target := []string{"existing"}
		diags := extractLabels(ctx, types.ListUnknown(types.StringType), &target)
		if diags.HasError() {
			t.Fatalf("unexpected error: %v", diags)
		}
		if len(target) != 1 || target[0] != "existing" {
			t.Errorf("expected target unchanged, got: %v", target)
		}
	})

	t.Run("empty list sets empty slice", func(t *testing.T) {
		target := []string{"old"}
		diags := extractLabels(ctx, types.ListValueMust(types.StringType, []attr.Value{}), &target)
		if diags.HasError() {
			t.Fatalf("unexpected error: %v", diags)
		}
		if len(target) != 0 {
			t.Errorf("expected empty target, got: %v", target)
		}
	})

	t.Run("populated list extracts all labels", func(t *testing.T) {
		var target []string
		diags := extractLabels(ctx, makeList("MacOS", "Developers", "Beta"), &target)
		if diags.HasError() {
			t.Fatalf("unexpected error: %v", diags)
		}
		want := []string{"MacOS", "Developers", "Beta"}
		if len(target) != len(want) {
			t.Fatalf("expected %d labels, got %d: %v", len(want), len(target), target)
		}
		for i, w := range want {
			if target[i] != w {
				t.Errorf("label %d: expected %q, got %q", i, w, target[i])
			}
		}
	})
}

func TestReadPackageContent(t *testing.T) {
	t.Run("reads local file and computes sha256", func(t *testing.T) {
		content := []byte("fake pkg content for testing")
		tmpFile := filepath.Join(t.TempDir(), "test.pkg")
		if err := os.WriteFile(tmpFile, content, 0600); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		expectedHash := sha256.Sum256(content)
		expectedSHA := hex.EncodeToString(expectedHash[:])

		model := &softwarePackageResourceModel{
			PackagePath: types.StringValue(tmpFile),
		}
		got, sha, err := readPackageContent(context.Background(), model)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if string(got) != string(content) {
			t.Errorf("content mismatch: expected %q, got %q", content, got)
		}
		if sha != expectedSHA {
			t.Errorf("SHA mismatch: expected %s, got %s", expectedSHA, sha)
		}
	})

	t.Run("different files produce different sha", func(t *testing.T) {
		dir := t.TempDir()
		fileA := filepath.Join(dir, "a.pkg")
		fileB := filepath.Join(dir, "b.pkg")
		_ = os.WriteFile(fileA, []byte("content A"), 0600)
		_ = os.WriteFile(fileB, []byte("content B"), 0600)

		modelA := &softwarePackageResourceModel{PackagePath: types.StringValue(fileA)}
		modelB := &softwarePackageResourceModel{PackagePath: types.StringValue(fileB)}

		_, shaA, _ := readPackageContent(context.Background(), modelA)
		_, shaB, _ := readPackageContent(context.Background(), modelB)
		if shaA == shaB {
			t.Error("expected different SHAs for different content")
		}
	})

	t.Run("missing file returns error", func(t *testing.T) {
		model := &softwarePackageResourceModel{
			PackagePath: types.StringValue("/nonexistent/path/pkg.pkg"),
		}
		_, _, err := readPackageContent(context.Background(), model)
		if err == nil {
			t.Fatal("expected error for missing file, got nil")
		}
	})

	t.Run("no source returns nil content", func(t *testing.T) {
		model := &softwarePackageResourceModel{}
		got, sha, err := readPackageContent(context.Background(), model)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil content, got %d bytes", len(got))
		}
		if sha != "" {
			t.Errorf("expected empty SHA, got %s", sha)
		}
	})
}
