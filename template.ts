import { Template } from "e2b";

export const template = Template()
	.fromImage("ghcr.io/brwse/claude-tools-mcp-runtime:latest")
	.runCmd("claude-tools-mcp --addr 0.0.0.0:8080");
