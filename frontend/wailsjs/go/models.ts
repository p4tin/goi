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
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.projectPath = source["projectPath"];
	        this.logPath = source["logPath"];
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

}

export namespace settings {
	
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

