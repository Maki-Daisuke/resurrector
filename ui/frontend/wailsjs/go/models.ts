export namespace main {
	
	export class AppConfig {
	    name: string;
	    enabled: boolean;
	    command: string;
	    args: string;
	    cwd: string;
	    restartDelaySec: number;
	    healthyTimeoutSec: number;
	    hideWindow: boolean;
	    maxRetries: number;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.cwd = source["cwd"];
	        this.restartDelaySec = source["restartDelaySec"];
	        this.healthyTimeoutSec = source["healthyTimeoutSec"];
	        this.hideWindow = source["hideWindow"];
	        this.maxRetries = source["maxRetries"];
	    }
	}

}

