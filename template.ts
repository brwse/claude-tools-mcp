import { Template, waitForPort } from "e2b";

export const template = Template()
	.fromImage("ghcr.io/brwse/claude-tools-mcp:latest")
	.setStartCmd("claude-tools-mcp --addr 0.0.0.0:8080", waitForPort(8080));
