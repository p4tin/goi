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

}

