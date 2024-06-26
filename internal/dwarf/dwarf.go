package dwarf

import (
	"debug/dwarf"
	"fmt"
	"log/slog"
	"strings"
)

type Content struct {
	Name string
	Addr uint64
	Size uint64
}

// SizeForDWARFVar need addr because it may in .bss section
// readMemory should only be called once
// return addr, size, type size, error
func SizeForDWARFVar(
	d *dwarf.Data,
	entry *dwarf.Entry,
	readMemory MemoryReader,
) ([]Content, uint64, error) {
	sizeOffset, ok := entry.Val(dwarf.AttrType).(dwarf.Offset)
	if !ok {
		return nil, 0, fmt.Errorf("failed to get type offset")
	}

	typ, err := d.Type(sizeOffset)
	if err != nil {
		return nil, 0, err
	}

	structTyp, ok := typ.(*dwarf.StructType)
	if ok {
		// check string
		// user can still define a struct has this name, but it's rare
		if structTyp.StructName == "string" {
			strAddr, size, err := readString(structTyp, readMemory)
			if err != nil || size == 0 {
				return nil, uint64(typ.Size()), err
			}

			return []Content{{
				Name: "string",
				Addr: strAddr,
				Size: size,
			}}, uint64(typ.Size()), nil
		} else if structTyp.StructName == "[]uint8" {
			// check byte slice, normally it comes from embed
			dataAddr, size, err := readSlice(structTyp, readMemory, "*uint8")
			if err != nil || size == 0 {
				return nil, uint64(typ.Size()), err
			}

			return []Content{{
				Name: "[]uint8",
				Addr: dataAddr,
				Size: size,
			}}, uint64(typ.Size()), nil
		}
	} else {
		typeDefTyp, ok := typ.(*dwarf.TypedefType)
		if ok {
			structTyp, ok = typeDefTyp.Type.(*dwarf.StructType)
			if !ok {
				return nil, uint64(typ.Size()), nil
			}

			if structTyp.StructName == "embed.FS" {
				// check embed.FS
				parts, err := readEmbedFS(structTyp, readMemory)
				if err != nil || len(parts) == 0 {
					return nil, uint64(typ.Size()), err
				}

				return parts, uint64(typ.Size()), nil
			}
		}
	}

	return nil, uint64(typ.Size()), nil
}

func EntryShouldIgnore(entry *dwarf.Entry) bool {
	declaration := entry.Val(dwarf.AttrDeclaration)
	if declaration != nil {
		val, ok := declaration.(bool)
		if ok && val {
			return true
		}
	}

	inline := entry.Val(dwarf.AttrInline)
	if inline != nil {
		val, ok := inline.(bool)
		if ok && val {
			return true
		}
	}

	abstractOrigin := entry.Val(dwarf.AttrAbstractOrigin)
	if abstractOrigin != nil {
		return true
	}

	specification := entry.Val(dwarf.AttrSpecification)

	return specification != nil
}

func EntryFileReader(cu *dwarf.Entry, d *dwarf.Data) func(entry *dwarf.Entry) string {
	var files []*dwarf.LineFile
	lr, err := d.LineReader(cu)
	if err != nil {
		slog.Warn(fmt.Sprintf("Failed to read DWARF line: %v", err))
	}
	if lr != nil {
		files = lr.Files()
	}

	return func(entry *dwarf.Entry) string {
		const defaultName = "<autogenerated>"
		if entry.Val(dwarf.AttrTrampoline) == nil {
			fileIndexAny := entry.Val(dwarf.AttrDeclFile)
			if fileIndexAny == nil {
				slog.Warn(fmt.Sprintf("Failed to load DWARF function file: %s", EntryPrettyPrinter(entry)))
				return defaultName
			}
			fileIndex, ok := fileIndexAny.(int64)
			if !ok || fileIndex < 0 || int(fileIndex) >= len(files) {
				slog.Warn(fmt.Sprintf("Failed to load DWARF function file: %s", EntryPrettyPrinter(entry)))
				return defaultName
			}

			return files[fileIndex].Name
		}

		return defaultName
	}
}

func EntryPrettyPrinter(entry *dwarf.Entry) string {
	ret := new(strings.Builder)
	for _, field := range entry.Field {
		ret.WriteString(fmt.Sprintf("%#v", field))
	}

	return ret.String()
}
