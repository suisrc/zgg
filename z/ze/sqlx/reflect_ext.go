package sqlx

import "reflect"

// A StructMap is an index of field metadata for a struct.
type StructMap struct {
	Tree  *FieldInfo
	Index []*FieldInfo
	Paths map[string]*FieldInfo
	Names map[string]*FieldInfo

	Fields map[string]*FieldInfo // +++ 扩展结构体属性字段名
}

// getMapping returns a mapping for the t type, using the tagName, mapFunc and
// tagMapFunc to determine the canonical names of fields.
func getMapping(t reflect.Type, tagName string, mapFunc, tagMapFunc mapf) *StructMap {
	m := []*FieldInfo{}

	root := &FieldInfo{}
	queue := []typeQueue{}
	queue = append(queue, typeQueue{Deref(t), root, ""})

QueueLoop:
	for len(queue) != 0 {
		// pop the first item off of the queue
		tq := queue[0]
		queue = queue[1:]

		// ignore recursive field
		for p := tq.fi.Parent; p != nil; p = p.Parent {
			if tq.fi.Field.Type == p.Field.Type {
				continue QueueLoop
			}
		}

		nChildren := 0
		if tq.t.Kind() == reflect.Struct {
			nChildren = tq.t.NumField()
		}
		tq.fi.Children = make([]*FieldInfo, nChildren)

		// iterate through all of its fields
		for fieldPos := 0; fieldPos < nChildren; fieldPos++ {

			f := tq.t.Field(fieldPos)

			// parse the tag and the target name using the mapping options for this field
			tag, name := parseName(f, tagName, mapFunc, tagMapFunc)

			// if the name is "-", disabled via a tag, skip it
			if name == "-" {
				continue
			}

			fi := FieldInfo{
				Field:   f,
				Name:    name,
				Zero:    reflect.New(f.Type).Elem(),
				Options: parseOptions(tag),
			}

			// if the path is empty this path is just the name
			if tq.pp == "" {
				fi.Path = fi.Name
			} else {
				fi.Path = tq.pp + "." + fi.Name
			}

			// skip unexported fields
			if len(f.PkgPath) != 0 && !f.Anonymous {
				continue
			}

			// bfs search of anonymous embedded structs
			if f.Anonymous {
				pp := tq.pp
				if tag != "" {
					pp = fi.Path
				}

				fi.Embedded = true
				fi.Index = apnd(tq.fi.Index, fieldPos)
				nChildren := 0
				ft := Deref(f.Type)
				if ft.Kind() == reflect.Struct {
					nChildren = ft.NumField()
				}
				fi.Children = make([]*FieldInfo, nChildren)
				queue = append(queue, typeQueue{Deref(f.Type), &fi, pp})
			} else if fi.Zero.Kind() == reflect.Struct || (fi.Zero.Kind() == reflect.Ptr && fi.Zero.Type().Elem().Kind() == reflect.Struct) {
				fi.Index = apnd(tq.fi.Index, fieldPos)
				fi.Children = make([]*FieldInfo, Deref(f.Type).NumField())
				queue = append(queue, typeQueue{Deref(f.Type), &fi, fi.Path})
			}

			fi.Index = apnd(tq.fi.Index, fieldPos)
			fi.Parent = tq.fi
			tq.fi.Children[fieldPos] = &fi
			m = append(m, &fi)
		}
	}

	flds := &StructMap{Index: m, Tree: root, Paths: map[string]*FieldInfo{}, Names: map[string]*FieldInfo{}, Fields: map[string]*FieldInfo{}}
	for _, fi := range flds.Index {
		// check if nothing has already been pushed with the same path
		// sometimes you can choose to override a type using embedded struct
		fld, ok := flds.Paths[fi.Path]
		if !ok || fld.Embedded {
			flds.Paths[fi.Path] = fi
			if fi.Name != "" && !fi.Embedded {
				flds.Names[fi.Path] = fi
			}
		}
		flds.Fields[fi.GetFieldName()] = fi
	}

	return flds
}

func (f *FieldInfo) GetFieldName() string {
	if f.Parent != nil && f.Parent.Field.Name != "" {
		return f.Parent.GetFieldName() + "." + f.Field.Name
	}
	return f.Field.Name
}

func (s *StructMap) GetIndexName() []*FieldInfo {
	fis := []*FieldInfo{}
	for _, fi := range s.Index {
		if _, ok := s.Names[fi.Name]; ok {
			fis = append(fis, fi)
		}
	}
	return fis
}
