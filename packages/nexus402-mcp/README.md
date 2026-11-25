# @nexus402/mcp

MCP server for the Nexus402 worker. Inspect mode is the default. Auto-pay runs only when the agent key, principal, and escrow address are all set.

`MAX_PER_TASK_TUSD8` is the most `fetch402` will lock for one call. The default is `1.00`.

## Inspect mode

Claude Desktop `claude_desktop_config.json` and Cursor `.cursor/mcp.json` use the same server entry.

```json
{
  "mcpServers": {
    "nexus402": {
      "command": "npx",
      "args": ["-y", "@nexus402/mcp"],
      "env": {
        "NEXUS_WORKER_URL": "http://127.0.0.1:8080",
        "MAX_PER_TASK_TUSD8": "1.00"
      }
    }
  }
}
```

A tool call receives HTTP 402 and returns the tUSD8 amount, chain id `44005`, and `taskHash`. It does not sign or lock funds.

## Auto-pay mode

```json
{
  "mcpServers": {
    "nexus402": {
      "command": "npx",
      "args": ["-y", "@nexus402/mcp"],
      "env": {
        "NEXUS_WORKER_URL": "http://127.0.0.1:8080",
        "AEVUM_RPC_URL": "https://rpc-test.qday.info",
        "NEXUS_ESCROW_ADDRESS": "<deployed-escrow>",
        "PRINCIPAL_ADDRESS": "<principal>",
        "ORCHESTRATOR_AGENT_PRIVATE_KEY": "<agent-session-private-key>",
        "MAX_PER_TASK_TUSD8": "1.00"
      }
    }
  }
}
```

The paid tool result includes `_meta["x402/payment-response"]` with `taskId`, `lockTxHash`, `claimTxHash`, and `explorerUrl`.

From this repository, before the package is published:

```json
{
  "command": "node",
  "args": ["packages/nexus402-mcp/dist/index.js"]
}
```

Add `--http` to that argument list for Streamable HTTP on `127.0.0.1:8787` at `/mcp`. `MCP_HTTP_PORT` changes the port.
