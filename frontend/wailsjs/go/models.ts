export namespace main {
	
	export class CliStatus {
	    found: boolean;
	    path: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new CliStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.path = source["path"];
	        this.version = source["version"];
	    }
	}
	export class SettingsInput {
	    backend: string;
	    baseURL: string;
	    apiKey: string;
	    model: string;
	    temperature: number;
	    hasTemperature: boolean;
	    maxSteps: number;
	    contextBudgetTokens: number;
	    autoApprove: boolean;
	    lastWorkspace: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backend = source["backend"];
	        this.baseURL = source["baseURL"];
	        this.apiKey = source["apiKey"];
	        this.model = source["model"];
	        this.temperature = source["temperature"];
	        this.hasTemperature = source["hasTemperature"];
	        this.maxSteps = source["maxSteps"];
	        this.contextBudgetTokens = source["contextBudgetTokens"];
	        this.autoApprove = source["autoApprove"];
	        this.lastWorkspace = source["lastWorkspace"];
	    }
	}
	export class SettingsView {
	    backend: string;
	    baseURL: string;
	    model: string;
	    temperature: number;
	    hasTemperature: boolean;
	    maxSteps: number;
	    contextBudgetTokens: number;
	    autoApprove: boolean;
	    lastWorkspace: string;
	    hasAPIKey: boolean;
	    apiKeyLast4: string;
	    apiKeyFromEnv: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SettingsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backend = source["backend"];
	        this.baseURL = source["baseURL"];
	        this.model = source["model"];
	        this.temperature = source["temperature"];
	        this.hasTemperature = source["hasTemperature"];
	        this.maxSteps = source["maxSteps"];
	        this.contextBudgetTokens = source["contextBudgetTokens"];
	        this.autoApprove = source["autoApprove"];
	        this.lastWorkspace = source["lastWorkspace"];
	        this.hasAPIKey = source["hasAPIKey"];
	        this.apiKeyLast4 = source["apiKeyLast4"];
	        this.apiKeyFromEnv = source["apiKeyFromEnv"];
	    }
	}

}

export namespace provider {
	
	export class ToolCall {
	    id: string;
	    name: string;
	    arguments: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.arguments = source["arguments"];
	    }
	}
	export class Message {
	    role: string;
	    content?: string;
	    model?: string;
	    tool_call_id?: string;
	    name?: string;
	    tool_calls?: ToolCall[];
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.model = source["model"];
	        this.tool_call_id = source["tool_call_id"];
	        this.name = source["name"];
	        this.tool_calls = this.convertValues(source["tool_calls"], ToolCall);
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
	
	export class Usage {
	    promptTokens: number;
	    completionTokens: number;
	    totalTokens: number;
	    cacheHitTokens: number;
	    cacheMissTokens: number;
	
	    static createFrom(source: any = {}) {
	        return new Usage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.cacheHitTokens = source["cacheHitTokens"];
	        this.cacheMissTokens = source["cacheMissTokens"];
	    }
	}

}

export namespace session {
	
	export class Meta {
	    id: string;
	    title: string;
	    workspace: string;
	    model: string;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Meta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.workspace = source["workspace"];
	        this.model = source["model"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class Session {
	    id: string;
	    title: string;
	    workspace: string;
	    model: string;
	    cliSessionID?: string;
	    cliSeeded?: boolean;
	    cliSeedV?: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    messages: provider.Message[];
	    compactedMsgs?: number;
	    compactedTurns?: number;
	    totalUsage?: provider.Usage;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.workspace = source["workspace"];
	        this.model = source["model"];
	        this.cliSessionID = source["cliSessionID"];
	        this.cliSeeded = source["cliSeeded"];
	        this.cliSeedV = source["cliSeedV"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.messages = this.convertValues(source["messages"], provider.Message);
	        this.compactedMsgs = source["compactedMsgs"];
	        this.compactedTurns = source["compactedTurns"];
	        this.totalUsage = this.convertValues(source["totalUsage"], provider.Usage);
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

