package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Tributary-ai-services/assembler-mcp/internal/formats"
	"github.com/Tributary-ai-services/assembler-mcp/internal/storage"
	"github.com/Tributary-ai-services/assembler-mcp/internal/templates"
)

// MCPToolsListResponse is the response for GET /mcp/tools/list
type MCPToolsListResponse struct {
	Tools []MCPTool `json:"tools"`
}

// MCPTool defines a single MCP tool
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// MCPToolCallRequest is the request body for POST /mcp/tools/call
type MCPToolCallRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// MCPToolCallResponse is the response for POST /mcp/tools/call
type MCPToolCallResponse struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError"`
}

// MCPContent is a content block in the MCP response
type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

var (
	templateEngine *templates.Engine
	formatWriter   *formats.Writer
	storageClient  *storage.Client
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
	}

	// Initialize template engine with built-in templates
	templatesDir := os.Getenv("TEMPLATES_DIR")
	if templatesDir == "" {
		templatesDir = "./templates"
	}
	templateEngine = templates.NewEngine(templatesDir)

	// Initialize format writer
	formatWriter = formats.NewWriter()

	// Initialize storage client (MinIO)
	minioEndpoint := os.Getenv("MINIO_ENDPOINT")
	if minioEndpoint == "" {
		minioEndpoint = "minio-shared.tas-shared.svc.cluster.local:9000"
	}
	minioAccessKey := os.Getenv("MINIO_ACCESS_KEY")
	if minioAccessKey == "" {
		minioAccessKey = "minioadmin"
	}
	minioSecretKey := os.Getenv("MINIO_SECRET_KEY")
	if minioSecretKey == "" {
		minioSecretKey = "minioadmin123"
	}
	minioBucket := os.Getenv("MINIO_BUCKET")
	if minioBucket == "" {
		minioBucket = "assembler-outputs"
	}
	minioUseSSL := os.Getenv("MINIO_USE_SSL") == "true"

	var err error
	storageClient, err = storage.NewClient(minioEndpoint, minioAccessKey, minioSecretKey, minioBucket, minioUseSSL)
	if err != nil {
		log.Printf("Warning: MinIO client initialization failed (storage disabled): %v", err)
	}

	// Routes
	http.HandleFunc("/mcp/tools/list", handleToolsList)
	http.HandleFunc("/mcp/tools/call", handleToolsCall)
	http.HandleFunc("/health", handleHealth)

	log.Printf("Assembler MCP Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleToolsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tools := MCPToolsListResponse{
		Tools: []MCPTool{
			{
				Name:        "assemble_document",
				Description: "Render a template with parameters and output as target format (markdown, docx, pdf)",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"template_name":    map[string]interface{}{"type": "string", "description": "Name of a built-in template to use"},
						"template_content": map[string]interface{}{"type": "string", "description": "Custom Go text/template content (used if template_name not provided)"},
						"params":           map[string]interface{}{"type": "object", "description": "Template parameters as key-value pairs"},
						"output_format":    map[string]interface{}{"type": "string", "enum": []string{"markdown", "docx", "pdf"}, "description": "Output format"},
					},
					"required": []string{"params"},
				},
			},
			{
				Name:        "list_templates",
				Description: "List available assembly templates",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"category": map[string]interface{}{"type": "string", "description": "Filter by category (optional)"},
					},
				},
			},
			{
				Name:        "create_template",
				Description: "Save a new Go text/template for document assembly",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":        map[string]interface{}{"type": "string", "description": "Template name"},
						"content":     map[string]interface{}{"type": "string", "description": "Go text/template content"},
						"category":    map[string]interface{}{"type": "string", "description": "Template category"},
						"description": map[string]interface{}{"type": "string", "description": "Template description"},
					},
					"required": []string{"name", "content"},
				},
			},
			{
				Name:        "store_artifact",
				Description: "Store an assembled document to MinIO object storage",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"content":     map[string]interface{}{"type": "string", "description": "Document content (text or base64 for binary)"},
						"filename":    map[string]interface{}{"type": "string", "description": "Output filename"},
						"format":      map[string]interface{}{"type": "string", "description": "File format (md, docx, pdf)"},
						"workflow_id": map[string]interface{}{"type": "string", "description": "Associated workflow ID"},
					},
					"required": []string{"content", "filename"},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tools)
}

func handleToolsCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MCPToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	var result string
	var err error

	switch req.Name {
	case "assemble_document":
		result, err = handleAssembleDocument(req.Arguments)
	case "list_templates":
		result, err = handleListTemplates(req.Arguments)
	case "create_template":
		result, err = handleCreateTemplate(req.Arguments)
	case "store_artifact":
		result, err = handleStoreArtifact(req.Arguments)
	default:
		respondError(w, fmt.Sprintf("Unknown tool: %s", req.Name))
		return
	}

	if err != nil {
		respondError(w, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MCPToolCallResponse{
		Content: []MCPContent{{Type: "text", Text: result}},
		IsError: false,
	})
}

func handleAssembleDocument(args map[string]interface{}) (string, error) {
	params, _ := args["params"].(map[string]interface{})
	if params == nil {
		params = make(map[string]interface{})
	}

	outputFormat, _ := args["output_format"].(string)
	if outputFormat == "" {
		outputFormat = "markdown"
	}

	// Get template content
	var templateContent string
	if name, ok := args["template_name"].(string); ok && name != "" {
		tmpl, err := templateEngine.Get(name)
		if err != nil {
			return "", fmt.Errorf("template not found: %s", name)
		}
		templateContent = tmpl.Content
	} else if content, ok := args["template_content"].(string); ok && content != "" {
		templateContent = content
	} else {
		// Default: generate a simple document from params
		templateContent = templates.DefaultTemplate()
	}

	// Render template
	rendered, err := templateEngine.Render(templateContent, params)
	if err != nil {
		return "", fmt.Errorf("template rendering failed: %v", err)
	}

	// Convert to output format
	switch outputFormat {
	case "markdown", "md":
		return rendered, nil
	case "docx":
		return formatWriter.ToDocx(rendered)
	case "pdf":
		return formatWriter.ToPDF(rendered)
	default:
		return rendered, nil
	}
}

func handleListTemplates(args map[string]interface{}) (string, error) {
	category, _ := args["category"].(string)
	tmplList := templateEngine.List(category)

	data, err := json.MarshalIndent(tmplList, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func handleCreateTemplate(args map[string]interface{}) (string, error) {
	name, _ := args["name"].(string)
	content, _ := args["content"].(string)
	category, _ := args["category"].(string)
	description, _ := args["description"].(string)

	if name == "" || content == "" {
		return "", fmt.Errorf("name and content are required")
	}

	err := templateEngine.Create(name, content, category, description)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Template '%s' created successfully", name), nil
}

func handleStoreArtifact(args map[string]interface{}) (string, error) {
	if storageClient == nil {
		return "", fmt.Errorf("storage client not initialized")
	}

	content, _ := args["content"].(string)
	filename, _ := args["filename"].(string)
	format, _ := args["format"].(string)
	workflowID, _ := args["workflow_id"].(string)

	if content == "" || filename == "" {
		return "", fmt.Errorf("content and filename are required")
	}

	objectKey := filename
	if workflowID != "" {
		objectKey = fmt.Sprintf("workflows/%s/%s", workflowID, filename)
	}

	url, err := storageClient.Upload(objectKey, []byte(content), format)
	if err != nil {
		return "", fmt.Errorf("failed to store artifact: %v", err)
	}

	result := map[string]string{
		"status":   "stored",
		"filename": filename,
		"url":      url,
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

func respondError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MCPToolCallResponse{
		Content: []MCPContent{{Type: "text", Text: msg}},
		IsError: true,
	})
}
