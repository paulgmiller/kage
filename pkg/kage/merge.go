package kage

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Merge combines independent changes from ours and theirs relative to base.
// Entries are identified by secret name and key. Conflicts return names only,
// never values, and no partial result. Inputs are not modified.
func Merge(base, ours, theirs File) (File, error) {
	for i, input := range []File{base, ours, theirs} {
		if err := input.Validate(); err != nil {
			return nil, fmt.Errorf("merge input %d: %w", i+1, err)
		}
	}
	blocks := func(file File) map[string]Secret {
		result := make(map[string]Secret)
		for _, secret := range file {
			result[secret.Name] = secret
		}
		return result
	}
	b, o, t := blocks(base), blocks(ours), blocks(theirs)
	var merged File
	var conflicts []string
	for _, name := range mergeNames(b, o, t) {
		bs, os, ts := b[name], o[name], t[name]
		entries := func(secret Secret) (map[string]Line, []Line) {
			keys := make(map[string]Line)
			var comments []Line
			for _, line := range secret.Lines {
				if line.Key == "" {
					comments = append(comments, line)
				} else {
					keys[line.Key] = line
				}
			}
			return keys, comments
		}
		bk, bc := entries(bs)
		ok, oc := entries(os)
		tk, tc := entries(ts)
		secret := Secret{Name: name}
		switch {
		case reflect.DeepEqual(oc, tc), reflect.DeepEqual(tc, bc):
			secret.Lines = append(secret.Lines, oc...)
		case reflect.DeepEqual(oc, bc):
			secret.Lines = append(secret.Lines, tc...)
		default:
			conflicts = append(conflicts, name+"/(comments)")
		}
		for _, key := range mergeNames(bk, ok, tk) {
			bl, bp := bk[key]
			ol, op := ok[key]
			tl, tp := tk[key]
			var line Line
			var present bool
			switch {
			case ol == tl && op == tp, tl == bl && tp == bp:
				line, present = ol, op
			case ol == bl && op == bp:
				line, present = tl, tp
			default:
				conflicts = append(conflicts, name+"/"+key)
			}
			if present {
				secret.Lines = append(secret.Lines, line)
			}
		}
		_, inOurs := o[name]
		_, inTheirs := t[name]
		if len(secret.Lines) > 0 || (inOurs && inTheirs) || bs.Name == "" {
			merged = append(merged, secret)
		}
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("merge conflicts: %s", strings.Join(conflicts, ", "))
	}
	if len(merged) == 0 {
		return nil, fmt.Errorf("merge would leave no secret blocks")
	}
	if err := merged.Validate(); err != nil {
		return nil, fmt.Errorf("merged secrets did not validate: %w", err)
	}
	return merged, nil
}

func mergeNames[T any](maps ...map[string]T) []string {
	seen := make(map[string]bool)
	for _, items := range maps {
		for name := range items {
			seen[name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
