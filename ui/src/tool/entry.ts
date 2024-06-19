import {File, FileSymbol, Package, Result, Section} from "../generated/schema.ts";
import {orderedID} from "./id.ts";
import {formatBytes, title, trimPrefix} from "./utils.ts";
import {aligner} from "./aligner.ts";

export type EntryType = "section" | "file" | "package" | "result" | "symbol" | "disasm" | "unknown" | "container";

export type EntryChildren = {
    "section": never[],
    "file": never[],
    "package": EntryLike<"package" | "symbol" | "disasm" | "file">[],
    "result": EntryLike<"section" | "container" | "unknown">[],
    "symbol": never[],
    "disasm": never[],
    "unknown": never[],
    "container": EntryLike<"package" | "disasm" | "section">[]
}

export interface EntryLike<T extends EntryType> {
    toString(): string;

    getSize(): number;

    getName(): string;

    getChildren(): EntryChildren[T]

    getID(): number;

    getType(): T;
}

class BaseImpl {
    private readonly id = orderedID()

    getID(): number {
        return this.id;
    }
}

export class SectionImpl extends BaseImpl implements EntryLike<"section"> {
    constructor(private readonly data: Section) {
        super();
    }

    getChildren(): EntryChildren["section"] {
        return [];
    }


    getName(): string {
        return this.data.name;
    }

    getSize(): number {
        return this.data.file_size - this.data.known_size;
    }

    toString(): string {
        const align = new aligner();
        align.add("Section:", this.data.name)
            .add("Size:", formatBytes(this.getSize()))
            .add("File Size:", formatBytes(this.data.file_size))
            .add("Known size:", formatBytes(this.data.known_size))
            .add("Unknown size:", formatBytes(this.getSize()))
            .add("Offset:", `0x${this.data.offset.toString(16)} - 0x${this.data.end.toString(16)}`)
            .add("Address:", `0x${this.data.addr.toString(16)} - 0x${this.data.addr_end.toString(16)}`)
            .add("Memory:", this.data.only_in_memory.toString())
            .add("Debug:", this.data.debug.toString());
        return align.toString();
    }

    getType(): "section" {
        return "section";
    }
}

export class FileImpl extends BaseImpl implements EntryLike<"file"> {
    constructor(private readonly data: File) {
        super();
    }

    getChildren(): EntryChildren["file"] {
        return [];
    }

    getName(): string {
        return this.data.file_path.split("/").pop()!;
    }

    getSize(): number {
        return this.data.size;
    }

    toString(): string {
        const align = new aligner();
        align.add("File:", this.data.file_path)
            .add("Path:", this.data.file_path)
            .add("Size:", formatBytes(this.data.size))
        if (this.data.pcln_size > 0) {
            align.add("Pcln Size:", formatBytes(this.data.pcln_size))
        }
        return align.toString();
    }

    getType(): "file" {
        return "file";
    }
}

export class PackageImpl extends BaseImpl implements EntryLike<"package"> {
    private readonly children: EntryChildren["package"];

    constructor(private readonly data: Package, private readonly parent?: string) {
        super();

        const children: EntryChildren["package"] = [];
        for (const file of data.files) {
            children.push(new FileImpl(file));
        }
        for (const subPackage of Object.values(data.subPackages)) {
            children.push(new PackageImpl(subPackage, data.name));
        }

        for (const s of data.symbols) {
            children.push(new SymbolImpl(s));
        }

        const leftSize = data.size - children.reduce((acc, child) => acc + child.getSize(), 0);
        if (leftSize > 0) {
            const name = `${data.name} Disasm`
            children.push(new DisasmImpl(name, leftSize));
        }

        this.children = children;
    }

    getChildren(): EntryChildren["package"] {
        return this.children;
    }

    getName(): string {
        if (this.parent != null) {
            return trimPrefix(this.data.name, this.parent);
        }

        return this.data.name;
    }

    getSize(): number {
        return this.data.size;
    }

    toString(): string {
        const align = new aligner();
        align.add("Package:", this.data.name)
            .add("Type:", this.data.type)
            .add("Size:", formatBytes(this.data.size));
        return align.toString();
    }

    getType(): "package" {
        return "package";
    }
}

export class DisasmImpl extends BaseImpl implements EntryLike<"disasm"> {
    constructor(private readonly name: string, private readonly size: number) {
        super();
    }

    getChildren(): EntryChildren["disasm"] {
        return [];
    }

    getName(): string {
        return this.name;
    }

    getSize(): number {
        return this.size;
    }

