package header

import "bytes"

const (
	KindEqual = iota
	KindChange
	KindInsert
)

type fieldDiff struct {
	kind  int
	field *Field
	index int
}

func diffFieldsMiddle(orig []*Field, changed []*Field, index int) (diffs []fieldDiff) {
	// either orig and changed are empty or the first element is different
	origLen, changedLen := len(orig), len(changed)
	changedI := 0
	switch {
	case origLen == 0 && changedLen == 0:
		return nil
	case origLen == 0:
		// orig empty -> everything must be inserts
		for _, c := range changed {
			diffs = append(diffs, fieldDiff{KindInsert, c, index})
		}
		return
	case changedLen == 0:
		// This should not happen since we do not delete headerField entries
		// but if the user completely replaces the headers it could indeed happen.
		// Panic in this case so the programming error surfaces.
		panic("internal structure error: do not completely replace transaction.Headers – use its methods to alter it")
	default: // origLen > 0 && changedLen > 0
		o := orig[0]
		if o.Index < 0 {
			panic("internal structure error: all elements in orig need to have an index bigger than -1: do not completely replace transaction.Headers – use its methods to alter it")
		}
		// find o.index in changed
		for i, c := range changed {
			if c.Index == o.Index {
				index = o.Index
				changedI = i
				for i = 0; i < changedI; i++ {
					diffs = append(diffs, fieldDiff{KindInsert, changed[i], index - 1})
				}
				if bytes.Equal(changed[changedI].Raw, o.Raw) {
					diffs = append(diffs, fieldDiff{KindEqual, o, o.Index})
				} else if changed[changedI].Key() == o.Key() {
					diffs = append(diffs, fieldDiff{KindChange, changed[changedI], o.Index})
				} else {
					// a HeaderFields.Replace call, delete the original
					diffs = append(diffs, fieldDiff{
						kind: KindChange,
						field: &Field{
							Index:        o.Index,
							CanonicalKey: o.CanonicalKey,
							Raw:          []byte(o.Key() + ":"),
						},
						index: o.Index,
					})
					// insert changed in front of deleted header
					diffs = append(diffs, fieldDiff{KindInsert, &Field{
						Index:        -1,
						CanonicalKey: changed[changedI].CanonicalKey,
						Raw:          changed[changedI].Raw,
					}, index})
					index-- // in this special case we actually do not need to increase the index below
				}
				changedI++
				break
			} else if c.Index > o.Index {
				panic("internal structure error: index of original was not found in changed: do not completely replace transaction.Headers – use its methods to alter it")
			}
		}
		// we only consumed the first element of orig
		index++
		restDiffs := diffFields(orig[1:], changed[changedI:], index)
		if len(restDiffs) > 0 {
			diffs = append(diffs, restDiffs...)
		}
		return
	}
}

func diffFields(orig []*Field, changed []*Field, index int) (diffs []fieldDiff) {
	origLen, changedLen := len(orig), len(changed)
	// find common prefix
	commonPrefixLen, commonSuffixLen := 0, 0
	for i := 0; i < origLen && i < changedLen; i++ {
		if !bytes.Equal(orig[i].Raw, changed[i].Raw) || orig[i].Index != changed[i].Index {
			break
		}
		commonPrefixLen += 1
		index = orig[i].Index
	}
	// find common suffix (down to the commonPrefixLen element)
	i, j := origLen-1, changedLen-1
	for i > commonPrefixLen-1 && j > commonPrefixLen-1 {
		if !bytes.Equal(orig[i].Raw, changed[j].Raw) || orig[i].Index != changed[j].Index {
			break
		}
		commonSuffixLen += 1
		i--
		j--
	}
	for i := 0; i < commonPrefixLen; i++ {
		diffs = append(diffs, fieldDiff{KindEqual, orig[i], orig[i].Index})
	}
	// find the changed parts, recursively calls diffFields afterwards
	middleDiffs := diffFieldsMiddle(orig[commonPrefixLen:origLen-commonSuffixLen], changed[commonPrefixLen:changedLen-commonSuffixLen], index)
	if len(middleDiffs) > 0 {
		diffs = append(diffs, middleDiffs...)
	}
	for i := origLen - commonSuffixLen; i < origLen; i++ {
		diffs = append(diffs, fieldDiff{KindEqual, orig[i], orig[i].Index})
	}
	return
}

type Op struct {
	Kind  int
	Index int
	Name  string
	Value string
}

// Diff finds differences between orig and changed.
// The differences are expressed as change and insert operations – to be mapped to milter modification actions.
// Deletions are changes to an empty value.
func Diff(orig *Header, changed *Header) (changeInsertOps []Op, addOps []Op) {
	origFields := orig.Fields()
	origLen := origFields.Len()
	origIndexByKeyCounter := make(map[string]int)
	origIndexByKey := make([]int, origLen)
	for i := 0; origFields.Next(); i++ {
		origIndexByKeyCounter[origFields.CanonicalKey()] += 1
		origIndexByKey[i] = origIndexByKeyCounter[origFields.CanonicalKey()]
	}
	diffs := diffFields(orig.fields, changed.fields, -1)
	for _, diff := range diffs {
		switch diff.kind {
		case KindInsert:
			idx := diff.index + 2
			if idx-1 >= origLen {
				addOps = append(addOps, Op{
					Index: idx,
					Name:  diff.field.Key(),
					Value: diff.field.Value(),
				})
			} else {
				changeInsertOps = append(changeInsertOps, Op{
					Kind:  KindInsert,
					Index: idx,
					Name:  diff.field.Key(),
					Value: diff.field.Value(),
				})
			}
		case KindChange:
			if diff.index < origLen {
				changeInsertOps = append(changeInsertOps, Op{
					Kind:  KindChange,
					Index: origIndexByKey[diff.index],
					Name:  diff.field.Key(),
					Value: diff.field.Value(),
				})
			} else { // should not happen but just make adds out of it
				addOps = append(addOps, Op{
					Index: diff.index + 1,
					Name:  diff.field.Key(),
					Value: diff.field.Value(),
				})
			}
		}
	}

	return
}

// Recreate deletes all headers of orig and adds all headers of changed.
func Recreate(orig *Header, changed *Header) (changeInsertOps []Op, addOps []Op) {
	origIndexByKeyCounter := make(map[string]int)
	origFields := orig.Fields()
	for i := 0; origFields.Next(); i++ {
		origIndexByKeyCounter[origFields.CanonicalKey()] += 1
		changeInsertOps = append(changeInsertOps, Op{
			Kind:  KindChange,
			Index: origIndexByKeyCounter[origFields.CanonicalKey()],
			Name:  origFields.Key(),
			Value: "",
		})
	}
	changedFields := changed.Fields()
	i := 0
	for changedFields.Next() {
		if changedFields.IsDeleted() {
			continue
		}
		addOps = append(addOps, Op{
			Index: i,
			Name:  changedFields.Key(),
			Value: changedFields.Value(),
		})
		i++
	}

	return
}

// DiffOrRecreate is a convenience method that either calls Diff or Recreate
func DiffOrRecreate(recreate bool, orig *Header, changed *Header) (changeInsertOps []Op, addOps []Op) {
	if recreate {
		return Recreate(orig, changed)
	}
	return Diff(orig, changed)
}
