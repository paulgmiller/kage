package kage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerge(t *testing.T) {
	t.Parallel()
	file := func(lines ...Line) File { return File{{Name: "api", Lines: lines}} }
	original := Line{Key: "TOKEN", Value: "original"}
	changed := Line{Key: "TOKEN", Value: "changed-value"}
	other := Line{Key: "TOKEN", Value: "other-value"}
	added := Line{Key: "EXTRA", Value: "new-value"}
	comment := Line{Comment: "keep this note"}
	tests := []struct {
		name                     string
		base, ours, theirs, want File
		conflict                 string
	}{
		{name: "unchanged", base: file(original), ours: file(original), theirs: file(original), want: file(original)},
		{name: "independent additions", base: file(original), ours: file(original, added), theirs: append(file(original), Secret{Name: "worker", Lines: []Line{other}}), want: append(file(added, original), Secret{Name: "worker", Lines: []Line{other}})},
		{name: "ours changed", base: file(original), ours: file(changed), theirs: file(original), want: file(changed)},
		{name: "theirs changed", base: file(original), ours: file(original), theirs: file(changed), want: file(changed)},
		{name: "same change", base: file(original), ours: file(changed), theirs: file(changed), want: file(changed)},
		{name: "different changes", base: file(original), ours: file(changed), theirs: file(other), conflict: "api/TOKEN"},
		{name: "different additions", base: file(), ours: file(changed), theirs: file(other), conflict: "api/TOKEN"},
		{name: "same addition", base: file(), ours: file(changed), theirs: file(changed), want: file(changed)},
		{name: "ours deleted", base: file(original), ours: file(), theirs: file(original, added), want: file(added)},
		{name: "theirs deleted", base: file(original), ours: file(original, added), theirs: file(), want: file(added)},
		{name: "both deleted key", base: file(original), ours: file(), theirs: file(), want: file()},
		{name: "delete versus edit", base: file(original), ours: file(), theirs: file(changed), conflict: "api/TOKEN"},
		{name: "edit versus delete", base: file(original), ours: file(changed), theirs: nil, conflict: "api/TOKEN"},
		{name: "delete block", base: append(file(original), Secret{Name: "worker"}), ours: File{{Name: "worker"}}, theirs: append(file(original), Secret{Name: "worker"}), want: File{{Name: "worker"}}},
		{name: "comments retained", base: file(comment, original), ours: file(comment, changed), theirs: file(comment, original, added), want: file(comment, added, changed)},
		{name: "comment edit", base: file(original), ours: file(comment, original), theirs: file(original), want: file(comment, original)},
		{name: "comment conflict", base: file(original), ours: file(comment, original), theirs: file(Line{Comment: "another note"}, original), conflict: "api/(comments)"},
		{name: "inline comment retained", base: file(original), ours: file(Line{Key: "TOKEN", Value: original.Value, Comment: "note"}), theirs: file(original), want: file(Line{Key: "TOKEN", Value: original.Value, Comment: "note"})},
		{name: "all blocks deleted", base: file(original), ours: nil, theirs: nil, conflict: "no secret blocks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Merge(tt.base, tt.ours, tt.theirs)
			if tt.conflict != "" {
				require.ErrorContains(t, err, tt.conflict)
				assert.NotContains(t, err.Error(), "changed-value")
				assert.NotContains(t, err.Error(), "other-value")
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