    toString(): string {
        const align = new aligner();
        align.add("Disasm:", this.name)
            .add("Size:", formatBytes(this.size));
        let ret = align.toString();
        ret += "\n\n" +
            "This size was not accurate." +
            "The real size determined by disassembling can be larger.";
        return ret;
    }

    getType(): "disasm" {
        return "disasm";
    }
}

export class SymbolImpl extends BaseImpl implements EntryLike<"symbol"> {
    constructor(private readonly data: FileSymbol) {
        super();
    }

    getChildren(): EntryChildren["symbol"] {
        return [];
    }

    getName(): string {
        return this.data.name;
    }

    getSize(): number {
        return this.data.size;
    }

    toString(): string {
        const align = new aligner();
        align.add("Symbol:", this.data.name)
            .add("Size:", formatBytes(this.data.size))
            .add("Address:", `0x${this.data.addr.toString(16)}`)
            .add("Type:", this.data.type);
        return align.toString();
    }

    getType(): "symbol" {
        return "symbol";
    }
}

export class ContainerImpl extends BaseImpl implements EntryLike<"container"> {
    constructor(private readonly name: string,
                private readonly size: number,
                private readonly children: EntryChildren["container"],
                private readonly explain: string = "") {
        super();
    }

    getChildren(): EntryChildren["container"] {
        return this.children;
    }

    getName(): string {
        return this.name;
    }

    getSize(): number {
        return this.size;
    }

    toString(): string {
        let ret = this.explain + "\n"
        const align = new aligner();
        align.add("Size:", formatBytes(this.size));
        ret += "\n" + align.toString();
        return ret;
    }

    getType(): "container" {
        return "container";
    }
}

export class UnknownImpl extends BaseImpl implements EntryLike<"unknown"> {
    constructor(private readonly size: number) {
        super();
    }

    getChildren(): EntryChildren["unknown"] {
        return [];
    }

    getName(): string {
        return "Unknown";
    }

    getSize(): number {
        return this.size;
    }

    toString(): string {
        const align = new aligner();
        align.add("Size:", formatBytes(this.size));
        let ret = align.toString();
        ret += "\n\n" +
            "The unknown part in the binary.\n" +
            "Can be ELF Header, Program Header, align offset...\n" +
            "We just don't know.";
        return ret;
    }

    getType(): "unknown" {
        return "unknown";
    }
}

export class ResultImpl extends BaseImpl implements EntryLike<"result"> {
    private readonly children: EntryChildren["result"];

    constructor(private readonly data: Result) {
        super();

        const children: EntryChildren["result"] = [];

        const sectionContainerChildren: EntryLike<"section">[] = []
        for (const section of data.sections) {
            sectionContainerChildren.push(new SectionImpl(section));
        }
        const sectionContainerSize = sectionContainerChildren.reduce((acc, child) => acc + child.getSize(), 0);
        const sectionContainer = new ContainerImpl(
            "Unknown Sections Size",
            sectionContainerSize,
            sectionContainerChildren,
            "The unknown size of the sections in the binary.");
        children.push(sectionContainer);

        const typedPackages: Record<string, Package[]> = {};
        for (const pkg of Object.values(data.packages)) {
            if (typedPackages[pkg.type] == null) {
                typedPackages[pkg.type] = [];
            }
            typedPackages[pkg.type].push(pkg);
        }
        const typedPackagesChildren: EntryLike<"container">[] = [];
        for (const [type, packages] of Object.entries(typedPackages)) {
            const packageContainerChildren: EntryLike<"package" | "disasm">[] = [];
            for (const pkg of packages) {
                packageContainerChildren.push(new PackageImpl(pkg));
            }
            const packageContainerSize = packageContainerChildren.reduce((acc, child) => acc + child.getSize(), 0);
            const packageContainer = new ContainerImpl(
                `${title(type)} Packages Size`,
                packageContainerSize,
                packageContainerChildren,
                `The size of the ${type} packages in the binary.`
            )

            typedPackagesChildren.push(packageContainer);
        }
        children.push(...typedPackagesChildren);

        const leftSize = data.size - children.reduce((acc, child) => acc + child.getSize(), 0);
        if (leftSize > 0) {
            children.push(new UnknownImpl(leftSize));
        }

        this.children = children;
    }

    getChildren(): EntryChildren["result"] {
        return this.children;
    }

    getName(): string {
        return this.data.name;
    }

    getSize(): number {
        return this.data.size;
    }

    toString(): string {
        const align = new aligner();
        align.add("Result:", this.data.name)
            .add("Size:", formatBytes(this.data.size));
        return align.toString();
    }

    getType(): "result" {
        return "result";
    }
}

export type Entry = EntryLike<EntryType>;

export function createEntry(data: Result): Entry {
    return new ResultImpl(data);
}