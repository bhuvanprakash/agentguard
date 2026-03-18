from agentguard import guard
from mcp import MCPClient

client = MCPClient(guard("http://localhost:8080"))
