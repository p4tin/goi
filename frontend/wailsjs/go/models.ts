export namespace dap {
	
	export class EvaluateResponse {
	    result: string;
	    type: string;
	    variablesReference: number;
	    namedVariables: number;
	    indexedVariables: number;
	
	    static createFrom(source: any = {}) {
	        return new EvaluateResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.result = source["result"];
	        this.type = source["type"];
	        this.variablesReference = source["variablesReference"];
	        this.namedVariables = source["namedVariables"];
	        this.indexedVariables = source["indexedVariables"];
	    }
	}
	export class ExceptionInfoResponse {
	    exceptionId: string;
	    description: string;
	    breakMode: string;
	    // Go type: struct { Message string "json:\"message\""; TypeName string "json:\"typeName\""; StackTrace string "json:\"stackTrace\"" }
	    details?: any;
	
	    static createFrom(source: any = {}) {
	        return new ExceptionInfoResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exceptionId = source["exceptionId"];
	        this.description = source["description"];
	        this.breakMode = source["breakMode"];
	        this.details = this.convertValues(source["details"], Object);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Scope {
	    name: string;
	    variablesReference: number;
	    expensive: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Scope(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.variablesReference = source["variablesReference"];
	        this.expensive = source["expensive"];
	    }
	}
	export class SetVariableResponse {
	    value: string;
	    type: string;
	    variablesReference: number;
	
	    static createFrom(source: any = {}) {
	        return new SetVariableResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.type = source["type"];
	        this.variablesReference = source["variablesReference"];
	    }
	}
	export class Source {
	    name?: string;
	    path?: string;
	    sourceReference?: number;
	
	    static createFrom(source: any = {}) {
	        return new Source(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.sourceReference = source["sourceReference"];
	    }
	}
	export class StackFrame {
	    id: number;
	    name: string;
	    source?: Source;
	    line: number;
	    column: number;
	    presentationHint?: string;
	    instructionPointerReference?: string;
	
	    static createFrom(source: any = {}) {
	        return new StackFrame(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.source = this.convertValues(source["source"], Source);
	        this.line = source["line"];
	        this.column = source["column"];
	        this.presentationHint = source["presentationHint"];
	        this.instructionPointerReference = source["instructionPointerReference"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StackTraceResponse {
	    stackFrames: StackFrame[];
	    totalFrames: number;
	
	    static createFrom(source: any = {}) {
	        return new StackTraceResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stackFrames = this.convertValues(source["stackFrames"], StackFrame);
	        this.totalFrames = source["totalFrames"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Thread {
	    id: number;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new Thread(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class Variable {
	    name: string;
	    value: string;
	    type: string;
	    evaluateName: string;
	    variablesReference: number;
	    namedVariables: number;
	    indexedVariables: number;
	
	    static createFrom(source: any = {}) {
	        return new Variable(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	        this.type = source["type"];
	        this.evaluateName = source["evaluateName"];
	        this.variablesReference = source["variablesReference"];
	        this.namedVariables = source["namedVariables"];
	        this.indexedVariables = source["indexedVariables"];
	    }
	}

}

export namespace format {
	
	export class SyntaxError {
	    line: number;
	    col: number;
	    msg: string;
	
	    static createFrom(source: any = {}) {
	        return new SyntaxError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = source["line"];
	        this.col = source["col"];
	        this.msg = source["msg"];
	    }
	}

}

export namespace git {
	
	export class BlameLine {
	    line: number;
	    hash: string;
	    author: string;
	    date: string;
	    summary: string;
	
	    static createFrom(source: any = {}) {
	        return new BlameLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = source["line"];
	        this.hash = source["hash"];
	        this.author = source["author"];
	        this.date = source["date"];
	        this.summary = source["summary"];
	    }
	}
	export class Branch {
	    name: string;
	    remote: boolean;
	    current: boolean;
	    upstream: string;
	    hash: string;
	    subject: string;
	    date: string;
	
	    static createFrom(source: any = {}) {
	        return new Branch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.remote = source["remote"];
	        this.current = source["current"];
	        this.upstream = source["upstream"];
	        this.hash = source["hash"];
	        this.subject = source["subject"];
	        this.date = source["date"];
	    }
	}
	export class ChangedFile {
	    path: string;
	    rel: string;
	    status: string;
	    orig: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.rel = source["rel"];
	        this.status = source["status"];
	        this.orig = source["orig"];
	    }
	}
	export class Commit {
	    hash: string;
	    short: string;
	    parents: string[];
	    author: string;
	    email: string;
	    date: string;
	    subject: string;
	    body: string;
	    refs: string;
	
	    static createFrom(source: any = {}) {
	        return new Commit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.short = source["short"];
	        this.parents = source["parents"];
	        this.author = source["author"];
	        this.email = source["email"];
	        this.date = source["date"];
	        this.subject = source["subject"];
	        this.body = source["body"];
	        this.refs = source["refs"];
	    }
	}
	export class Hunk {
	    oldStart: number;
	    oldLines: number;
	    newStart: number;
	    newLines: number;
	    header: string;
	    lines: string[];
	
	    static createFrom(source: any = {}) {
	        return new Hunk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.oldStart = source["oldStart"];
	        this.oldLines = source["oldLines"];
	        this.newStart = source["newStart"];
	        this.newLines = source["newLines"];
	        this.header = source["header"];
	        this.lines = source["lines"];
	    }
	}
	export class FileDiff {
	    oldPath: string;
	    newPath: string;
	    header: string[];
	    hunks: Hunk[];
	    binary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.oldPath = source["oldPath"];
	        this.newPath = source["newPath"];
	        this.header = source["header"];
	        this.hunks = this.convertValues(source["hunks"], Hunk);
	        this.binary = source["binary"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FileStatus {
	    path: string;
	    rel: string;
	    orig: string;
	    index: string;
	    work: string;
	    conflict: boolean;
	    untracked: boolean;
	    ignored: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.rel = source["rel"];
	        this.orig = source["orig"];
	        this.index = source["index"];
	        this.work = source["work"];
	        this.conflict = source["conflict"];
	        this.untracked = source["untracked"];
	        this.ignored = source["ignored"];
	    }
	}
	
	export class LogQuery {
	    path: string;
	    skip: number;
	    max: number;
	    grep: string;
	    ref: string;
	
	    static createFrom(source: any = {}) {
	        return new LogQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.skip = source["skip"];
	        this.max = source["max"];
	        this.grep = source["grep"];
	        this.ref = source["ref"];
	    }
	}
	export class Stash {
	    index: number;
	    message: string;
	    date: string;
	
	    static createFrom(source: any = {}) {
	        return new Stash(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.message = source["message"];
	        this.date = source["date"];
	    }
	}
	export class Status {
	    root: string;
	    branch: string;
	    head: string;
	    upstream: string;
	    ahead: number;
	    behind: number;
	    detached: boolean;
	    initial: boolean;
	    op: string;
	    files: FileStatus[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.branch = source["branch"];
	        this.head = source["head"];
	        this.upstream = source["upstream"];
	        this.ahead = source["ahead"];
	        this.behind = source["behind"];
	        this.detached = source["detached"];
	        this.initial = source["initial"];
	        this.op = source["op"];
	        this.files = this.convertValues(source["files"], FileStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace gopls {
	
	export class FileEdit {
	    path: string;
	    edits: lsp.TextEdit[];
	
	    static createFrom(source: any = {}) {
	        return new FileEdit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.edits = this.convertValues(source["edits"], lsp.TextEdit);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Location {
	    path: string;
	    range: lsp.Range;
	
	    static createFrom(source: any = {}) {
	        return new Location(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.range = this.convertValues(source["range"], lsp.Range);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Symbol {
	    name: string;
	    kind: number;
	    containerName: string;
	    location: Location;
	
	    static createFrom(source: any = {}) {
	        return new Symbol(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.containerName = source["containerName"];
	        this.location = this.convertValues(source["location"], Location);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace gosdk {
	
	export class SDK {
	    goroot: string;
	    version: string;
	    goBin: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new SDK(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.goroot = source["goroot"];
	        this.version = source["version"];
	        this.goBin = source["goBin"];
	        this.source = source["source"];
	    }
	}

}

export namespace gosyntax {
	
	export class RunMarker {
	    kind: string;
	    name: string;
	    line: number;
	
	    static createFrom(source: any = {}) {
	        return new RunMarker(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.line = source["line"];
	    }
	}

}

export namespace index {
	
	export class Match {
	    path: string;
	    rel: string;
	    root: string;
	    score: number;
	    positions: number[];
	
	    static createFrom(source: any = {}) {
	        return new Match(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.rel = source["rel"];
	        this.root = source["root"];
	        this.score = source["score"];
	        this.positions = source["positions"];
	    }
	}

}

export namespace journal {
	
	export class Entry {
	    path: string;
	    content: string;
	    // Go type: time
	    baseModTime: any;
	    // Go type: time
	    savedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	        this.baseModTime = this.convertValues(source["baseModTime"], null);
	        this.savedAt = this.convertValues(source["savedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace lsp {
	
	export class Command {
	    title: string;
	    command: string;
	    arguments?: any[];
	
	    static createFrom(source: any = {}) {
	        return new Command(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.command = source["command"];
	        this.arguments = source["arguments"];
	    }
	}
	export class TextEdit {
	    range: Range;
	    newText: string;
	
	    static createFrom(source: any = {}) {
	        return new TextEdit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.range = this.convertValues(source["range"], Range);
	        this.newText = source["newText"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VersionedTextDocumentIdentifier {
	    uri: string;
	    version?: number;
	
	    static createFrom(source: any = {}) {
	        return new VersionedTextDocumentIdentifier(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.version = source["version"];
	    }
	}
	export class TextDocumentEdit {
	    textDocument: VersionedTextDocumentIdentifier;
	    edits: TextEdit[];
	
	    static createFrom(source: any = {}) {
	        return new TextDocumentEdit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.textDocument = this.convertValues(source["textDocument"], VersionedTextDocumentIdentifier);
	        this.edits = this.convertValues(source["edits"], TextEdit);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceEdit {
	    changes?: Record<string, Array<TextEdit>>;
	    documentChanges?: TextDocumentEdit[];
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceEdit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.changes = this.convertValues(source["changes"], Array<TextEdit>, true);
	        this.documentChanges = this.convertValues(source["documentChanges"], TextDocumentEdit);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Disabled {
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new Disabled(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reason = source["reason"];
	    }
	}
	export class Location {
	    uri: string;
	    range: Range;
	
	    static createFrom(source: any = {}) {
	        return new Location(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.range = this.convertValues(source["range"], Range);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DiagnosticRelatedInformation {
	    location: Location;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticRelatedInformation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.location = this.convertValues(source["location"], Location);
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Position {
	    line: number;
	    character: number;
	
	    static createFrom(source: any = {}) {
	        return new Position(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = source["line"];
	        this.character = source["character"];
	    }
	}
	export class Range {
	    start: Position;
	    end: Position;
	
	    static createFrom(source: any = {}) {
	        return new Range(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = this.convertValues(source["start"], Position);
	        this.end = this.convertValues(source["end"], Position);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Diagnostic {
	    range: Range;
	    severity?: number;
	    code?: any;
	    source?: string;
	    message: string;
	    tags?: number[];
	    relatedInformation?: DiagnosticRelatedInformation[];
	    data?: any;
	
	    static createFrom(source: any = {}) {
	        return new Diagnostic(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.range = this.convertValues(source["range"], Range);
	        this.severity = source["severity"];
	        this.code = source["code"];
	        this.source = source["source"];
	        this.message = source["message"];
	        this.tags = source["tags"];
	        this.relatedInformation = this.convertValues(source["relatedInformation"], DiagnosticRelatedInformation);
	        this.data = source["data"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CodeAction {
	    title: string;
	    kind?: string;
	    diagnostics?: Diagnostic[];
	    isPreferred?: boolean;
	    disabled?: Disabled;
	    edit?: WorkspaceEdit;
	    command?: Command;
	    data?: any;
	
	    static createFrom(source: any = {}) {
	        return new CodeAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.kind = source["kind"];
	        this.diagnostics = this.convertValues(source["diagnostics"], Diagnostic);
	        this.isPreferred = source["isPreferred"];
	        this.disabled = this.convertValues(source["disabled"], Disabled);
	        this.edit = this.convertValues(source["edit"], WorkspaceEdit);
	        this.command = this.convertValues(source["command"], Command);
	        this.data = source["data"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class CompletionItem {
	    label: string;
	    kind?: number;
	    detail?: string;
	    documentation?: string;
	    deprecated?: boolean;
	    tags?: number[];
	    preselect?: boolean;
	    sortText?: string;
	    filterText?: string;
	    insertText?: string;
	    insertTextFormat?: number;
	    textEdit?: TextEdit;
	    additionalTextEdits?: TextEdit[];
	    command?: Command;
	
	    static createFrom(source: any = {}) {
	        return new CompletionItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.detail = source["detail"];
	        this.documentation = source["documentation"];
	        this.deprecated = source["deprecated"];
	        this.tags = source["tags"];
	        this.preselect = source["preselect"];
	        this.sortText = source["sortText"];
	        this.filterText = source["filterText"];
	        this.insertText = source["insertText"];
	        this.insertTextFormat = source["insertTextFormat"];
	        this.textEdit = this.convertValues(source["textEdit"], TextEdit);
	        this.additionalTextEdits = this.convertValues(source["additionalTextEdits"], TextEdit);
	        this.command = this.convertValues(source["command"], Command);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CompletionList {
	    isIncomplete: boolean;
	    items: CompletionItem[];
	
	    static createFrom(source: any = {}) {
	        return new CompletionList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isIncomplete = source["isIncomplete"];
	        this.items = this.convertValues(source["items"], CompletionItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class DocumentHighlight {
	    range: Range;
	    kind?: number;
	
	    static createFrom(source: any = {}) {
	        return new DocumentHighlight(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.range = this.convertValues(source["range"], Range);
	        this.kind = source["kind"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DocumentSymbol {
	    name: string;
	    detail?: string;
	    kind: number;
	    range: Range;
	    selectionRange: Range;
	    children?: DocumentSymbol[];
	
	    static createFrom(source: any = {}) {
	        return new DocumentSymbol(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.detail = source["detail"];
	        this.kind = source["kind"];
	        this.range = this.convertValues(source["range"], Range);
	        this.selectionRange = this.convertValues(source["selectionRange"], Range);
	        this.children = this.convertValues(source["children"], DocumentSymbol);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ParameterInformation {
	    label: string;
	    start: number;
	    end: number;
	    documentation?: string;
	
	    static createFrom(source: any = {}) {
	        return new ParameterInformation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.start = source["start"];
	        this.end = source["end"];
	        this.documentation = source["documentation"];
	    }
	}
	
	export class PrepareRenameResult {
	    ok: boolean;
	    range: Range;
	    placeholder: string;
	
	    static createFrom(source: any = {}) {
	        return new PrepareRenameResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.range = this.convertValues(source["range"], Range);
	        this.placeholder = source["placeholder"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SignatureInformation {
	    label: string;
	    documentation?: string;
	    parameters: ParameterInformation[];
	    activeParameter?: number;
	
	    static createFrom(source: any = {}) {
	        return new SignatureInformation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.documentation = source["documentation"];
	        this.parameters = this.convertValues(source["parameters"], ParameterInformation);
	        this.activeParameter = source["activeParameter"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SignatureHelp {
	    signatures: SignatureInformation[];
	    activeSignature: number;
	    activeParameter: number;
	
	    static createFrom(source: any = {}) {
	        return new SignatureHelp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.signatures = this.convertValues(source["signatures"], SignatureInformation);
	        this.activeSignature = source["activeSignature"];
	        this.activeParameter = source["activeParameter"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	

}

export namespace process {
	
	export class ExitInfo {
	    code: number;
	    signal: string;
	    err: string;
	
	    static createFrom(source: any = {}) {
	        return new ExitInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.signal = source["signal"];
	        this.err = source["err"];
	    }
	}

}

export namespace recent {
	
	export class Entry {
	    path: string;
	    name: string;
	    kind: string;
	    // Go type: time
	    openedAt: any;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.openedAt = this.convertValues(source["openedAt"], null);
	        this.exists = source["exists"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace rpc {
	
	export class AppInfo {
	    name: string;
	    version: string;
	    build: string;
	    goVersion: string;
	    os: string;
	    arch: string;
	    projectPath: string;
	    logPath: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.build = source["build"];
	        this.goVersion = source["goVersion"];
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.projectPath = source["projectPath"];
	        this.logPath = source["logPath"];
	    }
	}
	export class BufferText {
	    path: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new BufferText(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.text = source["text"];
	    }
	}
	export class ContextRequest {
	    path: string;
	    kind: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new ContextRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	    }
	}
	export class DlvStatus {
	    path: string;
	    found: boolean;
	    installing: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new DlvStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.found = source["found"];
	        this.installing = source["installing"];
	        this.message = source["message"];
	    }
	}
	export class IndexStatus {
	    ready: boolean;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new IndexStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.count = source["count"];
	    }
	}
	export class FileSearchResult {
	    matches: index.Match[];
	    index: IndexStatus;
	
	    static createFrom(source: any = {}) {
	        return new FileSearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.matches = this.convertValues(source["matches"], index.Match);
	        this.index = this.convertValues(source["index"], IndexStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FindQuery {
	    pattern: string;
	    regex: boolean;
	    caseSensitive: boolean;
	    wholeWord: boolean;
	    mask: string;
	    replace: string;
	    scope: string;
	    buffers: BufferText[];
	
	    static createFrom(source: any = {}) {
	        return new FindQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pattern = source["pattern"];
	        this.regex = source["regex"];
	        this.caseSensitive = source["caseSensitive"];
	        this.wholeWord = source["wholeWord"];
	        this.mask = source["mask"];
	        this.replace = source["replace"];
	        this.scope = source["scope"];
	        this.buffers = this.convertValues(source["buffers"], BufferText);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FindResult {
	    matches: textsearch.Match[];
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FindResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.matches = this.convertValues(source["matches"], textsearch.Match);
	        this.truncated = source["truncated"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FormatResult {
	    content: string;
	    changed: boolean;
	    syntaxError?: format.SyntaxError;
	
	    static createFrom(source: any = {}) {
	        return new FormatResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.content = source["content"];
	        this.changed = source["changed"];
	        this.syntaxError = this.convertValues(source["syntaxError"], format.SyntaxError);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FunctionBreakpoint {
	    name: string;
	    condition: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FunctionBreakpoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.condition = source["condition"];
	        this.enabled = source["enabled"];
	    }
	}
	export class GitState {
	    repo: boolean;
	    status: git.Status;
	
	    static createFrom(source: any = {}) {
	        return new GitState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo = source["repo"];
	        this.status = this.convertValues(source["status"], git.Status);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GoEnvSummary {
	    goos: string;
	    goarch: string;
	    goversion: string;
	    cross: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GoEnvSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.goos = source["goos"];
	        this.goarch = source["goarch"];
	        this.goversion = source["goversion"];
	        this.cross = source["cross"];
	    }
	}
	export class HoverResult {
	    markdown: string;
	    range?: lsp.Range;
	
	    static createFrom(source: any = {}) {
	        return new HoverResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.markdown = source["markdown"];
	        this.range = this.convertValues(source["range"], lsp.Range);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class LangStatus {
	    state: string;
	    message: string;
	    bin: string;
	
	    static createFrom(source: any = {}) {
	        return new LangStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.message = source["message"];
	        this.bin = source["bin"];
	    }
	}
	export class LineBreakpoint {
	    line: number;
	    condition: string;
	    hitCondition: string;
	    logMessage: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LineBreakpoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = source["line"];
	        this.condition = source["condition"];
	        this.hitCondition = source["hitCondition"];
	        this.logMessage = source["logMessage"];
	        this.enabled = source["enabled"];
	    }
	}
	export class LintResult {
	    issues: number;
	    files: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new LintResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.issues = source["issues"];
	        this.files = source["files"];
	        this.error = source["error"];
	    }
	}
	export class LintStatus {
	    found: boolean;
	    bin: string;
	    version: string;
	    installing: boolean;
	    running: number;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new LintStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.bin = source["bin"];
	        this.version = source["version"];
	        this.installing = source["installing"];
	        this.running = source["running"];
	        this.message = source["message"];
	    }
	}
	export class MenuItemSpec {
	    label: string;
	    command: string;
	    accel: string;
	    separator: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MenuItemSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.command = source["command"];
	        this.accel = source["accel"];
	        this.separator = source["separator"];
	    }
	}
	export class MenuSpec {
	    label: string;
	    items: MenuItemSpec[];
	
	    static createFrom(source: any = {}) {
	        return new MenuSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.items = this.convertValues(source["items"], MenuItemSpec);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OpenedFile {
	    meta: vfs.FileMeta;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new OpenedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.meta = this.convertValues(source["meta"], vfs.FileMeta);
	        this.content = source["content"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RunInfo {
	    id: string;
	    config: runconfig.Config;
	    state: string;
	    debug: boolean;
	    exit?: process.ExitInfo;
	    elapsedMs: number;
	    message: string;
	    command: string;
	    test: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RunInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.config = this.convertValues(source["config"], runconfig.Config);
	        this.state = source["state"];
	        this.debug = source["debug"];
	        this.exit = this.convertValues(source["exit"], process.ExitInfo);
	        this.elapsedMs = source["elapsedMs"];
	        this.message = source["message"];
	        this.command = source["command"];
	        this.test = source["test"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SDKStatus {
	    ok: boolean;
	    sdk: gosdk.SDK;
	    problem: string;
	
	    static createFrom(source: any = {}) {
	        return new SDKStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.sdk = this.convertValues(source["sdk"], gosdk.SDK);
	        this.problem = source["problem"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SettingsPaths {
	    user: string;
	    project: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsPaths(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = source["user"];
	        this.project = source["project"];
	    }
	}
	export class ShowResult {
	    text: string;
	    found: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ShowResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.found = source["found"];
	    }
	}
	export class TermInfo {
	    id: string;
	    shell: string;
	    cwd: string;
	
	    static createFrom(source: any = {}) {
	        return new TermInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.shell = source["shell"];
	        this.cwd = source["cwd"];
	    }
	}

}

export namespace runconfig {
	
	export class Task {
	    type: string;
	    config: string;
	    package: string;
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.config = source["config"];
	        this.package = source["package"];
	        this.command = source["command"];
	    }
	}
	export class Config {
	    name: string;
	    type: string;
	    shared: boolean;
	    temporary: boolean;
	    created: number;
	    kind: string;
	    package: string;
	    directory: string;
	    files: string[];
	    packages: string[];
	    pattern: string;
	    workingDir: string;
	    args: string;
	    env: Record<string, string>;
	    envFile: string;
	    buildFlags: string;
	    tags: string[];
	    output: string;
	    runAfterBuild?: boolean;
	    inputFile: string;
	    race: boolean;
	    short: boolean;
	    count: number;
	    timeout: string;
	    testFlags: string;
	    bench: string;
	    benchmem: boolean;
	    fuzz: string;
	    fuzzTime: string;
	    host: string;
	    port: number;
	    script: string;
	    scriptText: string;
	    interpreter: string;
	    makefile: string;
	    targets: string[];
	    beforeLaunch: Task[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.shared = source["shared"];
	        this.temporary = source["temporary"];
	        this.created = source["created"];
	        this.kind = source["kind"];
	        this.package = source["package"];
	        this.directory = source["directory"];
	        this.files = source["files"];
	        this.packages = source["packages"];
	        this.pattern = source["pattern"];
	        this.workingDir = source["workingDir"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.envFile = source["envFile"];
	        this.buildFlags = source["buildFlags"];
	        this.tags = source["tags"];
	        this.output = source["output"];
	        this.runAfterBuild = source["runAfterBuild"];
	        this.inputFile = source["inputFile"];
	        this.race = source["race"];
	        this.short = source["short"];
	        this.count = source["count"];
	        this.timeout = source["timeout"];
	        this.testFlags = source["testFlags"];
	        this.bench = source["bench"];
	        this.benchmem = source["benchmem"];
	        this.fuzz = source["fuzz"];
	        this.fuzzTime = source["fuzzTime"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.script = source["script"];
	        this.scriptText = source["scriptText"];
	        this.interpreter = source["interpreter"];
	        this.makefile = source["makefile"];
	        this.targets = source["targets"];
	        this.beforeLaunch = this.convertValues(source["beforeLaunch"], Task);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace settings {
	
	export class Debug {
	    dlvPath: string;
	    showTypes: boolean;
	    showAddresses: boolean;
	    showUnreadable: boolean;
	    inlineValues: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Debug(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dlvPath = source["dlvPath"];
	        this.showTypes = source["showTypes"];
	        this.showAddresses = source["showAddresses"];
	        this.showUnreadable = source["showUnreadable"];
	        this.inlineValues = source["inlineValues"];
	    }
	}
	export class Editor {
	    fontFamily: string;
	    fontSize: number;
	    tabSize: number;
	    softWrap: boolean;
	    maxHighlightSizeKB: number;
	    maxOpenSizeKB: number;
	
	    static createFrom(source: any = {}) {
	        return new Editor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fontFamily = source["fontFamily"];
	        this.fontSize = source["fontSize"];
	        this.tabSize = source["tabSize"];
	        this.softWrap = source["softWrap"];
	        this.maxHighlightSizeKB = source["maxHighlightSizeKB"];
	        this.maxOpenSizeKB = source["maxOpenSizeKB"];
	    }
	}
	export class Git {
	    path: string;
	    pullRebase: boolean;
	    autoFetchMinutes: number;
	
	    static createFrom(source: any = {}) {
	        return new Git(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.pullRebase = source["pullRebase"];
	        this.autoFetchMinutes = source["autoFetchMinutes"];
	    }
	}
	export class Go {
	    goroot: string;
	    env: Record<string, string>;
	    tags: string[];
	    goos: string;
	    goarch: string;
	    cgo: string;
	    vendor: string;
	
	    static createFrom(source: any = {}) {
	        return new Go(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.goroot = source["goroot"];
	        this.env = source["env"];
	        this.tags = source["tags"];
	        this.goos = source["goos"];
	        this.goarch = source["goarch"];
	        this.cgo = source["cgo"];
	        this.vendor = source["vendor"];
	    }
	}
	export class Gopls {
	    enabled: boolean;
	    path: string;
	    usePlaceholders: boolean;
	    staticcheck: boolean;
	    settings: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new Gopls(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.path = source["path"];
	        this.usePlaceholders = source["usePlaceholders"];
	        this.staticcheck = source["staticcheck"];
	        this.settings = source["settings"];
	    }
	}
	export class Lint {
	    enabled: boolean;
	    onSave: boolean;
	    golangciPath: string;
	    args: string[];
	
	    static createFrom(source: any = {}) {
	        return new Lint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.onSave = source["onSave"];
	        this.golangciPath = source["golangciPath"];
	        this.args = source["args"];
	    }
	}
	export class Project {
	    contentRoots: string[];
	    excludes: string[];
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.contentRoots = source["contentRoots"];
	        this.excludes = source["excludes"];
	    }
	}
	export class Run {
	    outputLimitLines: number;
	
	    static createFrom(source: any = {}) {
	        return new Run(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outputLimitLines = source["outputLimitLines"];
	    }
	}
	export class Save {
	    format: string;
	
	    static createFrom(source: any = {}) {
	        return new Save(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	    }
	}
	export class UI {
	    theme: string;
	    scheme: string;
	    keymap: string;
	
	    static createFrom(source: any = {}) {
	        return new UI(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.scheme = source["scheme"];
	        this.keymap = source["keymap"];
	    }
	}
	export class Terminal {
	    shell: string;
	    shellIntegration: boolean;
	    fontSize: number;
	    scrollback: number;
	
	    static createFrom(source: any = {}) {
	        return new Terminal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.shell = source["shell"];
	        this.shellIntegration = source["shellIntegration"];
	        this.fontSize = source["fontSize"];
	        this.scrollback = source["scrollback"];
	    }
	}
	export class Startup {
	    reopenLastProject: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Startup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reopenLastProject = source["reopenLastProject"];
	    }
	}
	export class Settings {
	    startup: Startup;
	    editor: Editor;
	    go: Go;
	    project: Project;
	    save: Save;
	    gopls: Gopls;
	    run: Run;
	    debug: Debug;
	    terminal: Terminal;
	    git: Git;
	    lint: Lint;
	    ui: UI;
	    keys: Record<string, Array<string>>;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.startup = this.convertValues(source["startup"], Startup);
	        this.editor = this.convertValues(source["editor"], Editor);
	        this.go = this.convertValues(source["go"], Go);
	        this.project = this.convertValues(source["project"], Project);
	        this.save = this.convertValues(source["save"], Save);
	        this.gopls = this.convertValues(source["gopls"], Gopls);
	        this.run = this.convertValues(source["run"], Run);
	        this.debug = this.convertValues(source["debug"], Debug);
	        this.terminal = this.convertValues(source["terminal"], Terminal);
	        this.git = this.convertValues(source["git"], Git);
	        this.lint = this.convertValues(source["lint"], Lint);
	        this.ui = this.convertValues(source["ui"], UI);
	        this.keys = source["keys"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	

}

export namespace textsearch {
	
	export class Match {
	    path: string;
	    line: number;
	    col: number;
	    end: number;
	    preview: string;
	    previewStart: number;
	
	    static createFrom(source: any = {}) {
	        return new Match(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.line = source["line"];
	        this.col = source["col"];
	        this.end = source["end"];
	        this.preview = source["preview"];
	        this.previewStart = source["previewStart"];
	    }
	}

}

export namespace vfs {
	
	export class Entry {
	    name: string;
	    path: string;
	    isDir: boolean;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	    }
	}
	export class FileMeta {
	    path: string;
	    size: number;
	    // Go type: time
	    modTime: any;
	    binary: boolean;
	    eol: string;
	    bom: boolean;
	    finalNewline: boolean;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new FileMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.size = source["size"];
	        this.modTime = this.convertValues(source["modTime"], null);
	        this.binary = source["binary"];
	        this.eol = source["eol"];
	        this.bom = source["bom"];
	        this.finalNewline = source["finalNewline"];
	        this.mode = source["mode"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace workspace {
	
	export class Module {
	    path: string;
	    dir: string;
	    goVersion: string;
	
	    static createFrom(source: any = {}) {
	        return new Module(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.dir = source["dir"];
	        this.goVersion = source["goVersion"];
	    }
	}
	export class ParseError {
	    file: string;
	    line: number;
	    msg: string;
	
	    static createFrom(source: any = {}) {
	        return new ParseError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.line = source["line"];
	        this.msg = source["msg"];
	    }
	}
	export class Project {
	    root: string;
	    name: string;
	    kind: string;
	    modules: Module[];
	    contentRoots: string[];
	    excludes: string[];
	    hash: string;
	    parseErrors: ParseError[];
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.modules = this.convertValues(source["modules"], Module);
	        this.contentRoots = source["contentRoots"];
	        this.excludes = source["excludes"];
	        this.hash = source["hash"];
	        this.parseErrors = this.convertValues(source["parseErrors"], ParseError);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

