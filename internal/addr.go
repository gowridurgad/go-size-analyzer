package internal

import (
	"cmp"
	"fmt"
	"github.com/samber/lo"
	"slices"
)

type AddrType string

const (
	AddrTypeUnknown AddrType = "unknown" // it exists, but should never be collected
	AddrTypeText             = "text"    // for text section
	AddrTypeData             = "data"    // data / rodata section
)

type AddrPos struct {
	Addr uint64
	Size uint64
	Type AddrType
}

type Addr struct {
	AddrPos

	Pkg      *Package
	Function *Function // for symbol source it will be a nil

	SourceType AddrSourceType

	Meta any
}

func (a Addr) String() string {
	msg := fmt.Sprintf("Addr: %x Size: %x Pkg: %s SourceType: %s", a.Addr, a.Size, a.Pkg.Name, a.SourceType)
	msg += fmt.Sprintf(" Meta: %#v", a.Meta)
	return msg
}

type AddrCoverage = []AddrPos

type AddrSpace map[uint64]*Addr

func (a AddrSpace) Get(addr uint64) (ret *Addr, ok bool) {
	ret, ok = a[addr]
	return
}

func (a AddrSpace) Insert(addr *Addr) {
	old, ok := a.Get(addr.Addr)
	if ok {
		// use the larger one
		if old.Size < addr.Size {
			a[addr.Addr] = addr
		}
		return
	}
	a[addr.Addr] = addr
}

func (a AddrSpace) Merge(other AddrSpace) {
	for _, addr := range other {
		a.Insert(addr)
	}
}

// GetCoverage get the coverage of the current address space
func (a AddrSpace) GetCoverage(coverages ...AddrCoverage) AddrCoverage {
	ranges := lo.MapToSlice(a, func(k uint64, v *Addr) AddrPos {
		return v.AddrPos
	})

	fromCoverage := lo.Flatten(coverages)

	ranges = append(ranges, fromCoverage...)
	ranges = lo.Uniq(ranges)

	slices.SortFunc(ranges, func(a, b AddrPos) int {
		if a.Addr != b.Addr {
			return cmp.Compare(a.Addr, b.Addr)
		}
		return cmp.Compare(a.Size, b.Size)
	})

	cover := make([]AddrPos, 0)
	for _, r := range ranges {
		if len(cover) == 0 {
			cover = append(cover, r)
			continue
		}

		last := &cover[len(cover)-1]
		if last.Addr+last.Size >= r.Addr {
			// merge
			if last.Type != r.Type {
				panic(fmt.Errorf("addr %x type %s and %s conflict", r.Addr, last.Type, r.Type))
			}

			if last.Addr+last.Size < r.Addr+r.Size {
				last.Size = r.Addr + r.Size - last.Addr
			}
		} else {
			cover = append(cover, r)
		}
	}

	return cover
}

type KnownAddr struct {
	pclntab AddrSpace

	symbol         AddrSpace // package can be nil for cgo symbols
	symbolCoverage []AddrPos

	k *KnownInfo
}

func NewKnownAddr(k *KnownInfo) *KnownAddr {
	return &KnownAddr{
		pclntab: make(map[uint64]*Addr),
		symbol:  make(map[uint64]*Addr),
		k:       k,
	}
}

func (f *KnownAddr) InsertPclntab(addr uint64, size uint64, fn *Function, meta GoPclntabMeta) {
	cur := Addr{
		AddrPos: AddrPos{
			Addr: addr,
			Size: size,
			Type: AddrTypeText,
		},
		Pkg:        fn.Pkg,
		Function:   fn,
		SourceType: AddrSourceGoPclntab,

		Meta: meta,
	}
	f.pclntab.Insert(&cur)
}

func (f *KnownAddr) InsertSymbol(addr uint64, size uint64, p *Package, typ AddrType, meta SymbolMeta) {
	cur := Addr{
		AddrPos: AddrPos{
			Addr: addr,
			Size: size,
			Type: typ,
		},
		Pkg:        p,
		Function:   nil, // TODO: try to find the function?
		SourceType: AddrSourceSymbol,

		Meta: meta,
	}
	if typ == AddrTypeText {
		if _, ok := f.pclntab.Get(addr); ok {
			// pclntab always more accurate
			return
		}
	}
	f.symbol.Insert(&cur)
}

func (f *KnownAddr) BuildSymbolCoverage() {
	f.symbolCoverage = f.symbol.GetCoverage()
}

func (f *KnownAddr) SymbolCovHas(addr uint64, size uint64) bool {
	_, ok := slices.BinarySearchFunc(f.symbolCoverage, AddrPos{Addr: addr}, func(cur AddrPos, target AddrPos) int {
		if cur.Addr+cur.Size <= target.Addr {
			return -1
		}
		if cur.Addr >= target.Addr+size {
			return 1
		}
		return 0
	})
	return ok
}

func (f *KnownAddr) InsertDisasm(addr uint64, size uint64, fn *Function) {
	cur := Addr{
		AddrPos: AddrPos{
			Addr: addr,
			Size: size,
			Type: AddrTypeData,
		},
		Pkg:        fn.Pkg,
		Function:   fn,
		SourceType: AddrSourceDisasm,
		Meta:       nil,
	}

	// symbol type check
	if sv, ok := f.symbol.Get(addr); ok {
		if sv.Type != AddrTypeData {
			panic(fmt.Errorf("addr %x already in symbol, but not data type", addr))
		}
		// symbol is more accurate
		return
	}
	// symbol coverage check
	// this exists since the linker can merge some constant
	if f.SymbolCovHas(addr, size) {
		// symbol coverage is more accurate
		return
	}

	fn.Disasm.Insert(&cur)
}