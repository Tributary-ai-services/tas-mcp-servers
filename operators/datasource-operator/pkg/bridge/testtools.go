package bridge

// TestToolDef defines a lightweight test tool invocation for a datasource type
type TestToolDef struct {
	Tool   string
	Params map[string]interface{}
}

// TestToolDefs maps datasource types to their test tool definitions
var TestToolDefs = map[string]TestToolDef{
	"postgres": {Tool: "query", Params: map[string]interface{}{"sql": "SELECT 1"}},
	"neo4j":    {Tool: "execute_query", Params: map[string]interface{}{"query": "RETURN 1 AS n"}},
	"minio":    {Tool: "list_buckets", Params: map[string]interface{}{}},
	"kafka":    {Tool: "list_topics", Params: map[string]interface{}{}},
	"grafana":  {Tool: "list_dashboards", Params: map[string]interface{}{}},
}

// DefaultBridgeEndpoints maps datasource types to their default bridge endpoints
var DefaultBridgeEndpoints = map[string]string{
	"postgres": "http://postgres-mcp.tas-mcp-servers.svc.cluster.local:8000",
	"neo4j":    "http://neo4j-mcp.tas-mcp-servers.svc.cluster.local:8000",
	"minio":    "http://minio-mcp.tas-mcp-servers.svc.cluster.local:8000",
	"kafka":    "http://kafka-mcp.tas-mcp-servers.svc.cluster.local:8000",
	"grafana":  "http://grafana-mcp.tas-mcp-servers.svc.cluster.local:8000",
}
