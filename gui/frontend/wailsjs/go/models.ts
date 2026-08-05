export namespace main {
	
	export class AddTailnetRequest {
	    id: string;
	    authKey: string;
	    controlUrl: string;
	    exitNode: string;
	    hostAccess: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AddTailnetRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.authKey = source["authKey"];
	        this.controlUrl = source["controlUrl"];
	        this.exitNode = source["exitNode"];
	        this.hostAccess = source["hostAccess"];
	    }
	}
	export class Event {
	    time: string;
	    kind: string;
	    tailnet: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Event(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.kind = source["kind"];
	        this.tailnet = source["tailnet"];
	        this.message = source["message"];
	    }
	}
	export class TailnetRow {
	    id: string;
	    namespace: string;
	    status: string;
	    address: string;
	    peers: number;
	    hostAccess: string;
	    exitNode: string;
	
	    static createFrom(source: any = {}) {
	        return new TailnetRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.namespace = source["namespace"];
	        this.status = source["status"];
	        this.address = source["address"];
	        this.peers = source["peers"];
	        this.hostAccess = source["hostAccess"];
	        this.exitNode = source["exitNode"];
	    }
	}
	export class Metrics {
	    tailnets: number;
	    reconnecting: number;
	    peers: number;
	    hostAccessOn: number;
	    reconcileSec: number;
	    uptime: string;
	
	    static createFrom(source: any = {}) {
	        return new Metrics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tailnets = source["tailnets"];
	        this.reconnecting = source["reconnecting"];
	        this.peers = source["peers"];
	        this.hostAccessOn = source["hostAccessOn"];
	        this.reconcileSec = source["reconcileSec"];
	        this.uptime = source["uptime"];
	    }
	}
	export class Dashboard {
	    host: string;
	    healthy: number;
	    dnsOk: boolean;
	    version: string;
	    metrics: Metrics;
	    tailnets: TailnetRow[];
	    events: Event[];
	
	    static createFrom(source: any = {}) {
	        return new Dashboard(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.healthy = source["healthy"];
	        this.dnsOk = source["dnsOk"];
	        this.version = source["version"];
	        this.metrics = this.convertValues(source["metrics"], Metrics);
	        this.tailnets = this.convertValues(source["tailnets"], TailnetRow);
	        this.events = this.convertValues(source["events"], Event);
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
	
	export class KV {
	    key: string;
	    value: string;
	    dim: boolean;
	
	    static createFrom(source: any = {}) {
	        return new KV(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	        this.dim = source["dim"];
	    }
	}
	
	export class Peer {
	    hostName: string;
	    address: string;
	    os: string;
	    routes: string;
	    lastSeen: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new Peer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostName = source["hostName"];
	        this.address = source["address"];
	        this.os = source["os"];
	        this.routes = source["routes"];
	        this.lastSeen = source["lastSeen"];
	        this.status = source["status"];
	    }
	}
	export class Settings {
	    mode: string;
	    socketPath: string;
	    sshHost: string;
	    remoteSocket: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.socketPath = source["socketPath"];
	        this.sshHost = source["sshHost"];
	        this.remoteSocket = source["remoteSocket"];
	    }
	}
	export class TailnetDetail {
	    id: string;
	    namespace: string;
	    status: string;
	    address: string;
	    peerCount: number;
	    uptime: string;
	    network: KV[];
	    dns: KV[];
	    hostAccess: KV[];
	    routes: KV[];
	    peers: Peer[];
	    events: Event[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new TailnetDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.namespace = source["namespace"];
	        this.status = source["status"];
	        this.address = source["address"];
	        this.peerCount = source["peerCount"];
	        this.uptime = source["uptime"];
	        this.network = this.convertValues(source["network"], KV);
	        this.dns = this.convertValues(source["dns"], KV);
	        this.hostAccess = this.convertValues(source["hostAccess"], KV);
	        this.routes = this.convertValues(source["routes"], KV);
	        this.peers = this.convertValues(source["peers"], Peer);
	        this.events = this.convertValues(source["events"], Event);
	        this.error = source["error"];
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

