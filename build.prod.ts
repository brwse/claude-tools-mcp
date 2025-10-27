import { defaultBuildLogger, Template } from "e2b";
import { template } from "./template";

async function main() {
	await Template.build(template, {
		alias: "claude-tools",
    cpuCount: 2,
    memoryMB: 2048,
		onBuildLogs: defaultBuildLogger(),
	});
}

main().catch(console.error);
